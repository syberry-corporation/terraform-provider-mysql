---
layout: "mysql"
page_title: "MySQL: mysql_grant"
sidebar_current: "docs-mysql-resource-grant"
description: |-
  Creates and manages privileges given to a user on a MySQL server
---

# mysql\_grant

The ``mysql_grant`` resource creates and manages privileges given to
a user on a MySQL server.

## Granting Privileges to a User

```hcl
# AWS secret value: {"jdoe_username": "<username>", "jdoe_password": "<password>"}

resource "mysql_user" "jdoe" {
  secret_name  = "rds-credentials"
  username_key = "jdoe_username"
  host         = "%"
  password_key = "jdoe_password"
}

resource "mysql_grant" "jdoe" {
  secret_name  = mysql_user.jdoe.secret_name
  username_key = mysql_user.jdoe.username_key
  host         = mysql_user.jdoe.host
  database     = "app"
  privileges   = ["SELECT", "UPDATE"]
  table        = "*"
}

resource "mysql_grant" "jdoe" {
  secret_name  = mysql_user.jdoe.secret_name
  username_key = mysql_user.jdoe.username_key
  host         = mysql_user.jdoe.host
  database     = "app2"
  privileges   = ["ALL PRIVILEGES"]
  table        = "pets"
}
```

~> **Note:** You can not create GRANT on nonexisting SQL table.

## Granting Privileges to a Role

```hcl
resource "mysql_role" "developer" {
  name = "developer"
}

resource "mysql_grant" "developer" {
  role       = "${mysql_role.developer.name}"
  database   = "app"
  privileges = ["SELECT", "UPDATE"]
}
```

## Adding a Role to a User

```hcl
resource "mysql_user" "jdoe" {
  user               = "jdoe"
  host               = "example.com"
  plaintext_password = "password"
}

resource "mysql_role" "developer" {
  name = "developer"
}

resource "mysql_grant" "developer" {
  user     = "${mysql_user.jdoe.user}"
  host     = "${mysql_user.jdoe.host}"
  database = "app"
  roles    = ["${mysql_role.developer.name}"]
}

resource "mysql_grant" "developer" {
  user      = "${mysql_user.jdoe.user}"
  host      = "${mysql_user.jdoe.host}"
  database  = "mysql.rds_kill_query" # set the procedure name here. Don't set table anywhere. NOTE: for revoking, only execute is supported. No other permissions are currently supported
  procedure = true
}
```

## Argument Reference

~> **Note:** MySQL removed the `REQUIRE` option from `GRANT` in version 8. `tls_option` is ignored in MySQL 8 and above.

~> **Note:** Attributes `role` and `roles` are only supported in MySQL 8 and above.

The following arguments are supported:

* `secret_name` - (Required) Then name of AWS secret, where username is stored.
* `username_key` - (Required) The key(field) in json of secret string.
* `host` - (Optional) The source host of the user. Defaults to "localhost". Conflicts with `role`.
* `role` - (Optional) The role to grant `privileges` to. Conflicts with `user` and `host`.
* `database` - (Required) The database to grant privileges on.
* `table` - (Optional) Which table to grant `privileges` on. Defaults to `*`, which is all tables.
* `privileges` - (Optional) A list of privileges to grant to the user. Refer to a list of privileges (such as [here](https://dev.mysql.com/doc/refman/5.5/en/grant.html)) for applicable privileges. Conflicts with `roles`.
* `roles` - (Optional) A list of rols to grant to the user. Conflicts with `privileges`.
* `tls_option` - (Optional) An TLS-Option for the `GRANT` statement. The value is suffixed to `REQUIRE`. A value of 'SSL' will generate a `GRANT ... REQUIRE SSL` statement. See the [MYSQL `GRANT` documentation](https://dev.mysql.com/doc/refman/5.7/en/grant.html) for more. Ignored if MySQL version is under 5.7.0.
* `grant` - (Optional) Whether to also give the user privileges to grant the same privileges to other users.
* `procedure` - (Optional) Only set this to true if you're looking to GRANT EXECUTE on PROCEDURE <procedure_name>.

## Attributes Reference

No further attributes are exported.

## Import

To import `mysql_grant` you should use this format of id `SECRET_NAME@USER@HOST`

```shell
terraform import mysql_grant.grant rds-credentials@jdoe_username@%
```
