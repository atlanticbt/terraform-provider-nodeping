package contacts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *ContactsDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ContactsDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func sampleContacts() map[string]client.Contact {
	return map[string]client.Contact{
		"C2": {
			ID:         "C2",
			CustomerID: "CUST1",
			Name:       "Second",
			CustRole:   "notify",
			Addresses: map[string]client.ContactAddress{
				"ZZ": {Type: "email", Address: "z@example.com"},
				"AA": {Type: "sms", Address: "+15551230000"},
			},
		},
		"C1": {
			ID:         "C1",
			CustomerID: "CUST1",
			Name:       "First",
			CustRole:   "notify",
		},
	}
}

func TestMapContactsIsSortedAndDeterministic(t *testing.T) {
	contacts := sampleContacts()

	for i := 0; i < 200; i++ {
		got := mapContacts(contacts)
		if len(got) != 2 {
			t.Fatalf("got %d contacts", len(got))
		}
		if got[0].ID.ValueString() != "C1" || got[1].ID.ValueString() != "C2" {
			t.Fatalf("iteration %d: contacts out of order", i)
		}
		if got[1].Addresses[0].ID.ValueString() != "AA" || got[1].Addresses[1].ID.ValueString() != "ZZ" {
			t.Fatalf("iteration %d: addresses out of order", i)
		}
	}

	if len(mapContacts(nil)) != 0 {
		t.Fatal("expected an empty list")
	}
}

func TestMapContactsFallsBackToMapKey(t *testing.T) {
	got := mapContacts(map[string]client.Contact{"KEY1": {Name: "n"}})
	if got[0].ID.ValueString() != "KEY1" {
		t.Fatalf("id = %q", got[0].ID.ValueString())
	}
}

func TestMetadataAndConfigure(t *testing.T) {
	ctx := context.Background()
	d := NewContactsDataSource().(*ContactsDataSource)

	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_contacts" {
		t.Fatalf("type name = %q", metaResp.TypeName)
	}

	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if d.client != nil {
		t.Fatal("expected nil client")
	}

	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for the wrong provider data type")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || d.client != c {
		t.Fatal("expected the client to be configured")
	}
}

func readState(t *testing.T, ctx context.Context, d *ContactsDataSource) (*datasource.ReadResponse, datasource.SchemaResponse) {
	t.Helper()
	schemaResp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp, schemaResp
}

func TestRead(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := json.Marshal(sampleContacts())
		_, _ = w.Write(body)
	})

	resp, _ := readState(t, ctx, d)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ContactsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if len(got.Contacts) != 2 || got.Contacts[0].ID.ValueString() != "C1" {
		t.Fatalf("unexpected state: %+v", got.Contacts)
	}
}

func TestReadError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	resp, _ := readState(t, ctx, d)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
