package contactgroup_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/provider"
)

// Acceptance tests run against a dedicated NodePing test subaccount and create
// and destroy live contacts and contact groups. They are skipped unless TF_ACC
// is set, and `make test-acc` is the only supported way to run them: it points
// TF_ACC_TERRAFORM_PATH at the tofu binary and sets the two provider host
// variables that plugin-testing otherwise fills in with Terraform defaults
// OpenTofu rejects.
//
// Deliberately no tfversion.SkipBelow guards: OpenTofu reports a hardcoded
// "1.13.0" host version, so a version gate written here would be meaningless.

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nodeping": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("NODEPING_API_TOKEN") == "" {
		t.Fatal("NODEPING_API_TOKEN must be set for acceptance tests")
	}
}

// testAccContactGroupConfig wires a group to contact *address* IDs, which is
// the only correct way to populate members. The second step adds the contact's
// SMS address so membership churn is exercised as well as the rename.
func testAccContactGroupConfig(groupName string, includeSMS bool) string {
	members := "    nodeping_contact.acc_ops.address[0].id,"
	if includeSMS {
		members = "    nodeping_contact.acc_ops.address[0].id,\n    nodeping_contact.acc_ops.address[1].id,"
	}

	return fmt.Sprintf(`
resource "nodeping_contact" "acc_ops" {
  name     = "tfacc-contactgroup-contact"
  custrole = "notify"

  address {
    type    = "email"
    address = "tfacc-contactgroup@example.com"
  }

  address {
    type    = "sms"
    address = "+15005550006"
  }
}

resource "nodeping_contactgroup" "acc" {
  name = %q

  members = [
%s
  ]
}
`, groupName, members)
}

// TestAccContactGroup_lifecycle covers create, import round trip, membership
// change, and a second import. ImportStateVerify is the highest value assertion
// here: it compares every attribute the provider writes on apply against the
// one it derives from a bare API response, which is where customer_id and
// member-set normalisation bugs surface.
func TestAccContactGroup_lifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccContactGroupConfig("tfacc contactgroup", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_contactgroup.acc", "id"),
					resource.TestCheckResourceAttrSet("nodeping_contactgroup.acc", "customer_id"),
					resource.TestCheckResourceAttr("nodeping_contactgroup.acc", "name", "tfacc contactgroup"),
					resource.TestCheckResourceAttr("nodeping_contactgroup.acc", "members.#", "1"),
					// members must hold an address ID, never the contact ID.
					resource.TestCheckTypeSetElemAttrPair(
						"nodeping_contactgroup.acc", "members.*",
						"nodeping_contact.acc_ops", "address.0.id",
					),
				),
			},
			{
				ResourceName:      "nodeping_contactgroup.acc",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccContactGroupConfig("tfacc contactgroup renamed", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contactgroup.acc", "name", "tfacc contactgroup renamed"),
					resource.TestCheckResourceAttr("nodeping_contactgroup.acc", "members.#", "2"),
					resource.TestCheckTypeSetElemAttrPair(
						"nodeping_contactgroup.acc", "members.*",
						"nodeping_contact.acc_ops", "address.1.id",
					),
				),
			},
			{
				ResourceName:      "nodeping_contactgroup.acc",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
