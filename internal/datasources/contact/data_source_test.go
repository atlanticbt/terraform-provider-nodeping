package contact

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// testConfig builds a datasource config holding the given model. tfsdk.Config has
// no Set method, so the raw value is produced through a tfsdk.State first.
func testConfig(t *testing.T, ctx context.Context, model ContactDataSourceModel) tfsdk.Config {
	t.Helper()
	d := &ContactDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}

	return tfsdk.Config{Schema: resp.Schema, Raw: state.Raw}
}

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *ContactDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ContactDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func TestMapAddressesIsSortedAndDeterministic(t *testing.T) {
	api := map[string]client.ContactAddress{
		"C3": {Type: "email", Address: "c@example.com"},
		"A1": {Type: "email", Address: "a@example.com", SuppressUp: true},
		"B2": {Type: "sms", Address: "+15551230000"},
	}

	for i := 0; i < 200; i++ {
		got := mapAddresses(api)
		if len(got) != 3 {
			t.Fatalf("got %d addresses", len(got))
		}
		if got[0].ID.ValueString() != "A1" || got[1].ID.ValueString() != "B2" || got[2].ID.ValueString() != "C3" {
			t.Fatalf("iteration %d: unsorted addresses: %v %v %v",
				i, got[0].ID.ValueString(), got[1].ID.ValueString(), got[2].ID.ValueString())
		}
	}

	got := mapAddresses(api)
	if !got[0].SuppressUp.ValueBool() || got[0].Address.ValueString() != "a@example.com" {
		t.Fatalf("unexpected mapping: %+v", got[0])
	}
	if len(mapAddresses(nil)) != 0 {
		t.Fatal("expected an empty list for no addresses")
	}
}

func TestDataSourceMetadataAndConfigure(t *testing.T) {
	ctx := context.Background()
	d := NewContactDataSource().(*ContactDataSource)

	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_contact" {
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

func TestDataSourceRead(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := json.Marshal(client.Contact{
			ID:         "C1",
			CustomerID: "CUST1",
			Name:       "Example",
			CustRole:   "notify",
			Addresses: map[string]client.ContactAddress{
				"B2": {Type: "sms", Address: "+15551230000"},
				"A1": {Type: "email", Address: "a@example.com"},
			},
		})
		_, _ = w.Write(body)
	})

	config := testConfig(t, ctx, ContactDataSourceModel{ID: types.StringValue("C1")})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: config.Schema, Raw: tftypes.NewValue(config.Schema.Type().TerraformType(ctx), nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ContactDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if got.Name.ValueString() != "Example" || got.CustomerID.ValueString() != "CUST1" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got.Addresses[0].ID.ValueString() != "A1" || got.Addresses[1].ID.ValueString() != "B2" {
		t.Fatal("addresses are not sorted by ID")
	}
}

func TestDataSourceReadError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	config := testConfig(t, ctx, ContactDataSourceModel{ID: types.StringValue("C1")})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: config.Schema, Raw: tftypes.NewValue(config.Schema.Type().TerraformType(ctx), nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
