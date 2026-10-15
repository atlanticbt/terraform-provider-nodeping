// Acceptance tests for nodeping_notificationprofile.
//
// The build tag keeps this file out of the default build: it needs
// github.com/hashicorp/terraform-plugin-testing, and the resource must already
// be registered in internal/provider. Run it with:
//
//	make test-acc GOFLAGS=-tags=acc
//
// or directly:
//
//	TF_ACC=1 \
//	TF_ACC_TERRAFORM_PATH=$(command -v tofu) \
//	TF_ACC_PROVIDER_HOST=registry.opentofu.org \
//	TF_ACC_PROVIDER_NAMESPACE=hashicorp \
//	go test -tags acc ./internal/resources/notificationprofile/...
//
// Per CONTRIBUTING.md the tests run against the tofu binary and a dedicated NodePing
// test subaccount, never a client subaccount and never the main account.
package notificationprofile_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/provider"
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
	if os.Getenv("NODEPING_TEST_CONTACT_ID") == "" {
		t.Skip("NODEPING_TEST_CONTACT_ID is not set; skipping the notification profile tests")
	}
}

func testAccNotificationProfileConfig(name string, delay int) string {
	return fmt.Sprintf(`
resource "nodeping_notificationprofile" "test" {
  name = %q

  notifications {
    contact_id = %q
    delay      = %d
    schedule   = "All"
  }
}
`, name, os.Getenv("NODEPING_TEST_CONTACT_ID"), delay)
}

func TestAccNotificationProfile_lifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNotificationProfileConfig("tfacc-notification-profile", 0),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "name", "tfacc-notification-profile"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "notifications.#", "1"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "notifications.0.delay", "0"),
					resource.TestCheckResourceAttrSet("nodeping_notificationprofile.test", "id"),
					resource.TestCheckResourceAttrSet("nodeping_notificationprofile.test", "customer_id"),
				),
			},
			{
				ResourceName:      "nodeping_notificationprofile.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// The notifications array is replaced wholesale on update.
				Config: testAccNotificationProfileConfig("tfacc-notification-profile-renamed", 15),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "name", "tfacc-notification-profile-renamed"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "notifications.#", "1"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.test", "notifications.0.delay", "15"),
				),
			},
		},
	})
}

// TestAccNotificationProfile_repeatedContact guards the block-count regression:
// two blocks naming the same contact must survive apply as two blocks.
func TestAccNotificationProfile_repeatedContact(t *testing.T) {
	contactID := os.Getenv("NODEPING_TEST_CONTACT_ID")
	config := fmt.Sprintf(`
resource "nodeping_notificationprofile" "repeated" {
  name = "tfacc-notification-profile-repeated"

  notifications {
    contact_id = %[1]q
    delay      = 0
    schedule   = "All"
  }

  notifications {
    contact_id = %[1]q
    delay      = 30
    schedule   = "All"
  }
}
`, contactID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_notificationprofile.repeated", "notifications.#", "2"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.repeated", "notifications.0.delay", "0"),
					resource.TestCheckResourceAttr("nodeping_notificationprofile.repeated", "notifications.1.delay", "30"),
				),
			},
			{
				ResourceName:      "nodeping_notificationprofile.repeated",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
