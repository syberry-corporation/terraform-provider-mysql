package mysql

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceUser() *schema.Resource {
	return &schema.Resource{
		Create: CreateUser,
		Update: UpdateUser,
		Read:   ReadUser,
		Delete: DeleteUser,
		Importer: &schema.ResourceImporter{
			State: ImportUser,
		},

		Schema: map[string]*schema.Schema{
			"secret_name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "AWS secret name",
			},

			"username_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "where username is stored",
			},

			"host": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "localhost",
			},

			"password_key": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: false,
				Description: "where password is stored",
			},

			"tls_option": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "NONE",
			},
		},
	}
}

func CreateUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	username, err := getValueFromSecret(secretName, region, d.Get("username_key").(string))
	if err != nil {
		return err
	}
	password, err := getValueFromSecret(secretName, region, d.Get("password_key").(string))
	if err != nil {
		return err
	}

	stmtSQL := fmt.Sprintf("CREATE USER '%s'@'%s'",
		username,
		d.Get("host").(string))

	stmtSQL = stmtSQL + fmt.Sprintf(" IDENTIFIED BY '%s'", password)

	requiredVersion, _ := version.NewVersion("5.7.0")
	currentVersion, err := serverVersion(db)
	if err != nil {
		return err
	}

	if currentVersion.GreaterThan(requiredVersion) && d.Get("tls_option").(string) != "" {
		stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
	}

	log.Println("Executing statement:", stmtSQL)
	_, err = db.Exec(stmtSQL)
	if err != nil {
		return err
	}

	userId := fmt.Sprintf("%s@%s@%s@%s", d.Get("secret_name"), d.Get("username_key").(string), d.Get("password_key").(string), d.Get("host").(string))
	d.SetId(userId)

	return nil
}

func UpdateUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion
	
	username, err := getValueFromSecret(secretName, region, d.Get("username_key").(string))
	if err != nil {
		return err
	}

	var newpw string
	if d.HasChange("password_key") {
		newpw, err = getValueFromSecret(secretName, region, d.Get("password_key").(string))
		if err != nil {
			return err
		}
	} else {
		newpw = ""
	}

	if newpw != "" {
		var stmtSQL string

		/* ALTER USER syntax introduced in MySQL 5.7.6 deprecates SET PASSWORD (GH-8230) */
		serverVersion, err := serverVersion(db)
		if err != nil {
			return fmt.Errorf("Could not determine server version: %s", err)
		}

		ver, _ := version.NewVersion("5.7.6")
		if serverVersion.LessThan(ver) {
			stmtSQL = fmt.Sprintf("SET PASSWORD FOR '%s'@'%s' = PASSWORD('%s')",
				username,
				d.Get("host").(string),
				newpw)
		} else {
			stmtSQL = fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'",
				username,
				d.Get("host").(string),
				newpw)
		}

		log.Println("Executing query:", stmtSQL)
		_, err = db.Exec(stmtSQL)
		if err != nil {
			return err
		}
	}

	requiredVersion, _ := version.NewVersion("5.7.0")
	currentVersion, err := serverVersion(db)
	if err != nil {
		return err
	}

	if d.HasChange("tls_option") && currentVersion.GreaterThan(requiredVersion) {
		var stmtSQL string

		stmtSQL = fmt.Sprintf("ALTER USER '%s'@'%s'  REQUIRE %s",
			username,
			d.Get("host").(string),
			fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string)))

		log.Println("Executing query:", stmtSQL)
		_, err := db.Exec(stmtSQL)
		if err != nil {
			return err
		}
	}

	return nil
}

func ReadUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	username, err := getValueFromSecret(secretName, region, d.Get("username_key").(string))
	if err != nil {
		return err
	}

	stmtSQL := fmt.Sprintf("SELECT USER FROM mysql.user WHERE USER='%s'",
		username)

	log.Println("Executing statement:", stmtSQL)

	rows, err := db.Query(stmtSQL)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() && rows.Err() == nil {
		d.SetId("")
	}
	return rows.Err()
}

func DeleteUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion
	
	username, err := getValueFromSecret(secretName, region, d.Get("username_key").(string))
	if err != nil {
		return err
	}

	stmtSQL := fmt.Sprintf("DROP USER '%s'@'%s'",
		username,
		d.Get("host").(string))

	log.Println("Executing statement:", stmtSQL)

	_, err = db.Exec(stmtSQL)
	if err == nil {
		d.SetId("")
	}
	return err
}

func ImportUser(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	region := meta.(*MySQLConfiguration).AWSRegion

	Ids := strings.SplitN(d.Id(), "@", 4)
	if len(Ids) != 4 {
		return nil, fmt.Errorf("wrong ID format %s (expected SECRET_NAME@USER_KEY@PASSWORD_KEY@HOST)", d.Id())
	}
	secretName := Ids[0]
	usernameKey := Ids[1]
	passwordKey := Ids[2]
	host := Ids[3]
	
	username, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return nil, err
	}
	
	password, err := getValueFromSecret(secretName, region, passwordKey)
	_ = password
	if err != nil {
		return nil, err
	}

	db := meta.(*MySQLConfiguration).Db

	var count int
	err = db.QueryRow("SELECT COUNT(1) FROM mysql.user WHERE user = ? AND host = ?", username, host).Scan(&count)

	if err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, fmt.Errorf("user from key '%s' in the secret %s not found", usernameKey, secretName)
	}

	d.Set("secret_name", secretName)
	d.Set("username_key", usernameKey)
	d.Set("password_key", passwordKey)
	d.Set("host", host)
	d.Set("tls_option", "NONE")

	return []*schema.ResourceData{d}, nil
}
