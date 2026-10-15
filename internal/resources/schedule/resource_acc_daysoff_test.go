package schedule_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccAPI(method, path string, body interface{}) ([]byte, error) {
	base := strings.TrimRight(os.Getenv("NODEPING_API_URL"), "/")
	if base == "" {
		base = "https://api.nodeping.com/api/1"
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(os.Getenv("NODEPING_API_TOKEN"), "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, out)
	}
	return out, nil
}

// checkDaysOffOnWire reads the schedule straight from NodePing and asserts each
// named day is still disabled and carries no hours.
func checkDaysOffOnWire(id string, days ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body, err := testAccAPI(http.MethodGet, "/schedules/"+url.PathEscape(id), nil)
		if err != nil {
			return err
		}
		var doc map[string]map[string]interface{}
		if err := json.Unmarshal(body, &doc); err != nil {
			return err
		}
		for _, d := range days {
			day := doc[d]
			// The provider writes schedule flags as 0/1 (ScheduleFlexBool);
			// NodePing's UI writes true/false. Either means disabled.
			if day == nil || (day["disabled"] != true && day["disabled"] != float64(1)) {
				return fmt.Errorf("%s = %v, want disabled", d, day)
			}
			for _, k := range []string{"time1", "time2"} {
				if v, ok := day[k]; ok && v != false && v != nil && v != "" {
					return fmt.Errorf("%s.%s = %v, want no hours on a day off", d, k, v)
				}
			}
		}
		return nil
	}
}

func testAccDaysOffConfig(id, mondayEnd string) string {
	return fmt.Sprintf(`
resource "nodeping_schedule" "off" {
  id = %q

  monday = {
    time1 = "8:00"
    time2 = %q
  }

  friday = {
    time1 = "8:00"
    time2 = "12:00"
  }

  saturday = {
    disabled = true
  }

  sunday = {
    disabled = true
  }
}
`, id, mondayEnd)
}

// TestAccScheduleResource_daysOff: two days off and two days with hours, then
// an update to one day's hours, which rewrites the whole schedule. The days off
// must survive the rewrite with no hours.
func TestAccScheduleResource_daysOff(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	id := fmt.Sprintf("acctest-daysoff-%d", os.Getpid())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDaysOffConfig(id, "17:00"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.off", "saturday.disabled", "true"),
					resource.TestCheckResourceAttr("nodeping_schedule.off", "sunday.disabled", "true"),
					resource.TestCheckNoResourceAttr("nodeping_schedule.off", "sunday.time1"),
					checkDaysOffOnWire(id, "saturday", "sunday"),
				),
			},
			{
				ResourceName:      "nodeping_schedule.off",
				ImportState:       true,
				ImportStateId:     id,
				ImportStateVerify: true,
			},
			{
				Config: testAccDaysOffConfig(id, "18:30"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.off", "monday.time2", "18:30"),
					resource.TestCheckResourceAttr("nodeping_schedule.off", "sunday.disabled", "true"),
					checkDaysOffOnWire(id, "saturday", "sunday"),
				),
			},
		},
	})
}

// TestAccScheduleResource_importDaysOffWrittenOutsideTofu: the NodePing UI
// stores a day off as {"time1": false, "time2": false, "disabled": true}, which
// the provider once could not decode at all. The schedule is seeded through the
// API in that shape and brought in with an import block; the step fails unless
// the plan after import is empty.
func TestAccScheduleResource_importDaysOffWrittenOutsideTofu(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	id := fmt.Sprintf("acctest-uidaysoff-%d", os.Getpid())
	t.Cleanup(func() {
		// Safety net if the test fails before tofu owns the schedule.
		_, _ = testAccAPI(http.MethodDelete, "/schedules/"+url.PathEscape(id), nil)
	})

	seed := func() {
		_, err := testAccAPI(http.MethodPut, "/schedules/"+url.PathEscape(id), map[string]interface{}{
			"data": map[string]interface{}{
				"monday":   map[string]interface{}{"time1": "08:00", "time2": "17:00", "exclude": false, "disabled": false, "allday": false},
				"saturday": map[string]interface{}{"time1": false, "time2": false, "exclude": false, "disabled": true, "allday": false},
				"sunday":   map[string]interface{}{"time1": false, "time2": false, "exclude": false, "disabled": true, "allday": false},
			},
		})
		if err != nil {
			t.Fatalf("seeding schedule: %v", err)
		}
	}

	config := fmt.Sprintf(`
import {
  to = nodeping_schedule.ui
  id = %[1]q
}

resource "nodeping_schedule" "ui" {
  id = %[1]q

  monday = {
    time1    = "08:00"
    time2    = "17:00"
    exclude  = false
    disabled = false
    allday   = false
  }

  saturday = {
    exclude  = false
    disabled = true
    allday   = false
  }

  sunday = {
    exclude  = false
    disabled = true
    allday   = false
  }
}
`, id)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: seed,
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.ui", "sunday.disabled", "true"),
					resource.TestCheckNoResourceAttr("nodeping_schedule.ui", "sunday.time1"),
					resource.TestCheckResourceAttr("nodeping_schedule.ui", "monday.time1", "08:00"),
				),
			},
			{
				// Writing the imported schedule back replaces the UI's false
				// times with no times; the days must stay off.
				Config: strings.Replace(config, `time2    = "17:00"`, `time2    = "16:00"`, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_schedule.ui", "monday.time2", "16:00"),
					checkDaysOffOnWire(id, "saturday", "sunday"),
				),
			},
		},
	})
}
