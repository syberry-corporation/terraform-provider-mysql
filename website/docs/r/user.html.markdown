---
layout: "mysql"
page_title: "MySQL: mysql_user"
sidebar_current: "docs-mysql-resource-user"
description: |-
  Creates and manages a user on a MySQL server.
---

# mysql\_user

The ``mysql_user`` resource creates and manages a user on a MySQL
server.

~> **Note:** The password for the user is provided in plain text, and is
obscured by an unsalted hash in the state
[Read more about sensitive data in state](/docs/state/sensitive-data.html).
Care is required when using this resource, to avoid disclosing the password.

## Example Usage

```hcl
# AWS secret value: {"jdoe_username": "<username>", "jdoe_password": "<password>"}

resource "mysql_user" "jdoe" {
  secret_name  = "rds-credentials"
  username_key = "jdoe_username"
  password_key = "jdoe_password"
  host         = "%"
}
```


## Argument Reference

The following arguments are supported:

* `secret_name` - (Required) The AWS secret name, where credentials stored.
* `username_key` - (Required) Key(field) in secret json string with username value.
* `host` - (Optional) The source host of the user. Defaults to "localhost".
* `password_key` - (Required) Key(field) in secret json string with username value.
* `tls_option` - (Optional) An TLS-Option for the `CREATE USER` or `ALTER USER` statement. The value is suffixed to `REQUIRE`. A value of 'SSL' will generate a `CREATE USER ... REQUIRE SSL` statement. See the [MYSQL `CREATE USER` documentation](https://dev.mysql.com/doc/refman/5.7/en/create-user.html) for more. Ignored if MySQL version is under 5.7.0.

## Attributes Reference

The following attributes are exported:

* `username_key` - The key in json of secret string.
* `password_key` - The password of the user.
* `secret_name` - The AWS secret name, where credentials stored
* `id` - The id of the user created, composed as "username@host".
* `host` - The host where the user was created.

## Import

`mysql_user` can be imported by using the next format of id `SECRET_NAME@USER_KEY@PASSWORD_KEY@HOST`

```shell
terraform import mysql_user.user rds-credentials@jdoe_username@jdoe_password@%
```
