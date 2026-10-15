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

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/provider"
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
//
// The target is written in the form NodePing stores. Import cannot reproduce
// any other form: state has to match a known configuration value, so an apply
// records what the practitioner wrote while an import records what the API
// returned, and the two only coincide when the configuration is already
// canonical. TestAccCheck_targetNormalization covers the case where it is not.
func TestAccCheck_importStateVerify(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_check" "test" {
  type         = "HTTP"
  target       = "https://example.com/"
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

// TestAccCheck_targetNormalization is the regression test for the defect the
// first live acceptance run found. NodePing rewrites "https://example.com" to
// "https://example.com/", so state can hold either spelling depending on how it
// got there: an apply records what the practitioner configured, an import
// records what the API returned. Before the plan modifier existed, the second
// case planned an update against a check that was already correct, on every run
// until someone edited the configuration.
//
// Step 1 puts the canonical form in state, which is the state an import
// produces. Step 2 then presents the bare origin as configuration. A PlanOnly
// step fails on a non-empty plan, so it asserts exactly the thing that was
// broken: the two spellings settle instead of planning an update.
func TestAccCheck_targetNormalization(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_check" "normalized" {
  type     = "HTTP"
  target   = "https://example.com/"
  label    = "tf-acc-target"
  enabled  = false
  interval = 15
}
`,
				Check: resource.TestCheckResourceAttr("nodeping_check.normalized", "target", "https://example.com/"),
			},
			{
				Config: `
resource "nodeping_check" "normalized" {
  type     = "HTTP"
  target   = "https://example.com"
  label    = "tf-acc-target"
  enabled  = false
  interval = 15
}
`,
				PlanOnly: true,
			},
			{
				// A target that differs by more than NodePing's own rewriting
				// must still plan an update, or the modifier would be hiding
				// real changes.
				Config: `
resource "nodeping_check" "normalized" {
  type     = "HTTP"
  target   = "https://example.com/changed"
  label    = "tf-acc-target"
  enabled  = false
  interval = 15
}
`,
				Check: resource.TestCheckResourceAttr("nodeping_check.normalized", "target", "https://example.com/changed"),
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
				//
				// modified is NodePing's own timestamp and it keeps moving after
				// the update response is returned: this step read 1790091927959
				// on apply and 1790091929202 on the import 1.2s later. Comparing
				// it makes the step fail whenever that write lands in between.
				// It is Computed only, so a value that changes under the
				// provider produces no plan diff.
				ImportStateVerifyIgnore: []string{"password_wo_version", "modified"},
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
  target  = "https://example.com/"
  label   = "tf-acc-tags"
  enabled = false
  tags    = ["team-devops"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// tags holds only what the configuration wrote. Merging the
					// defaults into it is what produced "planned value does not
					// match config value".
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "1"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "team-devops"),
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags_all.#", "3"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags_all.*", "tf-acc"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags_all.*", "managed-by-tofu"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags_all.*", "team-devops"),
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
  target  = "https://example.com/"
  label   = "tf-acc-tags"
  enabled = false
  tags    = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "0"),
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags_all.#", "1"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags_all.*", "tf-acc"),
				),
			},
			{
				// Removing default_tags entirely, with no explicit tags left,
				// must clear the set rather than resurrect the old values.
				Config: `
provider "nodeping" {}

resource "nodeping_check" "tags" {
  type    = "HTTP"
  target  = "https://example.com/"
  label   = "tf-acc-tags"
  enabled = false
  tags    = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "0"),
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags_all.#", "0"),
				),
			},
			{
				// A tag the practitioner writes that is also a default must stay
				// in tags, or the next plan would show it being removed from a
				// check that is already correct.
				Config: `
provider "nodeping" {
  default_tags = ["tf-acc"]
}

resource "nodeping_check" "tags" {
  type    = "HTTP"
  target  = "https://example.com/"
  label   = "tf-acc-tags"
  enabled = false
  tags    = ["tf-acc", "team-devops"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags.#", "2"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "tf-acc"),
					resource.TestCheckTypeSetElemAttr("nodeping_check.tags", "tags.*", "team-devops"),
					resource.TestCheckResourceAttr("nodeping_check.tags", "tags_all.#", "2"),
				),
			},
		},
	})
}

// TestAccCheck_subaccount creates a check inside a subaccount and imports it
// back with the customer_id prefixed import ID.
//
// customerID is resolved before the TestCase is built. A Config produced from a
// variable that PreCheck fills in later is evaluated while the composite
// literal is still being constructed, which wrote `customer_id = ""` into the
// configuration and made this test report a provider fault that was not there.
func TestAccCheck_subaccount(t *testing.T) {
	customerID := testAccSubaccountID(t)

	config := fmt.Sprintf(`
resource "nodeping_check" "sub" {
  customer_id = %[1]q
  type        = "HTTP"
  target      = "https://example.com/"
  label       = "tf-acc-subaccount"
  enabled     = false
}
`, customerID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_check.sub", "id"),
					resource.TestCheckResourceAttr("nodeping_check.sub", "customer_id", customerID),
					resource.TestCheckResourceAttr("nodeping_check.sub", "label", "tf-acc-subaccount"),
				),
			},
			{
				// The attribute check above only sees what apply wrote. A
				// customer_id that disagrees with the account the API reports
				// shows up one refresh later, and customer_id forces
				// replacement, so the drift would be a destroy and recreate on
				// every plan.
				Config:   config,
				PlanOnly: true,
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
