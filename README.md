**This repository is an unofficial fork**

---

Terraform Provider
==================

Usage
-----

```hcl
terraform {
  required_providers {
    mysql = {
      source  = "syberry-corporation/mysql"
    }
  }
  required_version = ">= 0.13"
}

provider "mysql" {
}
```
