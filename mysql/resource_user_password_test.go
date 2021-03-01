package mysql

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
)

func TestAccUserPassword_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserPasswordConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user_password.test", "user", "jdoe"),
					resource.TestCheckResourceAttrSet("mysql_user_password.test", "encrypted_password"),
				),
			},
			{
				Config: testAccUserPasswordConfig_pw_policy,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user_password.test", "user", "jdoe"),
					resource.TestCheckResourceAttrSet("mysql_user_password.test", "encrypted_password"),
					resource.TestCheckOutput("mysql_user.test_pw_policy.user", "jdoe"),
				),
			},
		},
	})
}

const testAccUserPasswordConfig_basic = `
resource "mysql_user" "test" {
  user = "jdoe"
}

resource "mysql_user_password" "test" {
  user    = "${mysql_user.test.user}"
  pgp_key = "keybase:joestump"
}
`

const testAccUserPasswordConfig_pw_policy = `
resource "mysql_user" "test_pw_policy" {
  user = "jdoe"
}

resource "mysql_user_password" "test_pw_policy" {
  user    = "${mysql_user.test.user}"
  pgp_key = "keybase:joestump"
  password_policy = {
	  length = 32
  }
}
`
