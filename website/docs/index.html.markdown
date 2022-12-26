---
layout: "mysql"
page_title: "Provider: MySQL"
sidebar_current: "docs-mysql-index"
description: |-
  A provider for MySQL Server.
---

# MySQL Provider

[MySQL](http://www.mysql.com) is a relational database server. The MySQL
provider exposes resources used to manage the configuration of resources
in a MySQL server.

Use the navigation to the left to read about the available resources.

## Example Usage

The following is a minimal example:

```hcl
# Configure the MySQL provider
provider "mysql" {
  endpoint = "my-database.example.com:3306"
  username = "app-user"
  password = "app-password"
}

# Create a Database
resource "mysql_database" "app" {
  name = "my_awesome_app"
}
```

This provider can be used in conjunction with other resources that create
MySQL servers. For example, ``aws_db_instance`` is able to create MySQL
servers in Amazon's RDS service. The next configuration defines how to avoid storing RDS password in Terraform state.

```hcl
# Create a database server
resource "aws_db_instance" "default" {
  engine         = "mysql"
  engine_version = "8.0.28"
  instance_class = "db.t3.micro"
  name           = "initial_db"
  username       = "rootuser"
  password       = "rootpasswd"

  # etc, etc; see aws_db_instance docs for more
}

# Creating secretsmanaget secret
resource "aws_secretsmanager_secret" "secret" {
  name = "rds-credentials"
}

# Generating random password, store it in AWS secret and apply to RDS instance
resource "null_resource" "change_db_pass" {
  provisioner "local-exec" {
    command = <<EOT
PWD=$(aws secretsmanager get-random-password --password-length 40 --exclude-characters '/@"\\'\'| jq -r .'RandomPassword')
aws secretsmanager put-secret-value \
	--secret-id $SECRET_ARN \
	--secret-string "$(echo "{}" | jq -rM --arg USER "$USER" --arg PWD "$PWD" '.rootpassword = $PWD | .rootusername = $USER')"
aws rds modify-db-instance --db-instance-identifier $RDS_INSTANCE_ID --master-user-password "$PWD" --apply-immediately
EOT
    environment = {
      SECRET_ARN      = aws_secretsmanager_secret.secret.id
      USER            = aws_db_instance.default.username
      RDS_INSTANCE_ID = aws_db_instance.default.id
    }
  }
}


# Configure the MySQL provider based on the outcome of
# creating the aws_db_instance.
provider "mysql" {
  endpoint = "${aws_db_instance.default.endpoint}"
  aws_secret = {
    secret_name  = aws_secretsmanager_secret.secret.name
    password_key = "root_password"
    username_key = "root_username"
  }
}

# Create a second database, in addition to the "initial_db" created
# by the aws_db_instance resource above.
resource "mysql_database" "app" {
  name = "another_db"
}
```

## SOCKS5 Proxy Support

The MySQL provider respects the `ALL_PROXY` and/or `all_proxy` environment variables.

```
$ export all_proxy="socks5://your.proxy:3306"
```

## Argument Reference

The following arguments are supported:

* `endpoint` - (Required) The address of the MySQL server to use. Most often a "hostname:port" pair, but may also be an absolute path to a Unix socket when the host OS is Unix-compatible. Can also be sourced from the `MYSQL_ENDPOINT` environment variable.
* `aws_secret` - (Required) Specify where credentials for MySQL is stored. See [below](#aws_secret-configuration-block)
* `proxy` - (Optional) Proxy socks url, can also be sourced from `ALL_PROXY` or `all_proxy` environment variables.
* `tls` - (Optional) The TLS configuration. One of `false`, `true`, or `skip-verify`. Defaults to `false`. Can also be sourced from the `MYSQL_TLS_CONFIG` environment variable.
* `max_conn_lifetime_sec` - (Optional) Sets the maximum amount of time a connection may be reused. If d <= 0, connections are reused forever.
* `max_open_conns` - (Optional) Sets the maximum number of open connections to the database. If n <= 0, then there is no limit on the number of open connections.
* `authentication_plugin` - (Optional) Sets the authentication plugin, it can be one of the following: `native` or `cleartext`. Defaults to `native`.

### aws_secret argument configuration block

Supported nested arguments for the `aws_secret` configuration block:

* `secret_name` - (Required) AWS secret name. 
* `region` - (Required) AWS region. Can also be sourced from the `AWS_DEFAULT_REGION` environment variable.
* `username_key` - (Required) key(field) in secret json string.
* `password_key` - (Required) key(field) in secret json string.