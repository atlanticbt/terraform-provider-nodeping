package contact_test

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/provider"
)

// Acceptance tests run against a dedicated NodePing test subaccount and create
// and destroy live contacts. They are skipped unless TF_ACC is set, and `make
// test-acc` is the only supported way to run them: it points
// TF_ACC_TERRAFORM_PATH at the tofu binary and sets the two provider host
// variables that plugin-testing otherwise fills in with Terraform defaults
// OpenTofu rejects.
//
// Every contact created here uses custrole "notify". NodePing emails login
// credentials to a contact created with "edit" or "view", so those roles must
// never be exercised against an address that could reach a real mailbox. The
// addresses below are all example.com/example.net and the phone numbers come
// from the reserved +1555555xxxx range.
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

// TestAccContact_lifecycle covers create, import round trip, update and a
// second import. The import steps are the strongest assertion available: they
// compare every attribute apply wrote against the one the provider derives
// from a bare API response, so a field the API drops or renames shows up here
// rather than as drift on a practitioner's next plan.
//
// The address list is verified through ImportStateCheck rather than
// ImportStateVerify because its order is not reproducible: NodePing returns a
// map of addresses, so an import lists them in ID order while an apply lists
// them in configuration order. ImportStateVerify still covers everything else.
func TestAccContact_lifecycle(t *testing.T) {
	var created addressAttrsByAddress
	var updated addressAttrsByAddress

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_contact" "acc" {
  name     = "tfacc contact"
  custrole = "notify"

  address {
    type    = "email"
    address = "tfacc-lifecycle@example.com"
  }

  address {
    type    = "sms"
    address = "+15555550101"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_contact.acc", "id"),
					resource.TestCheckResourceAttrSet("nodeping_contact.acc", "customer_id"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "name", "tfacc contact"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "custrole", "notify"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.#", "2"),
					resource.TestCheckResourceAttrSet("nodeping_contact.acc", "address.0.id"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.0.address", "tfacc-lifecycle@example.com"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.1.type", "sms"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.1.mute", "false"),
					captureAddressAttrs("nodeping_contact.acc", &created),
				),
			},
			{
				ResourceName:            "nodeping_contact.acc",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"address"},
				ImportStateCheck:        checkImportedAddresses(&created),
			},
			{
				// Renaming, editing an address in place and muting one exercise
				// the three things the update path can get wrong at once.
				Config: `
resource "nodeping_contact" "acc" {
  name     = "tfacc contact renamed"
  custrole = "notify"

  address {
    type    = "email"
    address = "tfacc-lifecycle-edited@example.com"
  }

  address {
    type    = "sms"
    address = "+15555550101"
    mute    = true
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.acc", "name", "tfacc contact renamed"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.#", "2"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.0.address", "tfacc-lifecycle-edited@example.com"),
					resource.TestCheckResourceAttr("nodeping_contact.acc", "address.1.mute", "true"),
					captureAddressAttrs("nodeping_contact.acc", &updated),
				),
			},
			{
				ResourceName:            "nodeping_contact.acc",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"address"},
				ImportStateCheck:        checkImportedAddresses(&updated),
			},
		},
	})
}

// addressIDsByAddress records the server-assigned address ID of every address
// block, keyed by the address value itself rather than by list position.
type addressIDsByAddress map[string]string

// addressAttrsByAddress holds every attribute of every address block, keyed by
// the address value. It is the order-independent form of the address list.
type addressAttrsByAddress map[string]map[string]string

// readAddressAttrs pulls the address blocks out of a flatmap attribute set.
//
// The list index is discarded deliberately. NodePing returns the addresses as a
// map, so an imported contact lists them in ID order while an applied one lists
// them in configuration order; comparing by position would make these tests
// pass or fail on which random IDs NodePing happened to hand out.
func readAddressAttrs(attributes map[string]string) (addressAttrsByAddress, error) {
	count, err := strconv.Atoi(attributes["address.#"])
	if err != nil {
		return nil, fmt.Errorf("reading address count: %w", err)
	}

	byIndex := make([]map[string]string, count)
	for i := range byIndex {
		byIndex[i] = make(map[string]string)
	}

	for key, value := range attributes {
		if !strings.HasPrefix(key, "address.") || key == "address.#" {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(key, "address."), ".", 2)
		if len(parts) != 2 {
			continue
		}
		index, err := strconv.Atoi(parts[0])
		if err != nil || index >= count {
			return nil, fmt.Errorf("unexpected address attribute %q", key)
		}
		byIndex[index][parts[1]] = value
	}

	result := make(addressAttrsByAddress, count)
	for i, attrs := range byIndex {
		address := attrs["address"]
		if address == "" {
			return nil, fmt.Errorf("address block %d has no address value", i)
		}
		if _, duplicate := result[address]; duplicate {
			return nil, fmt.Errorf("address %q appears more than once", address)
		}
		result[address] = attrs
	}
	return result, nil
}

// captureAddressAttrs records every address block so an import step can be
// compared against it without depending on list order.
func captureAddressAttrs(resourceName string, into *addressAttrsByAddress) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s is not in state", resourceName)
		}
		captured, err := readAddressAttrs(rs.Primary.Attributes)
		if err != nil {
			return fmt.Errorf("reading %s: %w", resourceName, err)
		}
		*into = captured
		return nil
	}
}

// checkImportedAddresses is the order-independent counterpart to
// ImportStateVerify for the address list: every block the apply wrote must come
// back from the API with identical attributes, including its ID.
func checkImportedAddresses(previous *addressAttrsByAddress) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("expected 1 imported instance, got %d", len(states))
		}
		imported, err := readAddressAttrs(states[0].Attributes)
		if err != nil {
			return fmt.Errorf("reading imported state: %w", err)
		}
		if !reflect.DeepEqual(map[string]map[string]string(imported), map[string]map[string]string(*previous)) {
			return fmt.Errorf("imported addresses differ from the applied ones:\napplied:  %v\nimported: %v", *previous, imported)
		}
		return nil
	}
}

// captureAddressIDs reads the address IDs out of state so a later step can
// assert they did not move.
func captureAddressIDs(resourceName string, into *addressIDsByAddress) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s is not in state", resourceName)
		}

		count, err := strconv.Atoi(rs.Primary.Attributes["address.#"])
		if err != nil {
			return fmt.Errorf("reading address count for %s: %w", resourceName, err)
		}

		captured := make(addressIDsByAddress, count)
		for i := 0; i < count; i++ {
			address := rs.Primary.Attributes[fmt.Sprintf("address.%d.address", i)]
			id := rs.Primary.Attributes[fmt.Sprintf("address.%d.id", i)]
			if id == "" {
				return fmt.Errorf("address %q of %s has no id", address, resourceName)
			}
			captured[address] = id
		}

		*into = captured
		return nil
	}
}

// checkAddressIDsUnchanged fails when any of the named addresses has been
// re-issued a different server-assigned ID. A changed address ID breaks every
// contact group and notification that points at it, silently and without
// anything in the plan saying so.
func checkAddressIDsUnchanged(resourceName string, previous *addressIDsByAddress, addresses ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var current addressIDsByAddress
		if err := captureAddressIDs(resourceName, &current)(s); err != nil {
			return err
		}

		for _, address := range addresses {
			want, ok := (*previous)[address]
			if !ok {
				return fmt.Errorf("address %q was not captured in the previous step", address)
			}
			got, ok := current[address]
			if !ok {
				return fmt.Errorf("address %q is missing from %s", address, resourceName)
			}
			if got != want {
				return fmt.Errorf("address %q was re-issued: id was %q, now %q", address, want, got)
			}
		}
		return nil
	}
}

// checkAddressIDsDistinct fails when two address blocks share an ID, which is
// what a positional match produces when blocks are added or removed.
func checkAddressIDsDistinct(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var current addressIDsByAddress
		if err := captureAddressIDs(resourceName, &current)(s); err != nil {
			return err
		}

		seen := make(map[string]string, len(current))
		for address, id := range current {
			if other, clash := seen[id]; clash {
				return fmt.Errorf("addresses %q and %q share id %q", other, address, id)
			}
			seen[id] = address
		}
		return nil
	}
}

const (
	testAccAddressA   = "tfacc-ids-a@example.com"
	testAccAddressB   = "tfacc-ids-b@example.com"
	testAccAddressC   = "tfacc-ids-c@example.com"
	testAccAddressSMS = "+15555550110"
)

func testAccContactAddressBlocks(blocks string) string {
	return fmt.Sprintf(`
resource "nodeping_contact" "ids" {
  name     = "tfacc contact address ids"
  custrole = "notify"
%s
}
`, blocks)
}

func testAccEmailBlock(address string) string {
	return fmt.Sprintf(`
  address {
    type    = "email"
    address = %q
  }
`, address)
}

func testAccSMSBlock(address string) string {
	return fmt.Sprintf(`
  address {
    type    = "sms"
    address = %q
  }
`, address)
}

// TestAccContact_addressIDStability is what AddressIDPlanModifier exists for.
// Terraform core proposes computed values for list elements by position, so
// removing, reordering or inserting an address block makes the next block
// inherit an unrelated address's ID unless the provider re-derives it by
// content. Against the live API that shows up as the wrong NodePing address
// being overwritten, which nothing in the plan or the state warns about.
//
// Each step asserts both halves: the addresses that did not change keep the ID
// NodePing gave them, and no two blocks end up claiming the same ID.
func TestAccContact_addressIDStability(t *testing.T) {
	var initial addressIDsByAddress
	var afterRemoval addressIDsByAddress
	var final addressAttrsByAddress

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccContactAddressBlocks(
					testAccEmailBlock(testAccAddressA) +
						testAccEmailBlock(testAccAddressB) +
						testAccSMSBlock(testAccAddressSMS),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.ids", "address.#", "3"),
					checkAddressIDsDistinct("nodeping_contact.ids"),
					captureAddressIDs("nodeping_contact.ids", &initial),
				),
			},
			{
				// Remove the middle address and reorder the survivors. A
				// positional match would hand the SMS address the ID of the
				// email that used to sit in slot 0.
				Config: testAccContactAddressBlocks(
					testAccSMSBlock(testAccAddressSMS) +
						testAccEmailBlock(testAccAddressA),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.ids", "address.#", "2"),
					resource.TestCheckResourceAttr("nodeping_contact.ids", "address.0.address", testAccAddressSMS),
					resource.TestCheckResourceAttr("nodeping_contact.ids", "address.1.address", testAccAddressA),
					checkAddressIDsDistinct("nodeping_contact.ids"),
					checkAddressIDsUnchanged("nodeping_contact.ids", &initial, testAccAddressA, testAccAddressSMS),
					captureAddressIDs("nodeping_contact.ids", &afterRemoval),
				),
			},
			{
				// Insert a new address ahead of both survivors. The new block
				// must get a fresh ID and the two existing ones must keep
				// theirs.
				Config: testAccContactAddressBlocks(
					testAccEmailBlock(testAccAddressC) +
						testAccSMSBlock(testAccAddressSMS) +
						testAccEmailBlock(testAccAddressA),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.ids", "address.#", "3"),
					checkAddressIDsDistinct("nodeping_contact.ids"),
					checkAddressIDsUnchanged("nodeping_contact.ids", &afterRemoval, testAccAddressA, testAccAddressSMS),
					captureAddressAttrs("nodeping_contact.ids", &final),
				),
			},
			{
				// The IDs the provider put in state must be the ones NodePing
				// actually holds, not merely self-consistent across steps.
				ResourceName:            "nodeping_contact.ids",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"address"},
				ImportStateCheck:        checkImportedAddresses(&final),
			},
		},
	})
}

// testAccWebhookPretty is the body as a practitioner would write it: indented,
// keys in the order that reads well.
const testAccWebhookPretty = `
resource "nodeping_contact" "hook" {
  name     = "tfacc contact webhook"
  custrole = "notify"

  address {
    type    = "webhook"
    address = "https://hooks.example.net/tfacc"
    action  = "post"

    headers = {
      "Content-Type"  = "application/json"
      "X-Acc-Token"   = "tfacc-header-value"
    }

    querystrings = {
      "source" = "nodeping"
      "env"    = "tfacc"
    }

    data = <<-EOT
      {
        "title": "NodePing",
        "text": "Host {label} : {type} is {event}"
      }
    EOT
  }
}
`

// testAccWebhookCompact carries the same JSON document written on one line with
// the keys in the other order. Nothing about the contact has changed, so the
// plan must be empty.
const testAccWebhookCompact = `
resource "nodeping_contact" "hook" {
  name     = "tfacc contact webhook"
  custrole = "notify"

  address {
    type    = "webhook"
    address = "https://hooks.example.net/tfacc"
    action  = "post"

    headers = {
      "Content-Type"  = "application/json"
      "X-Acc-Token"   = "tfacc-header-value"
    }

    querystrings = {
      "source" = "nodeping"
      "env"    = "tfacc"
    }

    data = "{\"text\":\"Host {label} : {type} is {event}\",\"title\":\"NodePing\"}"
  }
}
`

// TestAccContact_webhookDataSemanticEquality is the live exercise of the
// webhook data phantom-diff fix, which until then had only unit tests behind it. Two pieces are needed and
// each covers a different call: semantic equality on the value type settles the
// refresh and apply-consistency paths, and JSONSemanticPlanModifier settles
// PlanResourceChange, which is the only one that runs after an import.
//
// headers and querystrings ride along. They were deliberately left
// non-sensitive (A-015) because generated configuration writes null
// for a sensitive attribute, which made an imported webhook contact plan to
// delete them; ImportStateVerify is what proves that still holds.
func TestAccContact_webhookDataSemanticEquality(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookPretty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.#", "1"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.type", "webhook"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.action", "post"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.headers.Content-Type", "application/json"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.headers.X-Acc-Token", "tfacc-header-value"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.querystrings.source", "nodeping"),
					resource.TestCheckResourceAttr("nodeping_contact.hook", "address.0.querystrings.env", "tfacc"),
				),
			},
			{
				// Refresh path: state holds the indented document, the
				// configuration presents the compact one.
				Config:   testAccWebhookCompact,
				PlanOnly: true,
			},
			{
				ResourceName:      "nodeping_contact.hook",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Import path: an import block plans the resource in from the
				// API with none of the configuration's formatting carried over,
				// and the step fails unless that plan is a no-op. This is the
				// plan that produced a permanent "whitespace changes" diff
				// before JSONSemanticPlanModifier existed.
				Config:          testAccWebhookCompact,
				ResourceName:    "nodeping_contact.hook",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
			},
		},
	})
}

// TestAccContact_addressSetReplacement is the regression test for the second
// defect the live run found. NodePing treats the `addresses` map as the whole
// set and deletes anything missing from it, but the provider dropped the map
// from the request whenever it was empty. Swapping the only address for one of
// a different type left the old address in place next to the new one, and the
// apply died on "block count changed from 1 to 2".
//
// The last step covers the other end of the same behaviour: NodePing will not
// let a contact lose its last address, which the provider now says plainly
// instead of trying and reporting itself as buggy afterwards.
func TestAccContact_addressSetReplacement(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_contact" "swap" {
  name     = "tfacc contact address swap"
  custrole = "notify"

  address {
    type    = "email"
    address = "tfacc-swap@example.com"
  }
}
`,
				Check: resource.TestCheckResourceAttr("nodeping_contact.swap", "address.#", "1"),
			},
			{
				// A different type means no content, ID or positional match, so
				// the whole set moves to newaddresses and the map goes up empty.
				Config: `
resource "nodeping_contact" "swap" {
  name     = "tfacc contact address swap"
  custrole = "notify"

  address {
    type    = "sms"
    address = "+15555550120"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.swap", "address.#", "1"),
					resource.TestCheckResourceAttr("nodeping_contact.swap", "address.0.type", "sms"),
					resource.TestCheckResourceAttr("nodeping_contact.swap", "address.0.address", "+15555550120"),
				),
			},
			{
				Config: `
resource "nodeping_contact" "swap" {
  name     = "tfacc contact address swap"
  custrole = "notify"
}
`,
				ExpectError: regexp.MustCompile(`(?s)Cannot Remove Every Contact Address`),
			},
		},
	})
}

// TestAccContact_withoutName is the regression test for the third defect. A
// contact created with no name comes back from NodePing with no name field, and
// the provider wrote the resulting empty string into an Optional, non-Computed
// attribute. Every refresh then planned an update from null to "" that the
// apply could not settle, so the resource never reached a steady state.
//
// The PlanOnly step is the assertion: it fails on any non-empty plan, including
// the one produced by the refresh that precedes it.
func TestAccContact_withoutName(t *testing.T) {
	const config = `
resource "nodeping_contact" "anon" {
  custrole = "notify"

  address {
    type    = "email"
    address = "tfacc-no-name@example.com"
  }
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_contact.anon", "id"),
					resource.TestCheckNoResourceAttr("nodeping_contact.anon", "name"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      "nodeping_contact.anon",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccContact_suppressDiagRejected is the regression test for the defect
// this file found on its first live run. NodePing accepts suppressdiag on a
// contact address, discards it, and never returns it, so the provider planned
// true and wrote back false: OpenTofu aborted with "Provider produced
// inconsistent result after apply ... This is a bug in the provider". The
// validator now stops it at plan time and says what is actually wrong.
//
// suppress_all is asserted alongside it so the test would fail if the
// rejection were ever widened to the sibling flags, which NodePing does store.
func TestAccContact_suppressDiagRejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_contact" "diag" {
  name     = "tfacc contact suppress diag"
  custrole = "notify"

  address {
    type          = "email"
    address       = "tfacc-suppress-diag@example.com"
    suppress_diag = true
  }
}
`,
				ExpectError: regexp.MustCompile(`(?s)NodePing does not store suppress_diag`),
			},
			{
				Config: `
resource "nodeping_contact" "diag" {
  name     = "tfacc contact suppress diag"
  custrole = "notify"

  address {
    type          = "email"
    address       = "tfacc-suppress-diag@example.com"
    suppress_diag = false
    suppress_all  = true
    suppress_up   = true
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.diag", "address.0.suppress_diag", "false"),
					resource.TestCheckResourceAttr("nodeping_contact.diag", "address.0.suppress_all", "true"),
					resource.TestCheckResourceAttr("nodeping_contact.diag", "address.0.suppress_up", "true"),
				),
			},
		},
	})
}

// TestAccContact_subaccount creates a contact with an explicit customer_id and
// imports it back with the customer_id prefixed import ID.
//
// customerID is resolved before the TestCase is built. A Config produced from a
// variable that PreCheck fills in later is evaluated while the composite
// literal is still being constructed, which writes `customer_id = ""` into the
// configuration; that is what made the equivalent check test report a provider
// fault that was not there.
func TestAccContact_subaccount(t *testing.T) {
	customerID := testAccSubaccountID(t)

	config := fmt.Sprintf(`
resource "nodeping_contact" "sub" {
  customer_id = %[1]q
  name        = "tfacc contact subaccount"
  custrole    = "notify"

  address {
    type    = "email"
    address = "tfacc-subaccount@example.com"
  }
}
`, customerID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nodeping_contact.sub", "id"),
					resource.TestCheckResourceAttr("nodeping_contact.sub", "customer_id", customerID),
					resource.TestCheckResourceAttr("nodeping_contact.sub", "name", "tfacc contact subaccount"),
				),
			},
			{
				// customer_id forces replacement, so a value that disagrees
				// with the account the API reports would be a destroy and
				// recreate on every plan. Only a refresh surfaces that.
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      "nodeping_contact.sub",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["nodeping_contact.sub"]
					if !ok {
						return "", fmt.Errorf("nodeping_contact.sub is not in state")
					}
					return customerID + ":" + rs.Primary.ID, nil
				},
			},
		},
	})
}
