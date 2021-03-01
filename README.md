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
      source  = "kaplanmaxe/mysql"
      version = "1.9.4"
    }
  }
  required_version = ">= 0.13"
}

provider "mysql" {
}
```
