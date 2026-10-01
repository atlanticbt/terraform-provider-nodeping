package contact_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccAPIBase() string {
	if u := os.Getenv("NODEPING_API_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://api.nodeping.com/api/1"
}

// testAccRawContact fetches a contact as NodePing stores it, bypassing the
// provider's decoding, so a test can see whether a value is an object or a
// string on the wire.
func testAccRawContact(id string) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, testAccAPIBase()+"/contacts/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(os.Getenv("NODEPING_API_TOKEN"), "")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET contact %s: HTTP %d", id, resp.StatusCode)
	}
	var doc map[string]interface{}
	return doc, json.Unmarshal(body, &doc)
}

// checkStoredAsObject asserts that the contact's only address holds the named
// entry of the given map ("querystrings" or "headers") as a JSON object with
// the expected fields, not as a string.
func checkStoredAsObject(resourceName, mapName, key string, want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		doc, err := testAccRawContact(rs.Primary.ID)
		if err != nil {
			return err
		}
		addrs, ok := doc["addresses"].(map[string]interface{})
		if !ok || len(addrs) != 1 {
			return fmt.Errorf("addresses = %#v, want one address in a map", doc["addresses"])
		}
		for _, a := range addrs {
			m, _ := a.(map[string]interface{})[mapName].(map[string]interface{})
			obj, isObject := m[key].(map[string]interface{})
			if !isObject {
				return fmt.Errorf("%s.%s stored as %T (%v), want a JSON object", mapName, key, m[key], m[key])
			}
			for k, v := range want {
				if obj[k] != v {
					return fmt.Errorf("%s.%s.%s = %v, want %q", mapName, key, k, obj[k], v)
				}
			}
		}
		return nil
	}
}

func testAccObjectValuedWebhook(suppressUp bool) string {
	return fmt.Sprintf(`
resource "nodeping_contact" "obj" {
  name     = "acctest-object-valued-webhook"
  custrole = "notify"

  address {
    type        = "webhook"
    address     = "https://example.com/hook"
    action      = "post"
    suppress_up = %t

    querystrings = {
      source     = "nodeping"
      customerId = jsonencode({ value = "{customer_id}", title = "{customer_id}" })
    }

    headers = {
      Content-Type = "application/json"
      X-Acc-Meta   = jsonencode({ run = "acctest" })
    }
  }
}
`, suppressUp)
}

// TestAccContact_objectValuedWebhookMaps covers webhook query strings and
// headers whose values are JSON objects, the way an OpsGenie integration stores
// a custom field. The object must reach NodePing as an object on create and
// stay one after an unrelated update, which rewrites the whole address.
func TestAccContact_objectValuedWebhookMaps(t *testing.T) {
	qsWant := map[string]string{"value": "{customer_id}", "title": "{customer_id}"}
	hdrWant := map[string]string{"run": "acctest"}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccObjectValuedWebhook(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.obj", "address.0.querystrings.source", "nodeping"),
					checkStoredAsObject("nodeping_contact.obj", "querystrings", "customerId", qsWant),
					checkStoredAsObject("nodeping_contact.obj", "headers", "X-Acc-Meta", hdrWant),
				),
			},
			{
				ResourceName:      "nodeping_contact.obj",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccObjectValuedWebhook(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nodeping_contact.obj", "address.0.suppress_up", "true"),
					checkStoredAsObject("nodeping_contact.obj", "querystrings", "customerId", qsWant),
					checkStoredAsObject("nodeping_contact.obj", "headers", "X-Acc-Meta", hdrWant),
				),
			},
		},
	})
}

// TestAccContact_newAddressWithoutTypeRejected: type is Optional so owner and
// login addresses (stored without one) can be imported, but NodePing cannot
// create an address without a type, so the provider refuses it before calling
// the API.
func TestAccContact_newAddressWithoutTypeRejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoContactNamed("acctest-missing-type"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "nodeping_contact" "notype" {
  name     = "acctest-missing-type"
  custrole = "notify"

  address {
    address = "acctest-missing-type@example.com"
  }
}
`,
				ExpectError: regexp.MustCompile(`Missing Address Type`),
			},
		},
	})
}

// testAccCheckNoContactNamed confirms a rejected create left nothing behind.
func testAccCheckNoContactNamed(name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		req, err := http.NewRequest(http.MethodGet, testAccAPIBase()+"/contacts", nil)
		if err != nil {
			return err
		}
		req.SetBasicAuth(os.Getenv("NODEPING_API_TOKEN"), "")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var all map[string]map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
			return err
		}
		for id, c := range all {
			if c["name"] == name {
				return fmt.Errorf("contact %s named %q exists after a rejected create", id, name)
			}
		}
		return nil
	}
}
