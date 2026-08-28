package check_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/provider"
)

// Acceptance tests run against a dedicated NodePing test subaccount and create
// and destroy live checks. They are skipped unless TF_ACC is set, and `make
// test-acc` is the only supported way to run them: it points
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

// testAccSubaccountID returns the customer ID of the dedicated test
// subaccount, skipping the test when it is not configured.
func testAccSubaccountID(t *testing.T) string {
	t.Helper()
	customerID := os.Getenv("NODEPING_TEST_CUSTOMER_ID")
	if customerID == "" {
		t.Skip("NODEPING_TEST_CUSTOMER_ID is not set; skipping the subaccount test")
	}
	return customerID
}

// TestAccCheck_importStateVerify is the highest value case in this file: a
// round trip through import compares every attribute the provider writes on
// apply against the one it derives from a bare API response, which catches
// normalisation and import bugs across the whole schema at once.
func TestAccCheck_importStateVerify(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_check" "test" {
  type         = "HTTP"
  target       = "https://example.com"
  label        = "tf-acc-import"
  enabled      = false
  interval     = 15
  threshold    = 5
  sens         = 2
  runlocations = ["nam"]
  tags         = ["tf-acc"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_check.test", "id"),
					resource.TestCheckResourceAttr("nodeping_check.test", "type", "HTTP"),
					resource.TestCheckResourceAttr("nodeping_check.test", "label", "tf-acc-import"),
					resource.TestCheckResourceAttr("nodeping_check.test", "tags.#", "1"),
				),
			},
			{
				ResourceName:      "nodeping_check.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccCheck_writeOnlyPasswordNeverReachesState covers the property the
// write-only attribute exists for. NodePing returns the password in plaintext
// on every read, so an assertion that state is clean is the only thing that
// proves the provider is not writing it back.
func TestAccCheck_writeOnlyPasswordNeverReachesState(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_check" "pw" {
  type                = "FTP"
  target              = "ftp.example.com"
  label               = "tf-acc-write-only"
  enabled             = false
  username            = "tf-acc"
  password_wo         = "initial-secret"
  password_wo_version = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.pw", "username", "tf-acc"),
					resource.TestCheckResourceAttr("nodeping_check.pw", "password_wo_version", "1"),
					// Both the write-only value and the deprecated attribute
					// must be absent from state.
					resource.TestCheckNoResourceAttr("nodeping_check.pw", "password_wo"),
					resource.TestCheckNoResourceAttr("nodeping_check.pw", "password"),
				),
			},
			{
				// Bumping the version is what makes a rotation planable. The
				// state must still hold neither password afterwards.
				Config: `
resource "nodeping_check" "pw" {
  type                = "FTP"
  target              = "ftp.example.com"
  label               = "tf-acc-write-only"
  enabled             = false
  username            = "tf-acc"
  password_wo         = "rotated-secret"
  password_wo_version = 2
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.pw", "password_wo_version", "2"),
					resource.TestCheckNoResourceAttr("nodeping_check.pw", "password_wo"),
					resource.TestCheckNoResourceAttr("nodeping_check.pw", "password"),
				),
			},
			{
				ResourceName:      "nodeping_check.pw",
				ImportState:       true,
				ImportStateVerify: true,
				// password_wo_version is a client side trigger the API knows
				// nothing about, so an imported check cannot reproduce it.
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
		},
	})
}

// TestAccCheck_conflictingPasswordAttributes proves the deprecated attribute
// and its write-only replacement cannot both be set.
func TestAccCheck_conflictingPasswordAttributes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_check" "conflict" {
  type        = "FTP"
  target      = "ftp.example.com"
  enabled     = false
  password    = "legacy"
  password_wo = "modern"
}
`,
				ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Combination`),
			},
		},
	})
}

// TestAccCheck_defaultTagsMerge exercises the provider level default_tags
// merge in all three directions: added, extended, and removed. Removal is the
// case a naive Optional+Computed implementation gets wrong, because core copies
// prior state into the plan when the attribute disappears from config.
func TestAccCheck_defaultTagsMerge(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "nodeping" {
  default_tags = ["tf-acc", "managed-by-tofu"]
}

resource "nodeping_check" "tags" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "tf-acc-tags"
  enabled = false
  tags    = ["team-devops"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "3"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "tf-acc"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "managed-by-tofu"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "team-devops"),
				),
			},
			{
				// Dropping a default tag from the provider and the explicit tag
				// from the resource must actually remove them, not leave the
				// prior state in place.
				Config: `
provider "nodeping" {
  default_tags = ["tf-acc"]
}

resource "nodeping_check" "tags" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "tf-acc-tags"
  enabled = false
  tags    = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "1"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "tf-acc"),
				),
			},
			{
				// Removing default_tags entirely, with no explicit tags left,
				// must clear the set rather than resurrect the old values.
				Config: `
provider "nodeping" {}

resource "nodeping_check" "tags" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "tf-acc-tags"
  enabled = false
  tags    = []
}
`,
				Check: resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "0"),
			},
		},
	})
}

// TestAccCheck_subaccount creates a check inside a subaccount and imports it
// back with the customer_id prefixed import ID.
func TestAccCheck_subaccount(t *testing.T) {
	var customerID string

	config := func() string {
		return fmt.Sprintf(`
resource "nodeping_check" "sub" {
  customer_id = %[1]q
  type        = "HTTP"
  target      = "https://example.com"
  label       = "tf-acc-subaccount"
  enabled     = false
}
`, customerID)
	}

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			customerID = testAccSubaccountID(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_check.sub", "id"),
					resource.TestCheckResourceAttrPtr("nodeping_check.sub", "customer_id", &customerID),
					resource.TestCheckResourceAttr("nodeping_check.sub", "label", "tf-acc-subaccount"),
				),
			},
			{
				ResourceName:      "nodeping_check.sub",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["nodeping_check.sub"]
					if !ok {
						return "", fmt.Errorf("nodeping_check.sub is not in state")
					}
					return customerID + ":" + rs.Primary.ID, nil
				},
			},
		},
	})
}
