package mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/go-version"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"

	"golang.org/x/net/proxy"
)

const (
	cleartextPasswords = "cleartext"
	nativePasswords    = "native"
)

type MySQLConfiguration struct {
	Config                 *mysql.Config
	Db                     *sql.DB
	MaxConnLifetime        time.Duration
	MaxOpenConns           int
	ConnectRetryTimeoutSec time.Duration
	AWSRegion              string
}

func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("MYSQL_ENDPOINT", nil),
				ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
					value := v.(string)
					if value == "" {
						errors = append(errors, fmt.Errorf("Endpoint must not be an empty string"))
					}

					return
				},
			},

			"proxy": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"ALL_PROXY",
					"all_proxy",
				}, nil),
				ValidateFunc: validation.StringMatch(regexp.MustCompile("^socks5h?://.*:\\d+$"), "The proxy URL is not a valid socks url."),
			},

			"tls": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("MYSQL_TLS_CONFIG", "false"),
				ValidateFunc: validation.StringInSlice([]string{
					"true",
					"false",
					"skip-verify",
				}, false),
			},

			"max_conn_lifetime_sec": {
				Type:     schema.TypeInt,
				Optional: true,
			},

			"max_open_conns": {
				Type:     schema.TypeInt,
				Optional: true,
			},

			"authentication_plugin": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      nativePasswords,
				ValidateFunc: validation.StringInSlice([]string{cleartextPasswords, nativePasswords}, true),
			},

			"connect_retry_timeout_sec": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  300,
			},

			"aws_secret": {
				Type:        schema.TypeMap,
				Required:    true,
				Description: "Get username and password from AWS secret",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"secret_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "AWS secret name",
							ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
								value := v.(string)
								if value == "" {
									errors = append(errors, fmt.Errorf("Secret name must not be an empty string"))
								}
			
								return
							},
						},
						"region": {
							Type:        schema.TypeString,
							Required:    true,
							DefaultFunc: schema.EnvDefaultFunc("AWS_DEFAULT_REGION", nil),
							Description: "AWS region name for all secrets",
							ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
								value := v.(string)
								if value == "" {
									errors = append(errors, fmt.Errorf("Region must not be an empty string"))
								}
			
								return
							},
						},
						"username_key": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "json key in secret revision",
						},
						"password_key": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "json key in secret revision",
						},
					},
				},
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"mysql_tables": dataSourceTables(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"mysql_database":      resourceDatabase(),
			"mysql_grant":         resourceGrant(),
			"mysql_role":          resourceRole(),
			"mysql_user":          resourceUser(),
			"mysql_user_password": resourceUserPassword(),
			"mysql_sql":           resourceSql(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {

	var endpoint = d.Get("endpoint").(string)

	proto := "tcp"
	if len(endpoint) > 0 && endpoint[0] == '/' {
		proto = "unix"
	}

	var password string
	var username string
	var secretName string
	var AWSRegion string

	awsSecretConfig := d.Get("aws_secret")
	secretConfig := awsSecretConfig.(map[string]interface{})
	secretName = secretConfig["secret_name"].(string)
	AWSRegion = secretConfig["region"].(string)
	password, err := getValueFromSecret(secretName, AWSRegion, secretConfig["password_key"].(string))
	if err != nil {
		return nil, fmt.Errorf("Error getting secret value: %v", err)
	}
	username, err = getValueFromSecret(secretName, AWSRegion, secretConfig["username_key"].(string))
	if err != nil {
		return nil, fmt.Errorf("Error getting secret value: %v", err)
	}

	conf := mysql.Config{
		User:                    username,
		Passwd:                  password,
		Net:                     proto,
		Addr:                    endpoint,
		TLSConfig:               d.Get("tls").(string),
		AllowNativePasswords:    d.Get("authentication_plugin").(string) == nativePasswords,
		AllowCleartextPasswords: d.Get("authentication_plugin").(string) == cleartextPasswords,
	}

	dialer, err := makeDialer(d)
	if err != nil {
		return nil, err
	}

	mysql.RegisterDial("tcp", func(network string) (net.Conn, error) {
		return dialer.Dial("tcp", network)
	})

	mysqlConf := &MySQLConfiguration{
		Config:                 &conf,
		MaxConnLifetime:        time.Duration(d.Get("max_conn_lifetime_sec").(int)) * time.Second,
		MaxOpenConns:           d.Get("max_open_conns").(int),
		ConnectRetryTimeoutSec: time.Duration(d.Get("connect_retry_timeout_sec").(int)) * time.Second,
		AWSRegion:              AWSRegion,
	}

	db, err := connectToMySQL(mysqlConf)

	if err != nil {
		return nil, err
	}

	mysqlConf.Db = db

	return mysqlConf, nil
}

var identQuoteReplacer = strings.NewReplacer("`", "``")

func getValueFromSecret(secretId string, region string, key string) (string, error) {
	session, err := session.NewSession()
	if err != nil {
		return "", err
	}
	svc := secretsmanager.New(session, aws.NewConfig().WithRegion(region))
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretId),
		VersionStage: aws.String("AWSCURRENT"),
	}
	result, err := svc.GetSecretValue(input)
	if err != nil {
		return "", err
	}
	var secretValue map[string]string
	err = json.Unmarshal([]byte(*result.SecretString), &secretValue)
	if err != nil {
		return "", fmt.Errorf("Unmarshal error: %v. Check that secret value isn't clean", err)
	}
	password, exists := secretValue[key]
	if !exists {
		return "", fmt.Errorf("Key \"%s\" not found in the secret \"%s\"", key, secretId)
	}
	return password, nil
}

func makeDialer(d *schema.ResourceData) (proxy.Dialer, error) {
	proxyFromEnv := proxy.FromEnvironment()
	proxyArg := d.Get("proxy").(string)

	if len(proxyArg) > 0 {
		proxyURL, err := url.Parse(proxyArg)
		if err != nil {
			return nil, err
		}
		proxy, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}

		return proxy, nil
	}

	return proxyFromEnv, nil
}

func quoteIdentifier(in string) string {
	return fmt.Sprintf("`%s`", identQuoteReplacer.Replace(in))
}

func serverVersion(db *sql.DB) (*version.Version, error) {
	var versionString string
	err := db.QueryRow("SELECT @@GLOBAL.innodb_version").Scan(&versionString)
	if err != nil {
		return nil, err
	}

	return version.NewVersion(versionString)
}

func serverVersionString(db *sql.DB) (string, error) {
	var versionString string
	err := db.QueryRow("SELECT @@GLOBAL.version").Scan(&versionString)
	if err != nil {
		return "", err
	}

	return versionString, nil
}

func checkIfMariaDB(db *sql.DB) (bool, error) {
	var versionString string
	err := db.QueryRow("SELECT @@GLOBAL.version").Scan(&versionString)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(versionString), "mariadb") {
		return true, nil
	}

	return false, nil
}

func connectToMySQL(conf *MySQLConfiguration) (*sql.DB, error) {

	dsn := conf.Config.FormatDSN()
	var db *sql.DB
	var err error

	// When provisioning a database server there can often be a lag between
	// when Terraform thinks it's available and when it is actually available.
	// This is particularly acute when provisioning a server and then immediately
	// trying to provision a database on it.
	retryError := resource.Retry(conf.ConnectRetryTimeoutSec, func() *resource.RetryError {
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			return resource.RetryableError(err)
		}

		err = db.Ping()
		if err != nil {
			return resource.RetryableError(err)
		}

		return nil
	})

	if retryError != nil {
		return nil, fmt.Errorf("Could not connect to server: %s", retryError)
	}
	db.SetConnMaxLifetime(conf.MaxConnLifetime)
	db.SetMaxOpenConns(conf.MaxOpenConns)
	return db, nil
}
