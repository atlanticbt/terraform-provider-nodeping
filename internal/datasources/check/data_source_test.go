package check

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

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

func newTestDataSource(t *testing.T, checks map[string]interface{}) (*CheckDataSource, *string) {
	t.Helper()
	lastQuery := new(string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastQuery = r.URL.RawQuery
		id := r.URL.Path[len("/checks/"):]
		check, ok := checks[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(check)
	}))
	t.Cleanup(srv.Close)

	return &CheckDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:  "token",
		BaseURL:   srv.URL,
		RateLimit: 1000,
	})}, lastQuery
}

func dataSourceSchema(t *testing.T, ctx context.Context, d *CheckDataSource) tfsdk.Config {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	return tfsdk.Config{Schema: resp.Schema}
}

func readDataSource(t *testing.T, d *CheckDataSource, config CheckDataSourceModel) (CheckDataSourceModel, *datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	if config.Tags.ElementType(ctx) == nil {
		config.Tags = types.ListNull(types.StringType)
	}

	cfg := dataSourceSchema(t, ctx, d)
	raw := tftypes.NewValue(cfg.Schema.Type().TerraformType(ctx), nil)
	state := tfsdk.State{Schema: cfg.Schema, Raw: raw}
	if diags := state.Set(ctx, &config); diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	cfg.Raw = state.Raw

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)

	var out CheckDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &out); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return out, resp
}

func TestCheckDataSource_Read(t *testing.T) {
	d, lastQuery := newTestDataSource(t, map[string]interface{}{
		"CHK-1": map[string]interface{}{
			"_id":         "CHK-1",
			"customer_id": "SUB-1",
			"type":        "HTTP",
			"label":       "web",
			"enable":      "active",
			"interval":    5,
			"state":       1,
			"created":     10,
			"modified":    20,
			"description": "notes",
			"dep":         "CHK-DEP",
			"tags":        []string{"a"},
			"parameters": map[string]interface{}{
				"target":    "https://example.com",
				"threshold": "10",
				"sens":      3,
			},
		},
	})

	got, resp := readDataSource(t, d, CheckDataSourceModel{
		ID:         types.StringValue("CHK-1"),
		CustomerID: types.StringValue("SUB-1"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}

	if got.Threshold.ValueInt64() != 10 {
		t.Errorf("threshold = %v, want 10 (string encoded numbers must decode)", got.Threshold)
	}
	if got.Sens.ValueInt64() != 3 {
		t.Errorf("sens = %v, want 3", got.Sens)
	}
	if !got.Enabled.ValueBool() || got.Interval.ValueFloat64() != 5 {
		t.Errorf("enabled/interval = %v %v", got.Enabled, got.Interval)
	}
	if got.Dep.ValueString() != "CHK-DEP" || got.Description.ValueString() != "notes" {
		t.Errorf("dep/description = %v %v", got.Dep, got.Description)
	}
	if len(got.Tags.Elements()) != 1 {
		t.Errorf("tags = %v", got.Tags)
	}
	if *lastQuery != "customerid=SUB-1" {
		t.Errorf("query = %q, want the subaccount to be scoped", *lastQuery)
	}
}

func TestCheckDataSource_MissingCheck(t *testing.T) {
	d, _ := newTestDataSource(t, nil)
	_, resp := readDataSource(t, d, CheckDataSourceModel{ID: types.StringValue("NOPE")})
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for a missing check")
	}
}

func TestCheckDataSource_NullsForUnsetValues(t *testing.T) {
	d, _ := newTestDataSource(t, map[string]interface{}{
		"CHK-2": map[string]interface{}{
			"_id":        "CHK-2",
			"type":       "PING",
			"enable":     "false",
			"dep":        false,
			"parameters": map[string]interface{}{"target": "example.com"},
		},
	})
	got, resp := readDataSource(t, d, CheckDataSourceModel{ID: types.StringValue("CHK-2")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	if !got.Dep.IsNull() || !got.Description.IsNull() || !got.Tags.IsNull() {
		t.Errorf("unset values should be null: %v %v %v", got.Dep, got.Description, got.Tags)
	}
	if !got.Threshold.IsNull() || !got.Sens.IsNull() {
		t.Errorf("absent numbers should be null: %v %v", got.Threshold, got.Sens)
	}
	if got.Enabled.ValueBool() {
		t.Error("enabled should be false")
	}
}

func TestNumberOrNull(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int64
		null bool
	}{
		{in: float64(1), want: 1},
		{in: 2, want: 2},
		{in: int64(3), want: 3},
		{in: json.Number("4"), want: 4},
		{in: "5", want: 5},
		{in: " 6 ", want: 6},
		{in: nil, null: true},
		{in: "abc", null: true},
		{in: false, null: true},
	}
	for _, c := range cases {
		got := numberOrNull(c.in)
		if c.null {
			if !got.IsNull() {
				t.Errorf("numberOrNull(%#v) = %v, want null", c.in, got)
			}
			continue
		}
		if got.ValueInt64() != c.want {
			t.Errorf("numberOrNull(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCheckDataSource_MetadataAndConfigure(t *testing.T) {
	ctx := context.Background()
	d := NewCheckDataSource().(*CheckDataSource)

	mResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, mResp)
	if mResp.TypeName != "nodeping_check" {
		t.Errorf("type name = %q", mResp.TypeName)
	}

	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Error("nil provider data should be a no-op")
	}

	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for the wrong provider data type")
	}

	resp = &datasource.ConfigureResponse{}
	c := client.NewClient(client.ClientConfig{APIToken: "x"})
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if d.client != c {
		t.Error("client was not stored")
	}

	sResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, sResp)
	if !sResp.Schema.Attributes["customer_id"].IsOptional() {
		t.Error("customer_id should be configurable for subaccount lookups")
	}
}
