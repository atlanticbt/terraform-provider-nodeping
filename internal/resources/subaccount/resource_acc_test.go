//go:build acceptance_subaccount

// Package subaccount's acceptance test.
//
// !!! THIS TEST CREATES AND DESTROYS A REAL NODEPING SUBACCOUNT !!!
//
// Creating subaccounts needs a token for a parent account, so this test runs
// with more privilege than any other. Point it at a parent account used only
// for testing, never one whose subaccounts hold real monitoring. The subaccount
// it creates is destroyed at the end of the run, and destroying a subaccount
// destroys every check, contact and stored result inside it.
//
// Build tag: this file is excluded from the default build, so `make test-acc`
// does not create a subaccount. Run it deliberately with its own target:
//
//	make test-acc-subaccount
//
// An earlier version of this comment claimed the exclusion was temporary,
// pending terraform-plugin-testing entering go.mod. That dependency landed, the
// tag did not, and for a while the test ran as part of every acceptance run
// with nothing but an environment variable standing between it and the parent
// account. The tag is now permanent and the separation is the point: creating
// and deleting a real account is not something an ordinary test run should be
// able to do by accident.
//
// TF_ACC_TERRAFORM_PATH must point at `tofu`; without it plugin-testing
// downloads HashiCorp Terraform and tests the wrong binary. The make target
// sets it along with the other three variables OpenTofu needs.

package subaccount_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/provider"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"nodeping": providerserver.NewProtocol6WithError(provider.New("acctest")()),
	}
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("NODEPING_API_TOKEN") == "" {
		t.Fatal("NODEPING_API_TOKEN must be set for acceptance tests")
	}
}

func testAccSubaccountConfig(name, timezone, location string) string {
	return fmt.Sprintf(`
resource "nodeping_subaccount" "test" {
  name        = %q
  contactname = "Terraform Acceptance Test"
  email       = "terraform-acctest@example.com"
  timezone    = %q
  location    = %q
}
`, name, timezone, location)
}

// TestAccSubaccountLifecycle creates a subaccount, updates its mutable fields,
// verifies import, and destroys it.
//
// BILLING WARNING: the created subaccount is real. If the test panics or the
// process is killed between the create and destroy steps, the subaccount is
// left behind and must be removed by hand in the NodePing dashboard.
func TestAccSubaccountLifecycle(t *testing.T) {
	name := "tf-acc-" + acctestSuffix()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccSubaccountConfig(name, "-7", "nam"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "name", name),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "location", "nam"),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "timezone", "-7"),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "status", "Active"),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "emailme", "no"),
					resource.TestCheckResourceAttrSet("nodeping_subaccount.test", "id"),
					resource.TestCheckResourceAttrSet("nodeping_subaccount.test", "type"),
				),
			},
			{
				ResourceName:      "nodeping_subaccount.test",
				ImportState:       true,
				ImportStateVerify: true,
				// NodePing accepts these on write but never returns them, so an
				// imported subaccount cannot reproduce them. See the schema.
				ImportStateVerifyIgnore: []string{
					"contactname",
					"email",
					"emailme",
					"autodiagnotifications",
					"location",
				},
			},
			{
				Config: testAccSubaccountConfig(name+"-renamed", "0", "eur"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "name", name+"-renamed"),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "timezone", "0"),
					resource.TestCheckResourceAttr("nodeping_subaccount.test", "location", "eur"),
				),
			},
		},
	})
}

func acctestSuffix() string {
	if v := os.Getenv("TF_ACC_RUN_ID"); v != "" {
		return v
	}
	return fmt.Sprintf("%d", os.Getpid())
}
