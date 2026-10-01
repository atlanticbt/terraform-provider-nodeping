package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shapes NodePing returns for contacts that the provider could not decode,
// each taken from a live account (values replaced).

// GET /contacts/:id returns a login contact's addresses as an array of
// {address, status}, with no address IDs or types, while GET /contacts keys the
// same address by the address itself.
func TestGetContact_LoginContactWithAddressArrayDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"_id":"201205050153W2Q4C-X9MA3","type":"contact","customer_id":"201205050153W2Q4C","name":"Login","custrole":"edit","addresses":[{"address":"login@example.com","status":"new"}]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "t", BaseURL: server.URL})
	got, err := c.GetContact(context.Background(), "201205050153W2Q4C-X9MA3")
	if err != nil {
		t.Fatalf("should decode addresses returned as an array: %v", err)
	}
	addr, ok := got.Addresses["login@example.com"]
	if !ok || len(got.Addresses) != 1 {
		t.Fatalf("addresses = %+v, want one keyed by its address", got.Addresses)
	}
	if addr.Address != "login@example.com" || addr.Type != "" {
		t.Errorf("address decoded as %+v", addr)
	}
}

func TestContactAddresses_DecodesMapAndRejectsOtherShapes(t *testing.T) {
	var m ContactAddresses
	if err := json.Unmarshal([]byte(`{"VMVVD":{"address":"owner@example.com","status":"new"}}`), &m); err != nil {
		t.Fatalf("map form: %v", err)
	}
	if m["VMVVD"].Address != "owner@example.com" {
		t.Errorf("map form decoded as %+v", m)
	}
	for _, body := range []string{`"x"`, `12`, `[{"status":"new"}]`, `[1]`} {
		var bad ContactAddresses
		if err := json.Unmarshal([]byte(body), &bad); err == nil {
			t.Errorf("%s: expected an error", body)
		}
	}
	var empty ContactAddresses
	if err := json.Unmarshal([]byte(`null`), &empty); err != nil || empty != nil {
		t.Errorf("null: got %v, %v", empty, err)
	}
}

// An OpsGenie integration stores a custom field as an object inside the
// webhook's query strings. It must survive a read followed by a write
// unchanged, or an update would replace the object with a string.
func TestFlexStringMap_ObjectValueRoundTrips(t *testing.T) {
	in := `{"apiKey":"k","customerId":{"title":"{customer}","value":"{customer}"}}`
	var m FlexStringMap
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("should decode an object value: %v", err)
	}
	if m["apiKey"] != "k" || !strings.HasPrefix(m["customerId"], "{") {
		t.Fatalf("decoded as %+v", m)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var want, got interface{}
	if err := json.Unmarshal([]byte(in), &want); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wb, _ := json.Marshal(want)
	gb, _ := json.Marshal(got)
	if string(wb) != string(gb) {
		t.Errorf("round trip = %s, want %s", gb, wb)
	}
}

func TestFlexStringMap_PlainStringsStayStrings(t *testing.T) {
	out, err := json.Marshal(FlexStringMap{"a": "plain", "n": "12", "b": "true", "s": "[not json"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for k, v := range got {
		if _, isString := v.(string); !isString {
			t.Errorf("%s encoded as %T, want string", k, v)
		}
	}
}
