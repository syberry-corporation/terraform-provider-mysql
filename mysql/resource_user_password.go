package mysql

import (
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/helper/encryption"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/sethvargo/go-password/password"
)

const (
	requiredPasswordLength = 32
)

func resourceUserPassword() *schema.Resource {
	return &schema.Resource{
		Create: SetUserPassword,
		Read:   ReadUserPassword,
		Delete: DeleteUserPassword,
		Schema: map[string]*schema.Schema{
			"user": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"host": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "localhost",
			},
			"pgp_key": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},
			"key_fingerprint": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"encrypted_password": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"password_policy": {
				Type:     schema.TypeSet,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"length": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  64,
							ForceNew: true,
							ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
								value := v.(int)
								if value < requiredPasswordLength {
									errors = append(errors, fmt.Errorf("password_policy_length must be at least %d", requiredPasswordLength))
								}

								return
							},
						},
						"allow_repeat": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},
						"num_symbols": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  5,
							ForceNew: true,
						},
						"num_digits": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  5,
							ForceNew: true,
						},
					},
				},
			},
		},
	}
}

func SetUserPassword(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	var (
		passwordPolicyLength      int
		passwordPolicyNumDigits   int
		passwordPolicyNumSymbols  int
		passwordPolicyAllowRepeat bool
	)
	if v, ok := d.GetOk("password_policy"); ok && v.(*schema.Set).Len() > 0 {
		for _, tfMapRaw := range v.(*schema.Set).List() {
			tfMap, ok := tfMapRaw.(map[string]interface{})

			if !ok {
				continue
			}

			if v, ok := tfMap["length"].(int); ok {
				passwordPolicyLength = v
			}

			if v, ok := tfMap["num_digits"].(int); ok {
				passwordPolicyNumDigits = v
			}

			if v, ok := tfMap["num_symbols"].(int); ok {
				passwordPolicyNumSymbols = v
			}

			if v, ok := tfMap["allow_repeat"].(bool); ok {
				passwordPolicyAllowRepeat = v
			}
		}
	}

	// TODO: make dynamic and allow user to set
	password, err := password.Generate(passwordPolicyLength, passwordPolicyNumDigits, passwordPolicyNumSymbols, false, passwordPolicyAllowRepeat)
	if err != nil {
		return err
	}

	pgpKey := d.Get("pgp_key").(string)
	encryptionKey, err := encryption.RetrieveGPGKey(pgpKey)
	if err != nil {
		return err
	}
	fingerprint, encrypted, err := encryption.EncryptValue(encryptionKey, password, "MySQL Password")
	if err != nil {
		return err
	}
	d.Set("key_fingerprint", fingerprint)
	d.Set("encrypted_password", encrypted)

	requiredVersion, _ := version.NewVersion("8.0.0")
	currentVersion, err := serverVersion(db)
	if err != nil {
		return err
	}

	isMaria, err := checkIfMariaDB(db)
	if err != nil {
		return err
	}

	passSQL := fmt.Sprintf("'%s'", password)
	// MariaDB still relies on PASSWORD() helper
	if currentVersion.LessThan(requiredVersion) || isMaria == true {
		passSQL = fmt.Sprintf("PASSWORD(%s)", passSQL)
	}

	sql := fmt.Sprintf("SET PASSWORD FOR '%s'@'%s' = %s",
		d.Get("user").(string),
		d.Get("host").(string),
		passSQL)

	_, err = db.Exec(sql)
	if err != nil {
		return err
	}
	user := fmt.Sprintf("%s@%s",
		d.Get("user").(string),
		d.Get("host").(string))
	d.SetId(user)
	return nil
}

func ReadUserPassword(d *schema.ResourceData, meta interface{}) error {
	// This is obviously not possible.
	return nil
}

func DeleteUserPassword(d *schema.ResourceData, meta interface{}) error {
	// We don't need to do anything on the MySQL side here. Just need TF
	// to remove from the state file.
	return nil
}
