// Acceptance tests for nodeping_schedule.
//
// The build tag exists because terraform-plugin-testing is not yet listed in
// go.mod. Once it is, drop the tag: the TF_ACC guard below is what actually
// keeps these tests from running by accident.
//
// Run with `make test-acc`, which sets TF_ACC, TF_ACC_TERRAFORM_PATH,
// TF_ACC_PROVIDER_HOST and TF_ACC_PROVIDER_NAMESPACE. Setting only
// TF_ACC_TERRAFORM_PATH is not enough, and leaving it unset makes
// plugin-testing download HashiCorp Terraform and test the wrong binary.
//
// These tests create and destroy live NodePing schedules. Point
// NODEPING_API_TOKEN at the dedicated test subaccount, never a client
// subaccount and never the main account.

package schedule_test

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
	if os.Getenv("NODEPING_API_TOKEN") == "" {
		t.Fatal("NODEPING_API_TOKEN must be set for acceptance tests")
	}
}

func testAccScheduleConfig(id, mondayEnd string, excludeSunday bool) string {
	return fmt.Sprintf(`
resource "nodeping_schedule" "test" {
  id = %q

  monday = {
    time1 = "8:00"
    time2 = %q
  }

  saturday = {
    disabled = true
  }

  sunday = {
    time1   = "0:00"
    time2   = "6:00"
    exclude = %t
  }
}
`, id, mondayEnd, excludeSunday)
}

func TestAccScheduleResource_lifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}

	scheduleID := fmt.Sprintf("tfacc-%d", os.Getpid())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleConfig(scheduleID, "18:00", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.test", "id", scheduleID),
					resource.TestCheckResourceAttr("nodeping_schedule.test", "monday.time1", "8:00"),
					resource.TestCheckResourceAttr("nodeping_schedule.test", "monday.time2", "18:00"),
					resource.TestCheckResourceAttr("nodeping_schedule.test", "saturday.disabled", "true"),
					resource.TestCheckResourceAttr("nodeping_schedule.test", "sunday.exclude", "false"),
					resource.TestCheckNoResourceAttr("nodeping_schedule.test", "tuesday.time1"),
				),
			},
			{
				// The schedules endpoint has no POST: this update is the same
				// PUT the create used.
				Config: testAccScheduleConfig(scheduleID, "17:00", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.test", "monday.time2", "17:00"),
					resource.TestCheckResourceAttr("nodeping_schedule.test", "sunday.exclude", "true"),
				),
			},
			{
				ResourceName:      "nodeping_schedule.test",
				ImportState:       true,
				ImportStateId:     scheduleID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccScheduleResource_renameForcesReplacement(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}

	first := fmt.Sprintf("tfacc-a-%d", os.Getpid())
	second := fmt.Sprintf("tfacc-b-%d", os.Getpid())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleConfig(first, "18:00", false),
				Check:  resource.TestCheckResourceAttr("nodeping_schedule.test", "id", first),
			},
			{
				// The ID is the API's primary key, so renaming destroys the old
				// schedule and creates a new one.
				Config: testAccScheduleConfig(second, "18:00", false),
				Check:  resource.TestCheckResourceAttr("nodeping_schedule.test", "id", second),
			},
		},
	})
}
