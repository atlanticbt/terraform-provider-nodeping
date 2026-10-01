package check_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Write paths the first estate import showed had only ever been exercised
// against mocks. Each creates a check, requires an empty plan afterwards, and
// round trips through import.

// NodePing returns "queue": false on a disabled check, which once made every
// disabled check unreadable. Toggling enabled exercises the read after each
// transition.
func TestAccCheck_disabledCheckToggles(t *testing.T) {
	config := func(enabled bool) string {
		return fmt.Sprintf(`
resource "nodeping_check" "test" {
  type         = "HTTP"
  target       = "https://example.com/"
  label        = "acctest-disabled-toggle"
  enabled      = %t
  interval     = 15
  runlocations = ["nam"]
}
`, enabled)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(false), Check: resource.TestCheckResourceAttr("nodeping_check.test", "enabled", "false")},
			{Config: config(false), PlanOnly: true},
			{ResourceName: "nodeping_check.test", ImportState: true, ImportStateVerify: true},
			{Config: config(true), Check: resource.TestCheckResourceAttr("nodeping_check.test", "enabled", "true")},
			{Config: config(false), Check: resource.TestCheckResourceAttr("nodeping_check.test", "enabled", "false")},
			{Config: config(false), PlanOnly: true},
		},
	})
}

// sens = 0 is what a live PUSH check holds and what "alert on the first
// failure" needs; the provider used to refuse it.
func TestAccCheck_sensZero(t *testing.T) {
	config := `
resource "nodeping_check" "test" {
  type         = "HTTP"
  target       = "https://example.com/"
  label        = "acctest-sens-zero"
  enabled      = false
  sens         = 0
  runlocations = ["nam"]
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttr("nodeping_check.test", "sens", "0")},
			{Config: config, PlanOnly: true},
			{ResourceName: "nodeping_check.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

// statuscode = 0 exists on live HTTPADV checks. The test records whether
// NodePing keeps it as 0 when the provider sends it.
func TestAccCheck_statusCodeZero(t *testing.T) {
	config := `
resource "nodeping_check" "test" {
  type         = "HTTPADV"
  target       = "https://example.com/"
  label        = "acctest-statuscode-zero"
  enabled      = false
  statuscode   = 0
  runlocations = ["nam"]
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttr("nodeping_check.test", "statuscode", "0")},
			{Config: config, PlanOnly: true},
			{ResourceName: "nodeping_check.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

// The settings the NC Courts exposure canary relies on: exact status, inverted
// content match, no redirects, no certificate verification, a custom agent.
func TestAccCheck_httpAdvCanarySettings(t *testing.T) {
	config := `
resource "nodeping_check" "test" {
  type          = "HTTPADV"
  target        = "https://example.com/"
  label         = "acctest-httpadv-canary-settings"
  enabled       = false
  method        = "GET"
  statuscode    = 403
  contentstring = "acctest-token"
  invert        = true
  regex         = false
  follow        = false
  verify        = false
  sens          = 1
  runlocations  = ["nam"]
  sendheaders = {
    User-Agent = "acctest-monitor"
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
					resource.TestCheckResourceAttr("nodeping_check.test", "verify", "false"),
					resource.TestCheckResourceAttr("nodeping_check.test", "follow", "false"),
					resource.TestCheckResourceAttr("nodeping_check.test", "invert", "true"),
					resource.TestCheckResourceAttr("nodeping_check.test", "sendheaders.User-Agent", "acctest-monitor"),
				),
			},
			{Config: config, PlanOnly: true},
			{ResourceName: "nodeping_check.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

// Every check type no other acceptance test creates. Each gets a minimal
// configuration against a public or unreachable target with no notifications,
// is left disabled, must plan empty after apply, and must survive an import.
// NodePing refuses localhost targets, so database types point at
// db.example.com. WHOIS and RDAP leave interval unset, which covers the plan
// default of 1440 NodePing applies below their minimums.
func TestAccCheck_everyType(t *testing.T) {
	fixture := os.Getenv("NODEPING_TEST_CHECK_ID")
	cases := map[string]string{
		"AGENT":       ``,
		"AUDIO":       `target = "https://example.com/stream.mp3"`,
		"CLUSTER":     fmt.Sprintf(`data = { %q = "1" }`, fixture),
		"DOHDOT":      `target = "cloudflare-dns.com"` + "\n" + `dohdot = "doh"` + "\n" + `dnstype = "A"` + "\n" + `dnstoresolve = "example.com"`,
		"DNS":         `target = "1.1.1.1"` + "\n" + `dnstype = "A"` + "\n" + `dnstoresolve = "example.com"`,
		"HTTPCONTENT": `target = "https://example.com/"` + "\n" + `contentstring = "Example Domain"`,
		"HTTPPARSE":   `target = "https://example.com/"` + "\n" + `fields = { status = { name = "status", min = 0, max = 1 } }`,
		"HTTPADV":     `target = "https://example.com/"`,
		"IMAP4":       `target = "imap.example.com"` + "\n" + `port = 993` + "\n" + `secure = "ssl"`,
		"MONGODB":     `target = "mongodb://db.example.com:27017/acctest"`,
		"MTR":         `target = "example.com"`,
		"MYSQL":       `target = "db.example.com"` + "\n" + `port = 3306`,
		"NTP":         `target = "pool.ntp.org"`,
		"PGSQL":       `target = "postgresql://db.example.com:5432/acctest"`,
		"PING":        `target = "1.1.1.1"`,
		"POP3":        `target = "pop.example.com"` + "\n" + `port = 995` + "\n" + `secure = "ssl"`,
		"PORT":        `target = "example.com"` + "\n" + `port = 443`,
		"PUSH":        `fields = { acctest = { name = "acctest", min = 0, max = 1 } }`,
		"RBL":         `target = "1.1.1.1"`,
		"RDAP":        `target = "example.com"` + "\n" + `rdapurl = "https://rdap.verisign.com/com/v1/"`,
		"RDP":         `target = "example.com"` + "\n" + `port = 3389`,
		"REDIS":       `target = "redis://db.example.com:6379"` + "\n" + `redistype = "standalone"`,
		"SIP":         `target = "sip:example.com"` + "\n" + `port = 5060`,
		"SMTP":        `target = "smtp.example.com"` + "\n" + `port = 25`,
		"SNMP":        `target = "example.com"` + "\n" + `snmpv = "2c"` + "\n" + `fields = { "1.3.6.1.2.1.1.3.0" = { name = "uptime" } }`,
		"SPEC10DNS":   `target = "example.com"`,
		"SPEC10RDDS":  `target = "example.com"`,
		"SSH":         `target = "example.com"` + "\n" + `port = 22`,
		"SSL":         `target = "https://example.com/"`,
		"WEBSOCKET":   `target = "wss://example.com/"`,
		"WHOIS":       `target = "example.com"`,
	}
	only := os.Getenv("NODEPING_ACC_TYPES")
	for checkType, extra := range cases {
		if only != "" && !strings.Contains(","+only+",", ","+checkType+",") {
			continue
		}
		t.Run(checkType, func(t *testing.T) {
			config := fmt.Sprintf(`
resource "nodeping_check" "test" {
  type         = %q
  label        = "acctest-type-%s"
  enabled      = false
  runlocations = ["nam"]
  %s
}
`, checkType, strings.ToLower(checkType), strings.ReplaceAll(extra, "\n", "\n  "))
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config},
					{ResourceName: "nodeping_check.test", ImportState: true, ImportStateVerify: true},
				},
			})
		})
	}
}
