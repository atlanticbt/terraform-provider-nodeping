package maintenance

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

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

func dataSourceSchema(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	d := &MaintenanceDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
}

// testConfig builds a datasource config holding the given model. tfsdk.Config
// has no Set method, so the raw value is produced through a tfsdk.State first.
func testConfig(t *testing.T, ctx context.Context, model MaintenanceDataSourceModel) tfsdk.Config {
	t.Helper()
	state := dataSourceSchema(t, ctx)
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}
	return tfsdk.Config(state)
}

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *MaintenanceDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &MaintenanceDataSource{client: client.NewClient(client.ClientConfig{
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
	d := NewMaintenanceDataSource().(*MaintenanceDataSource)

	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_maintenance" {
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
	d := &MaintenanceDataSource{client: base}

	if d.clientFor(types.StringNull()) != base || d.clientFor(types.StringValue("")) != base ||
		d.clientFor(types.StringUnknown()) != base {
		t.Error("an unset customer_id must use the provider-level client")
	}
	if d.clientFor(types.StringValue("SUB1")) == base {
		t.Error("a set customer_id must produce a scoped client")
	}
}

func TestSortedStringsIsDeterministic(t *testing.T) {
	got := SortedStrings([]string{"C3", "C1", "C2"})
	if len(got) != 3 || got[0].ValueString() != "C1" || got[2].ValueString() != "C3" {
		t.Fatalf("unsorted: %v", got)
	}
	if len(SortedStrings(nil)) != 0 {
		t.Fatal("expected an empty list for no input")
	}
}

func TestMapHistoryIsSortedOldestFirst(t *testing.T) {
	maint := &client.Maintenance{History: map[string]client.MaintenanceRun{
		"1700000300000": {DisabledChecklist: []string{"C3"}},
		"1700000100000": {DisabledChecklist: []string{"C2", "C1"}, SkippedChecklist: []string{"S1"}},
		"1700000200000": {},
	}}

	for i := 0; i < 200; i++ {
		got := MapHistory(maint)
		if len(got) != 3 {
			t.Fatalf("got %d entries", len(got))
		}
		if got[0].Timestamp.ValueInt64() != 1700000100000 ||
			got[1].Timestamp.ValueInt64() != 1700000200000 ||
			got[2].Timestamp.ValueInt64() != 1700000300000 {
			t.Fatalf("iteration %d: unsorted history", i)
		}
		if got[0].DisabledChecks[0].ValueString() != "C1" {
			t.Fatalf("disabled checks were not sorted: %v", got[0].DisabledChecks)
		}
		if got[0].SkippedChecks[0].ValueString() != "S1" {
			t.Fatalf("skipped checks = %v", got[0].SkippedChecks)
		}
	}

	if MapHistory(&client.Maintenance{}) != nil {
		t.Fatal("expected nil for no history")
	}
}

func TestDataSourceRead(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/maintenance/9VKOB" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		nextRun := int64(1700000000000)
		body, _ := json.Marshal(client.Maintenance{
			ID: "9VKOB", CustomerID: "CUST1", Name: "Weekly", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C2", "C1"},
			Processing: true, NextRun: &nextRun,
			Update: &client.MaintenanceUpdateInfo{Timestamp: 1699999999000, User: "ops@example.com"},
			History: map[string]client.MaintenanceRun{
				"1700000100000": {DisabledChecklist: []string{"C1"}},
			},
		})
		_, _ = w.Write(body)
	})

	resp := &datasource.ReadResponse{State: dataSourceSchema(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenanceDataSourceModel{
		ID: types.StringValue("9VKOB"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got MaintenanceDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}

	if got.Name.ValueString() != "Weekly" || got.Duration.ValueInt64() != 60 ||
		!got.Enabled.ValueBool() || !got.Processing.ValueBool() {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.CustomerID.ValueString() != "CUST1" {
		t.Fatalf("customer_id = %q", got.CustomerID.ValueString())
	}
	if got.NextRun.ValueInt64() != 1700000000000 {
		t.Fatalf("nextrun = %v", got.NextRun)
	}
	if got.LastEditedBy.ValueString() != "ops@example.com" || got.LastEditedAt.ValueInt64() != 1699999999000 {
		t.Fatalf("update info = %v / %v", got.LastEditedBy, got.LastEditedAt)
	}
	if len(got.Checklist) != 2 || got.Checklist[0].ValueString() != "C1" {
		t.Fatalf("checklist = %v", got.Checklist)
	}
	if len(got.History) != 1 || got.History[0].Timestamp.ValueInt64() != 1700000100000 {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestDataSourceReadNullsAbsentOptionalValues(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"id":"9VKOB","name":"Weekly","duration":60}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceSchema(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenanceDataSourceModel{
		ID: types.StringValue("9VKOB"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got MaintenanceDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if !got.NextRun.IsNull() || !got.LastEditedBy.IsNull() || !got.LastEditedAt.IsNull() {
		t.Fatalf("absent values must be null: %+v", got)
	}
	if !got.CustomerID.IsNull() {
		t.Fatal("customer_id must be null when neither config nor API supplies one")
	}
	if got.History != nil {
		t.Fatal("history must be nil when the API reports none")
	}
}

func TestDataSourceReadScopesToCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write([]byte(`{"id":"9VKOB","name":"Weekly","duration":60}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceSchema(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenanceDataSourceModel{
		ID:         types.StringValue("9VKOB"),
		CustomerID: types.StringValue("SUB1"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid query = %q", seen)
	}
}

// TestDataSourceReadReportsDisappearedWindow documents what happens when the
// referenced window no longer exists — the usual reason being that it was an
// ad-hoc window that NodePing deleted after it finished running. A data source
// has no state to remove, so this must be a hard error rather than a silent
// empty result.
func TestDataSourceReadReportsDisappearedWindow(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	resp := &datasource.ReadResponse{State: dataSourceSchema(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, MaintenanceDataSourceModel{
		ID: types.StringValue("ad-hoc"),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
