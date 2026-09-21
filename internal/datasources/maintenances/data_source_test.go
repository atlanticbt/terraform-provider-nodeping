package maintenances

import (
	"context"
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

func dataSourceState(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	d := &MaintenancesDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
}

func testConfig(t *testing.T, ctx context.Context, model MaintenancesDataSourceModel) tfsdk.Config {
	t.Helper()
	state := dataSourceState(t, ctx)
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}
	return tfsdk.Config{Schema: state.Schema, Raw: state.Raw}
}

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *MaintenancesDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &MaintenancesDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func TestDataSourceMetadataAndConfigure(t *testing.T) {
	ctx := context.Background()
	d := NewMaintenancesDataSource().(*MaintenancesDataSource)

	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_maintenances" {
		t.Fatalf("type name = %q", metaResp.TypeName)
	}

	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if d.client != nil || resp.Diagnostics.HasError() {
		t.Fatal("nil provider data must be a no-op")
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

func TestClientForScopesToCustomerID(t *testing.T) {
	base := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &MaintenancesDataSource{client: base}

	if d.clientFor(types.StringNull()) != base || d.clientFor(types.StringValue("")) != base ||
		d.clientFor(types.StringUnknown()) != base {
		t.Error("an unset customer_id must use the provider-level client")
	}
	if d.clientFor(types.StringValue("SUB1")) == base {
		t.Error("a set customer_id must produce a scoped client")
	}
}

func TestMapMaintenancesIsSortedAndDeterministic(t *testing.T) {
	nextRun := int64(1700000000000)
	windows := map[string]client.Maintenance{
		"C3":     {ID: "C3", Name: "third", Duration: 30, Checklist: []string{"K2", "K1"}},
		"A1":     {ID: "A1", Name: "first", Enabled: true, Cron: "0 3 * * 0", Duration: 60, NextRun: &nextRun},
		"B2":     {Name: "second", Duration: 15},
		"ad-hoc": {ID: "ad-hoc", Name: "one off", Duration: 5},
	}

	for i := 0; i < 200; i++ {
		got := MapMaintenances(windows)
		if len(got) != 4 {
			t.Fatalf("got %d windows", len(got))
		}
		ids := []string{
			got[0].ID.ValueString(), got[1].ID.ValueString(),
			got[2].ID.ValueString(), got[3].ID.ValueString(),
		}
		want := []string{"A1", "B2", "C3", "ad-hoc"}
		for j := range want {
			if ids[j] != want[j] {
				t.Fatalf("iteration %d: unsorted result %v", i, ids)
			}
		}
	}

	got := MapMaintenances(windows)

	// B2 has no `id` in its payload; the map key must be used as a fallback.
	if got[1].ID.ValueString() != "B2" {
		t.Fatalf("id fallback failed: %q", got[1].ID.ValueString())
	}
	if !got[3].AdHoc.ValueBool() {
		t.Fatal("the ad-hoc window must be flagged")
	}
	if got[0].AdHoc.ValueBool() {
		t.Fatal("a scheduled window must not be flagged as ad-hoc")
	}
	if got[0].NextRun.ValueInt64() != nextRun || !got[1].NextRun.IsNull() {
		t.Fatalf("nextrun mapping is wrong: %v / %v", got[0].NextRun, got[1].NextRun)
	}
	if got[2].Checklist[0].ValueString() != "K1" {
		t.Fatalf("checklist was not sorted: %v", got[2].Checklist)
	}
	if !got[0].LastEditedBy.IsNull() || !got[0].LastEditedAt.IsNull() {
		t.Fatal("absent update info must be null")
	}

	if len(MapMaintenances(nil)) != 0 {
		t.Fatal("expected an empty list for no windows")
	}
}

func TestMapMaintenancesCarriesUpdateInfo(t *testing.T) {
	got := MapMaintenances(map[string]client.Maintenance{
		"A1": {
			ID: "A1", Name: "first", Duration: 60,
			Update: &client.MaintenanceUpdateInfo{Timestamp: 1699999999000, User: "ops@example.com"},
		},
	})

	if got[0].LastEditedBy.ValueString() != "ops@example.com" || got[0].LastEditedAt.ValueInt64() != 1699999999000 {
		t.Fatalf("update info = %v / %v", got[0].LastEditedBy, got[0].LastEditedAt)
	}
}

func TestDataSourceRead(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/maintenance" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"B2":{"id":"B2","name":"second","enabled":false,"duration":15,"checklist":["C1"]},
			"A1":{"id":"A1","name":"first","enabled":true,"cron":"0 3 * * 0","duration":60,"checklist":["C1","C2"]}
		}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenancesDataSourceModel{})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got MaintenancesDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if len(got.Maintenances) != 2 {
		t.Fatalf("got %d windows", len(got.Maintenances))
	}
	if got.Maintenances[0].ID.ValueString() != "A1" || got.Maintenances[1].ID.ValueString() != "B2" {
		t.Fatalf("results are not sorted by ID: %+v", got.Maintenances)
	}
	if !got.Maintenances[0].Enabled.ValueBool() || got.Maintenances[1].Enabled.ValueBool() {
		t.Fatal("enabled was not mapped")
	}
}

// The live GET /maintenance response wraps the windows in an envelope and
// separates ad-hoc windows into their own group.
func TestDataSourceReadDecodesTheMaintenanceEnvelope(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{
			"customer_id":"201203232048C76FH",
			"schedules":{"A1":{"id":"A1","name":"first","enabled":true,"cron":"0 3 * * 0","duration":60,"checklist":["C1"]}},
			"ad-hoc":{"B2":{"id":"B2","name":"one off","enabled":true,"duration":15,"checklist":["C1"]}}
		}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenancesDataSourceModel{})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got MaintenancesDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if len(got.Maintenances) != 2 {
		t.Fatalf("got %d windows: %+v", len(got.Maintenances), got.Maintenances)
	}
	if got.Maintenances[0].ID.ValueString() != "A1" || got.Maintenances[0].AdHoc.ValueBool() {
		t.Errorf("a scheduled window was mapped wrongly: %+v", got.Maintenances[0])
	}
	if got.Maintenances[1].ID.ValueString() != "B2" || !got.Maintenances[1].AdHoc.ValueBool() {
		t.Errorf("a window from the ad-hoc group must be flagged: %+v", got.Maintenances[1])
	}
}

func TestDataSourceReadScopesToCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write([]byte(`{}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenancesDataSourceModel{
		CustomerID: types.StringValue("SUB1"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid query = %q", seen)
	}
}

func TestDataSourceReadSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	resp := &datasource.ReadResponse{State: dataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenancesDataSourceModel{})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
