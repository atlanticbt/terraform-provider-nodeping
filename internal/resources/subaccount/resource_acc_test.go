// Package subaccount's acceptance test.
//
// !!! THIS TEST CREATES AND DESTROYS A REAL, BILLABLE NODEPING SUBACCOUNT !!!
//
// It runs only when TF_ACC=1 and NODEPING_API_TOKEN are both set, and it must
// be pointed at a dedicated test account — never a client subaccount and never
// the production account. The subaccount it creates is destroyed at the end of
// the run, and destroying a subaccount destroys every check, contact and stored
// result inside it.
//
// Build tag: this file is excluded from the default build because
// github.com/hashicorp/terraform-plugin-testing is not yet a requirement in
// go.mod. Once it is, drop the `acceptance` tag from the build constraint and
// the Makefile target below; TF_ACC already gates execution on its own.
//
//	go test -tags acceptance ./internal/resources/subaccount/ -run TestAccSubaccount -v
//
// Use `make test-acc`, which sets the four environment variables OpenTofu needs
// (TF_ACC, TF_ACC_TERRAFORM_PATH, TF_ACC_PROVIDER_HOST, TF_ACC_PROVIDER_NAMESPACE).
// TF_ACC_TERRAFORM_PATH must point at `tofu`; without it plugin-testing
// downloads HashiCorp Terraform and tests the wrong binary.

package subaccount_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/provider"
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
