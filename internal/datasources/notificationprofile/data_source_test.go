package notificationprofile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

// testConfig builds a datasource config holding the given model. tfsdk.Config
// has no Set method, so the raw value is produced through a tfsdk.State first.
func testConfig(t *testing.T, ctx context.Context, model NotificationProfileDataSourceModel) (tfsdk.Config, tfsdk.State) {
	t.Helper()
	d := &NotificationProfileDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}

	return tfsdk.Config{Schema: resp.Schema, Raw: state.Raw}, state
}

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *NotificationProfileDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &NotificationProfileDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func profileJSON(t *testing.T, id, customerID, name string, notifications []map[string]interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(client.NotificationProfile{
		ID:            id,
		Type:          "notification profile",
		CustomerID:    customerID,
		Name:          name,
		Notifications: notifications,
	})
	if err != nil {
		t.Fatalf("failed to marshal profile: %v", err)
	}
	return body
}

func contactIDs(notifications []NotificationDataSourceModel) []string {
	out := make([]string, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, n.ContactID.ValueString())
	}
	return out
}

func TestMetadataUsesProviderTypeName(t *testing.T) {
	d := NewNotificationProfileDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_notificationprofile" {
		t.Fatalf("expected nodeping_notificationprofile, got %s", resp.TypeName)
	}
}

func TestConfigureAcceptsClientAndRejectsOtherTypes(t *testing.T) {
	ctx := context.Background()
	d := &NotificationProfileDataSource{}

	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if d.client != nil {
		t.Fatal("expected no client when ProviderData is nil")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || d.client != c {
		t.Fatalf("expected the provider client to be stored: %v", resp.Diagnostics)
	}

	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an unexpected ProviderData type")
	}
}

func TestClientForScopesToCustomer(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &NotificationProfileDataSource{client: c}

	if d.clientFor(types.StringNull()) != c {
		t.Fatal("expected the provider client when customer_id is null")
	}
	if d.clientFor(types.StringUnknown()) != c {
		t.Fatal("expected the provider client when customer_id is unknown")
	}
	if d.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client when customer_id is set")
	}
}

func TestMapNotificationsIsDeterministicAcrossRepeatedRuns(t *testing.T) {
	api := []map[string]interface{}{
		{
			"C-3": map[string]interface{}{"delay": float64(30), "schedule": "Nights"},
			"A-1": map[string]interface{}{"delay": float64(0), "schedule": "All"},
			"B-2": map[string]interface{}{"delay": float64(15), "schedule": "Days"},
		},
	}

	want := []string{"A-1", "B-2", "C-3"}
	for i := 0; i < 50; i++ {
		if got := contactIDs(MapNotifications(api)); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: got %v, want %v", i, got, want)
		}
	}
}

func TestMapNotificationsPreservesRepeatedContactIDs(t *testing.T) {
	api := []map[string]interface{}{
		{"A-1": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
		{"A-1": map[string]interface{}{"delay": float64(30), "schedule": "Days"}},
	}

	got := MapNotifications(api)
	if len(got) != 2 {
		t.Fatalf("expected repeated entries to be preserved, got %d", len(got))
	}
}

func TestMapNotificationsAppliesDefaultsAndSkipsMalformedEntries(t *testing.T) {
	got := MapNotifications([]map[string]interface{}{
		{"A-1": map[string]interface{}{}},
		{"B-2": "nonsense"},
	})
	if len(got) != 1 {
		t.Fatalf("expected the malformed entry to be skipped, got %d entries", len(got))
	}
	if got[0].Delay.ValueInt64() != 0 || got[0].Schedule.ValueString() != defaultSchedule {
		t.Fatalf("unexpected defaults: %v", got[0])
	}
}

func TestToInt64DecodesNodePingNumericVariants(t *testing.T) {
	for _, v := range []interface{}{float64(45), float32(45), 45, int64(45), "45", json.Number("45")} {
		got, ok := ToInt64(v)
		if !ok || got != 45 {
			t.Fatalf("%#v: got %d (ok=%v)", v, got, ok)
		}
	}
	for _, v := range []interface{}{nil, "", "  ", "abc", true, json.Number("abc")} {
		if got, ok := ToInt64(v); ok {
			t.Fatalf("expected %#v to be undecodable, got %d", v, got)
		}
	}
	if got, ok := ToInt64("1.9"); !ok || got != 1 {
		t.Fatalf("expected 1, got %d (ok=%v)", got, ok)
	}
	if got, ok := ToInt64(json.Number("1.9")); !ok || got != 1 {
		t.Fatalf("expected 1, got %d (ok=%v)", got, ok)
	}
}

func TestReadPopulatesState(t *testing.T) {
	ctx := context.Background()

	var query string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "SUB1", "Escalation", []map[string]interface{}{
			{"A-1": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
			{"G-1": map[string]interface{}{"delay": float64(15), "schedule": "Days"}},
		}))
	})

	config, state := testConfig(t, ctx, NotificationProfileDataSourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
	})

	resp := &datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if query != "SUB1" {
		t.Fatalf("expected the lookup to be scoped to SUB1, got %q", query)
	}

	var got NotificationProfileDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got.Name.ValueString() != "Escalation" {
		t.Fatalf("unexpected name: %q", got.Name.ValueString())
	}
	if !reflect.DeepEqual(contactIDs(got.Notifications), []string{"A-1", "G-1"}) {
		t.Fatalf("unexpected notifications: %v", contactIDs(got.Notifications))
	}
}

func TestReadDefaultsCustomerIDWhenAPIOmitsIt(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "", "Escalation", nil))
	})

	config, state := testConfig(t, ctx, NotificationProfileDataSourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringNull(),
	})

	resp := &datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got NotificationProfileDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got.CustomerID.IsNull() || got.CustomerID.IsUnknown() {
		t.Fatal("computed customer_id must be resolved before it reaches state")
	}
}

func TestReadReportsAPIError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	config, state := testConfig(t, ctx, NotificationProfileDataSourceModel{
		ID: types.StringValue("ACC-P-MISSING"),
	})

	resp := &datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
