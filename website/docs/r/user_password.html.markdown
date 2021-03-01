---
layout: "mysql"
page_title: "MySQL: mysql_user_password"
sidebar_current: "docs-mysql-resource-user-password"
description: |-
  Creates and manages the password for a user on a MySQL server.
---
# mysql_user_password

The `mysql_user_password` resource sets and manages a password for a given 
user on a MySQL server.

~> **NOTE on MySQL Passwords:** This resource conflicts with the `password` 
   argument for `mysql_user`. This resource uses PGP encryption to avoid 
   storing unencrypted passwords in Terraform state.

## Example Usage

 ```hcl
resource "mysql_user" "jdoe" {
  user = "jdoe"
}

resource "mysql_user_password" "jdoe" {
  user    = "${mysql_user.jdoe.user}"
  pgp_key = "keybase:joestump"
  password_policy {
      length = 64
      num_digits = 5
      num_symbols = 5
      allow_repeat = true
  }
}
```

You can rotate passwords by running `terraform taint mysql_user_password.jdoe`. 
The next time Terraform applies a new password will be generated and the user's
password will be updated accordingly.

## Argument Reference
The following arguments are supported:

* `user` - (Required) The IAM user to associate with this access key.
* `pgp_key` - (Required) Either a base-64 encoded PGP public key, or a keybase username in the form `keybase:some_person_that_exists`.
* `host` - (Optional) The source host of the user. Defaults to `localhost`.
* `password_policy` - (Optional) The password_policy you'd like to enforce for each user's password.

## Attributes Reference

The following additional attributes are exported:

* `key_fingerprint` - The fingerprint of the PGP key used to encrypt the password 
* `encrypted_password` - The encrypted password, base64 encoded.

~> **NOTE:** The encrypted password may be decrypted using the command line,
   for example: `terraform output encrypted_password | base64 --decode | keybase pgp decrypt`.
