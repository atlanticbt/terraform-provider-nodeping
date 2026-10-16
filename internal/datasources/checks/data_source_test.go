package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

func newTestDataSource(t *testing.T, checks map[string]interface{}) (*ChecksDataSource, *string) {
	t.Helper()
	lastQuery := new(string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastQuery = r.URL.RawQuery
		if checks == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(checks)
	}))
	t.Cleanup(srv.Close)

	return &ChecksDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:   "token",
		BaseURL:    srv.URL,
		RateLimit:  1000,
		MaxRetries: 1,
	})}, lastQuery
}

func readDataSource(t *testing.T, d *ChecksDataSource, config ChecksDataSourceModel) (ChecksDataSourceModel, *datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	if config.Tags.ElementType(ctx) == nil {
		config.Tags = types.ListNull(types.StringType)
	}

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
	if diags := state.Set(ctx, &config); diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, resp)

	var out ChecksDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &out); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return out, resp
}

func sampleChecks() map[string]interface{} {
	return map[string]interface{}{
		"CHK-B": map[string]interface{}{
			"_id":         "CHK-B",
			"customer_id": "ACCT-1",
			"type":        "DNS",
			"label":       "dns",
			"enable":      "active",
			"interval":    15,
			"state":       1,
			"tags":        []string{"prod"},
			"parameters":  map[string]interface{}{"target": "8.8.8.8"},
		},
		"CHK-A": map[string]interface{}{
			"customer_id": "ACCT-1",
			"type":        "HTTP",
			"label":       "web",
			"enable":      "active",
			"interval":    5,
			"state":       1,
			"dep":         "CHK-B",
			"tags":        []string{"prod", "web"},
			"parameters":  map[string]interface{}{"target": "https://example.com"},
		},
	}
}

func TestChecksDataSource_ListIsSortedAndComplete(t *testing.T) {
	d, _ := newTestDataSource(t, sampleChecks())

	got, resp := readDataSource(t, d, ChecksDataSourceModel{})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	if len(got.Checks) != 2 {
		t.Fatalf("check count = %d, want 2", len(got.Checks))
	}
	if got.Checks[0].ID.ValueString() != "CHK-A" || got.Checks[1].ID.ValueString() != "CHK-B" {
		t.Errorf("checks are not sorted by ID: %v %v", got.Checks[0].ID, got.Checks[1].ID)
	}
	// CHK-A carries no "_id" in the payload; the map key must be used.
	if got.Checks[0].Target.ValueString() != "https://example.com" {
		t.Errorf("target = %v", got.Checks[0].Target)
	}
	if got.Checks[0].Dep.ValueString() != "CHK-B" || !got.Checks[1].Dep.IsNull() {
		t.Errorf("dep = %v / %v", got.Checks[0].Dep, got.Checks[1].Dep)
	}
	if got.Checks[0].Interval.ValueFloat64() != 5 || !got.Checks[0].Enabled.ValueBool() {
		t.Errorf("interval/enabled = %v %v", got.Checks[0].Interval, got.Checks[0].Enabled)
	}
}

func TestChecksDataSource_Filters(t *testing.T) {
	d, _ := newTestDataSource(t, sampleChecks())

	t.Run("type filter is case insensitive", func(t *testing.T) {
		got, resp := readDataSource(t, d, ChecksDataSourceModel{Type: types.StringValue("http")})
		if resp.Diagnostics.HasError() {
			t.Fatalf("read failed: %v", resp.Diagnostics)
		}
		if len(got.Checks) != 1 || got.Checks[0].Type.ValueString() != "HTTP" {
			t.Errorf("type filter returned %#v", got.Checks)
		}
	})

	t.Run("label filter", func(t *testing.T) {
		got, _ := readDataSource(t, d, ChecksDataSourceModel{Label: types.StringValue("dns")})
		if len(got.Checks) != 1 || got.Checks[0].ID.ValueString() != "CHK-B" {
			t.Errorf("label filter returned %#v", got.Checks)
		}
	})

	t.Run("tag filter requires all tags", func(t *testing.T) {
		tags, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"prod", "web"})
		got, _ := readDataSource(t, d, ChecksDataSourceModel{Tags: tags})
		if len(got.Checks) != 1 || got.Checks[0].ID.ValueString() != "CHK-A" {
			t.Errorf("tag filter returned %#v", got.Checks)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		got, _ := readDataSource(t, d, ChecksDataSourceModel{Type: types.StringValue("SSL")})
		if len(got.Checks) != 0 {
			t.Errorf("expected no matches, got %#v", got.Checks)
		}
	})
}

func TestChecksDataSource_CustomerIDIsScoped(t *testing.T) {
	d, lastQuery := newTestDataSource(t, sampleChecks())
	_, resp := readDataSource(t, d, ChecksDataSourceModel{CustomerID: types.StringValue("SUB-2")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	if *lastQuery != "customerid=SUB-2" {
		t.Errorf("query = %q, want customerid=SUB-2", *lastQuery)
	}
}

func TestChecksDataSource_APIError(t *testing.T) {
	d, _ := newTestDataSource(t, nil)
	_, resp := readDataSource(t, d, ChecksDataSourceModel{})
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error when the API fails")
	}
}

func TestHasAllTags(t *testing.T) {
	if !hasAllTags(nil, nil) {
		t.Error("no filter should match everything")
	}
	if hasAllTags([]string{"a"}, []string{"a", "b"}) {
		t.Error("missing tag should not match")
	}
	if !hasAllTags([]string{"a", "b"}, []string{"b"}) {
		t.Error("subset filter should match")
	}
}

func TestChecksDataSource_SchemaAndConfigure(t *testing.T) {
	ctx := context.Background()
	d := NewChecksDataSource().(*ChecksDataSource)

	mResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, mResp)
	if mResp.TypeName != "nodeping_checks" {
		t.Errorf("type name = %q", mResp.TypeName)
	}

	sResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, sResp)
	typeAttr := sResp.Schema.Attributes["type"]
	if len(typeAttr.(interface{ GetDescription() string }).GetDescription()) == 0 {
		t.Error("type attribute should be documented")
	}
	for _, name := range []string{"customer_id", "label", "tags"} {
		if _, ok := sResp.Schema.Attributes[name]; !ok {
			t.Errorf("%s attribute is missing", name)
		}
	}

	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Error("nil provider data should be a no-op")
	}

	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for the wrong provider data type")
	}

	resp = &datasource.ConfigureResponse{}
	c := client.NewClient(client.ClientConfig{APIToken: "x"})
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if d.client != c {
		t.Error("client was not stored")
	}
}
