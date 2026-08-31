package notificationprofiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

func testConfig(t *testing.T, ctx context.Context, model NotificationProfilesDataSourceModel) (tfsdk.Config, tfsdk.State) {
	t.Helper()
	d := &NotificationProfilesDataSource{}
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

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *NotificationProfilesDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &NotificationProfilesDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func profileIDs(profiles []NotificationProfileListModel) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.ID.ValueString())
	}
	return out
}

func TestMetadataUsesProviderTypeName(t *testing.T) {
	d := NewNotificationProfilesDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_notificationprofiles" {
		t.Fatalf("expected nodeping_notificationprofiles, got %s", resp.TypeName)
	}
}

func TestConfigureAcceptsClientAndRejectsOtherTypes(t *testing.T) {
	ctx := context.Background()
	d := &NotificationProfilesDataSource{}

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
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an unexpected ProviderData type")
	}
}

func TestClientForScopesToCustomer(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &NotificationProfilesDataSource{client: c}

	if d.clientFor(types.StringNull()) != c {
		t.Fatal("expected the provider client when customer_id is null")
	}
	if d.clientFor(types.StringValue("")) != c {
		t.Fatal("expected the provider client when customer_id is empty")
	}
	if d.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client when customer_id is set")
	}
}

func TestMapProfilesIsSortedAndDeterministic(t *testing.T) {
	profiles := map[string]client.NotificationProfile{
		"C-P-3": {ID: "C-P-3", Name: "third"},
		"A-P-1": {ID: "A-P-1", Name: "first"},
		"B-P-2": {ID: "B-P-2", Name: "second"},
	}

	want := []string{"A-P-1", "B-P-2", "C-P-3"}
	for i := 0; i < 50; i++ {
		if got := profileIDs(MapProfiles(profiles)); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: got %v, want %v", i, got, want)
		}
	}
}

func TestMapProfilesFallsBackToTheMapKeyForMissingIDs(t *testing.T) {
	got := MapProfiles(map[string]client.NotificationProfile{
		"ACC-P-KEY": {Name: "no _id in the payload"},
	})
	if len(got) != 1 || got[0].ID.ValueString() != "ACC-P-KEY" {
		t.Fatalf("expected the map key to be used as the ID, got %v", got)
	}
}

func TestMapProfilesFlattensNotifications(t *testing.T) {
	got := MapProfiles(map[string]client.NotificationProfile{
		"ACC-P-1": {
			ID:   "ACC-P-1",
			Name: "Escalation",
			Notifications: []map[string]interface{}{
				{"B-2": map[string]interface{}{"delay": float64(15), "schedule": "Days"}},
				{"A-1": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
			},
		},
	})

	if len(got[0].Notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(got[0].Notifications))
	}
	// Array order is the API's; only keys within one element are sorted.
	if got[0].Notifications[0].ContactID.ValueString() != "B-2" {
		t.Fatalf("expected the API array order to be preserved, got %v", got[0].Notifications)
	}
}

func TestReadListsProfilesFromAnObjectResponse(t *testing.T) {
	ctx := context.Background()

	var query string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		_, _ = w.Write([]byte(`{
			"B-P-2": {"_id":"B-P-2","type":"notification profile","customer_id":"SUB1","name":"second","notifications":[]},
			"A-P-1": {"_id":"A-P-1","type":"notification profile","customer_id":"SUB1","name":"first","notifications":[{"C-1":{"delay":0,"schedule":"All"}}]}
		}`))
	})

	config, state := testConfig(t, ctx, NotificationProfilesDataSourceModel{
		CustomerID: types.StringValue("SUB1"),
	})

	resp := &datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if query != "SUB1" {
		t.Fatalf("expected the listing to be scoped to SUB1, got %q", query)
	}

	var got NotificationProfilesDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !reflect.DeepEqual(profileIDs(got.Profiles), []string{"A-P-1", "B-P-2"}) {
		t.Fatalf("expected profiles sorted by ID, got %v", profileIDs(got.Profiles))
	}
	if len(got.Profiles[0].Notifications) != 1 {
		t.Fatalf("expected 1 notification on the first profile, got %d", len(got.Profiles[0].Notifications))
	}
}

func TestReadReportsAPIError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	config, state := testConfig(t, ctx, NotificationProfilesDataSourceModel{})

	resp := &datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}
