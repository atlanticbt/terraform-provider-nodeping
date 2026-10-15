// Acceptance tests for nodeping_maintenance.
//
// Build-tagged because terraform-plugin-testing is not yet listed in go.mod;
// drop the tag (and the `-tags acc` below) once it is a declared dependency.
//
// Run with:
//
//	TF_ACC=1 \
//	TF_ACC_TERRAFORM_PATH=$(which tofu) \
//	TF_ACC_PROVIDER_HOST=registry.opentofu.org \
//	TF_ACC_PROVIDER_NAMESPACE=hashicorp \
//	NODEPING_API_TOKEN=... NODEPING_TEST_CHECK_ID=... \
//	go test -tags acc ./internal/resources/maintenance/ -v -timeout 30m
//
// These tests create and destroy live NodePing resources. Point them at the
// dedicated test subaccount, never a client subaccount and never the main
// account.
package maintenance_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nodeping": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	// A missing token means the suite was not meant to run at all, which is an
	// error. A missing fixture ID means this one test cannot run while the rest
	// of the suite still can, which is a skip. Failing there would make the
	// whole acceptance run unusable to anyone without every optional fixture.
	if os.Getenv("NODEPING_API_TOKEN") == "" {
		t.Fatal("NODEPING_API_TOKEN must be set for acceptance tests")
	}
	if os.Getenv("NODEPING_TEST_CHECK_ID") == "" {
		t.Skip("NODEPING_TEST_CHECK_ID is not set; skipping the maintenance tests")
	}
}

func testAccMaintenanceConfig(name, cron string, duration int) string {
	return fmt.Sprintf(`
resource "nodeping_maintenance" "test" {
  name      = %q
  cron      = %q
  duration  = %d
  enabled   = true
  checklist = [%q]
}
`, name, cron, duration, os.Getenv("NODEPING_TEST_CHECK_ID"))
}

func TestAccMaintenanceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMaintenanceConfig("tf-acc weekly patching", "0 3 * * 0", 60),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "name", "tf-acc weekly patching"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "cron", "0 3 * * 0"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "duration", "60"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "enabled", "true"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "checklist.#", "1"),
					resource.TestCheckResourceAttrSet("nodeping_maintenance.test", "id"),
				),
			},
			{
				ResourceName:      "nodeping_maintenance.test",
				ImportState:       true,
				ImportStateVerify: true,
				// processing and nextrun are server-side and move on their own
				// between the apply and the import refresh.
				ImportStateVerifyIgnore: []string{"processing", "nextrun"},
			},
			{
				Config: testAccMaintenanceConfig("tf-acc weekly patching renamed", "30 4 * * 1", 90),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "name", "tf-acc weekly patching renamed"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "cron", "30 4 * * 1"),
					resource.TestCheckResourceAttr("nodeping_maintenance.test", "duration", "90"),
				),
			},
		},
	})
}

func TestAccMaintenanceDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMaintenanceConfig("tf-acc datasource window", "0 5 * * 2", 30) + `
data "nodeping_maintenance" "test" {
  id = nodeping_maintenance.test.id
}

data "nodeping_maintenances" "all" {
  depends_on = [nodeping_maintenance.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.nodeping_maintenance.test", "name", "tf-acc datasource window"),
					resource.TestCheckResourceAttr("data.nodeping_maintenance.test", "duration", "30"),
					resource.TestCheckResourceAttrSet("data.nodeping_maintenances.all", "maintenances.#"),
				),
			},
		},
	})
}
