package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

const nonexistingGrantErrCode = 1141

type MySQLGrant struct {
	Database   string
	Table      string
	Privileges []string
	Grant      bool
}

func resourceGrant() *schema.Resource {
	return &schema.Resource{
		Create: CreateGrant,
		Update: UpdateGrant,
		Read:   ReadGrant,
		Delete: DeleteGrant,
		Importer: &schema.ResourceImporter{
			State: ImportGrant,
		},

		Schema: map[string]*schema.Schema{
			"secret_name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "AWS secret name",
			},

			"username_key": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"role"},
				Description:   "where username is stored",
			},

			"role": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"username_key", "host"},
			},

			"host": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				Default:       "localhost",
				ConflictsWith: []string{"role"},
			},

			"database": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"table": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "*",
			},

			"privileges": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},

			"roles": {
				Type:          schema.TypeSet,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"privileges"},
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
			},

			"grant": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
			},

			"procedure": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
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

func flattenList(list []interface{}, template string) string {
	var result []string
	for _, v := range list {
		result = append(result, fmt.Sprintf(template, v.(string)))
	}

	return strings.Join(result, ", ")
}

func formatDatabaseName(database string) string {
	if strings.Compare(database, "*") != 0 && !strings.HasSuffix(database, "`") {
		return fmt.Sprintf("`%s`", database)
	}

	return database
}

func formatTableName(table string) string {
	if table == "" || table == "*" {
		return fmt.Sprintf("*")
	}
	return fmt.Sprintf("`%s`", table)
}

func userOrRole(user string, host string, role string, hasRoles bool) (string, bool, error) {
	if len(user) > 0 && len(host) > 0 {
		return fmt.Sprintf("'%s'@'%s'", user, host), false, nil
	} else if len(role) > 0 {
		if !hasRoles {
			return "", false, fmt.Errorf("Roles are only supported on MySQL 8 and above")
		}

		return fmt.Sprintf("'%s'", role), true, nil
	} else {
		return "", false, fmt.Errorf("user with host or a role is required")
	}
}

func supportsRoles(db *sql.DB) (bool, error) {
	currentVersion, err := serverVersion(db)
	if err != nil {
		return false, err
	}

	requiredVersion, _ := version.NewVersion("8.0.0")
	hasRoles := currentVersion.GreaterThan(requiredVersion)
	return hasRoles, nil
}

func CreateGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}

	var (
		privilegesOrRoles string
		grantOn           string
	)

	hasPrivs := false
	rolesGranted := 0
	if attr, ok := d.GetOk("privileges"); ok {
		privilegesOrRoles = flattenList(attr.(*schema.Set).List(), "%s")
		hasPrivs = true
	} else if attr, ok := d.GetOk("roles"); ok {
		if !hasRoles {
			return fmt.Errorf("Roles are only supported on MySQL 8 and above")
		}
		listOfRoles := attr.(*schema.Set).List()
		rolesGranted = len(listOfRoles)
		privilegesOrRoles = flattenList(listOfRoles, "'%s'")
	} else {
		return fmt.Errorf("One of privileges or roles is required")
	}

	usernameKey := d.Get("username_key").(string)
	host := d.Get("host").(string)
	role := d.Get("role").(string)

	user, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return err
	}

	userOrRole, isRole, err := userOrRole(user, host, role, hasRoles)
	if err != nil {
		return err
	}

	database := formatDatabaseName(d.Get("database").(string))

	table := formatTableName(d.Get("table").(string))

	if (!isRole || hasPrivs) && rolesGranted == 0 && d.Get("procedure").(bool) == false {
		grantOn = fmt.Sprintf(" ON %s.%s", database, table)
	} else if (!isRole || hasPrivs) && rolesGranted == 0 && d.Get("procedure").(bool) == true {
		grantOn = fmt.Sprintf(" ON PROCEDURE %s", d.Get("database").(string))
	}

	stmtSQL := fmt.Sprintf("GRANT %s%s TO %s",
		privilegesOrRoles,
		grantOn,
		userOrRole)

	// MySQL 8+ doesn't allow REQUIRE on a GRANT statement.
	if !hasRoles && d.Get("tls_option").(string) != "" {
		stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
	}

	if !hasRoles && !isRole && d.Get("grant").(bool) {
		stmtSQL += " WITH GRANT OPTION"
	}

	log.Println("Executing statement:", stmtSQL)
	_, err = db.Exec(stmtSQL)
	if err != nil {
		return fmt.Errorf("Error running SQL (%s): %s", stmtSQL, err)
	}

	id := fmt.Sprintf("%s@%s:%s", user, host, database)
	if isRole {
		id = fmt.Sprintf("%s:%s", role, database)
	}

	d.SetId(id)

	return ReadGrant(d, meta)
}

func ReadGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}
	usernameKey := d.Get("username_key").(string)
	user, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return err
	}

	userOrRole, _, err := userOrRole(
		user,
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)
	if err != nil {
		return err
	}

	grants, err := showGrants(db, userOrRole)

	if err != nil {
		log.Printf("[WARN] GRANT not found for %s - removing from state", userOrRole)
		d.SetId("")
		return nil
	}

	database := d.Get("database").(string)
	table := d.Get("table").(string)

	var privileges []string
	var grantOption bool

	for _, grant := range grants {
		if grant.Database == database && grant.Table == table {
			privileges = grant.Privileges
		}

		if grant.Grant {
			grantOption = true
		}
	}

	d.Set("privileges", privileges)
	d.Set("grant", grantOption)

	return nil
}

func UpdateGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}
	
	usernameKey := d.Get("username_key").(string)
	user, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return err
	}

	userOrRole, _, err := userOrRole(
		user,
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)

	if err != nil {
		return err
	}

	database := d.Get("database").(string)
	table := d.Get("table").(string)

	if d.HasChange("privileges") {
		err = updatePrivileges(d, db, userOrRole, database, table)

		if err != nil {
			return err
		}
	}

	return nil
}

func updatePrivileges(d *schema.ResourceData, db *sql.DB, user string, database string, table string) error {
	oldPrivsIf, newPrivsIf := d.GetChange("privileges")
	oldPrivs := oldPrivsIf.(*schema.Set)
	newPrivs := newPrivsIf.(*schema.Set)
	grantIfs := newPrivs.Difference(oldPrivs).List()
	revokeIfs := oldPrivs.Difference(newPrivs).List()

	if len(grantIfs) > 0 {
		grants := make([]string, len(grantIfs))

		for i, v := range grantIfs {
			grants[i] = v.(string)
		}

		sql := fmt.Sprintf("GRANT %s ON %s.%s TO %s", strings.Join(grants, ","), database, table, user)

		log.Printf("[DEBUG] SQL: %s", sql)

		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	if len(revokeIfs) > 0 {
		revokes := make([]string, len(revokeIfs))

		for i, v := range revokeIfs {
			revokes[i] = v.(string)
		}

		sql := fmt.Sprintf("REVOKE %s ON %s.%s FROM %s", strings.Join(revokes, ","), database, table, user)

		log.Printf("[DEBUG] SQL: %s", sql)

		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	return nil
}

func DeleteGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	secretName := d.Get("secret_name").(string)
	region := meta.(*MySQLConfiguration).AWSRegion

	database := formatDatabaseName(d.Get("database").(string))

	table := formatTableName(d.Get("table").(string))

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}
	
	usernameKey := d.Get("username_key").(string)
	user, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return err
	}

	userOrRole, isRole, err := userOrRole(
		user,
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)
	if err != nil {
		return err
	}

	roles := d.Get("roles").(*schema.Set)
	privileges := d.Get("privileges").(*schema.Set)

	var sql string
	if !isRole && len(roles.List()) == 0 && d.Get("procedure").(bool) == false {
		sql = fmt.Sprintf("REVOKE GRANT OPTION ON %s.%s FROM %s",
			database,
			table,
			userOrRole)

		log.Printf("[DEBUG] SQL: %s", sql)
		_, err = db.Exec(sql)
		if err != nil {
			return fmt.Errorf("error revoking GRANT (%s): %s", sql, err)
		}
	}

	whatToRevoke := fmt.Sprintf("ALL ON %s.%s", database, table)
	if len(roles.List()) > 0 {
		whatToRevoke = flattenList(roles.List(), "'%s'")
	} else if len(privileges.List()) > 0 && d.Get("procedure").(bool) == false {
		privilegeList := flattenList(privileges.List(), "%s")
		whatToRevoke = fmt.Sprintf("%s ON %s.%s", privilegeList, database, table)
	} else if d.Get("procedure").(bool) == true {
		// TODO: a bit hacky but will only revoke execute atm
		whatToRevoke = fmt.Sprintf("EXECUTE ON PROCEDURE %s", d.Get("database").(string))
	}

	sql = fmt.Sprintf("REVOKE %s FROM %s", whatToRevoke, userOrRole)
	log.Printf("[DEBUG] SQL: %s", sql)
	_, err = db.Exec(sql)
	if err != nil {
		return fmt.Errorf("error revoking ALL (%s): %s", sql, err)
	}

	return nil
}

func ImportGrant(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	ids := strings.SplitN(d.Id(), "@", 3)
	region := meta.(*MySQLConfiguration).AWSRegion

	if len(ids) != 3 {
		return nil, fmt.Errorf("wrong ID format %s (expected SECRET_NAME@USERNAME_KEY@HOST)", d.Id())
	}

	secretName := ids[0]
	usernameKey := ids[1]
	user, err := getValueFromSecret(secretName, region, usernameKey)
	if err != nil {
		return nil, err
	}
	host := ids[2]

	db := meta.(*MySQLConfiguration).Db

	grants, err := showGrants(db, fmt.Sprintf("'%s'@'%s'", user, host))

	if err != nil {
		return nil, err
	}

	results := []*schema.ResourceData{}

	for _, grant := range grants {
		results = append(results, restoreGrant(secretName, usernameKey, host, grant))
	}

	return results, nil
}

func restoreGrant(secretName string, usernameKey string, host string, grant *MySQLGrant) *schema.ResourceData {
	d := resourceGrant().Data(nil)

	database := grant.Database
	id := fmt.Sprintf("%s@%s@%s:%s", secretName, usernameKey, host, formatDatabaseName(database))
	d.SetId(id)

	d.Set("username_key", usernameKey)
	d.Set("secret_name", secretName)
	d.Set("host", host)
	d.Set("database", database)
	d.Set("table", grant.Table)
	d.Set("grant", grant.Grant)
	d.Set("tls_option", "NONE")
	d.Set("privileges", grant.Privileges)

	return d
}

func showGrants(db *sql.DB, user string) ([]*MySQLGrant, error) {
	grants := []*MySQLGrant{}

	sql := fmt.Sprintf("SHOW GRANTS FOR %s", user)
	rows, err := db.Query(sql)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	re := regexp.MustCompile(`^GRANT (.+) ON (.+?)\.(.+?) TO`)
	reGrant := regexp.MustCompile(`\bGRANT OPTION\b`)

	for rows.Next() {
		var rawGrant string

		err := rows.Scan(&rawGrant)

		if err != nil {
			return nil, err
		}

		m := re.FindStringSubmatch(rawGrant)

		if len(m) != 4 {
			return nil, fmt.Errorf("failed to parse grant statement: %s", rawGrant)
		}

		privsStr := m[1]
		priv_list := strings.Split(privsStr, ",")
		privileges := make([]string, len(priv_list))

		for i, priv := range priv_list {
			privileges[i] = strings.TrimSpace(priv)
		}

		grant := &MySQLGrant{
			Database:   strings.ReplaceAll(m[2], "`", ""),
			Table:      strings.Trim(m[3], "`"),
			Privileges: privileges,
			Grant:      reGrant.MatchString(rawGrant),
		}

		grants = append(grants, grant)
	}

	return grants, nil
}
