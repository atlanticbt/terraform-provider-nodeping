package contactgroups

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *ContactGroupsDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ContactGroupsDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func testSchema(t *testing.T, ctx context.Context) datasource.SchemaResponse {
	t.Helper()
	d := &ContactGroupsDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return *resp
}

// buildConfig builds a datasource config holding the given model. tfsdk.Config
// has no Set method, so the value is built through a State and the raw value is
// carried across.
func buildConfig(t *testing.T, ctx context.Context, customerID types.String) tfsdk.Config {
	t.Helper()
	s := testSchema(t, ctx).Schema

	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := st.Set(ctx, &ContactGroupsDataSourceModel{CustomerID: customerID}); diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	return tfsdk.Config{Schema: s, Raw: st.Raw}
}

func emptyState(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	s := testSchema(t, ctx).Schema
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) ContactGroupsDataSourceModel {
	t.Helper()
	var out ContactGroupsDataSourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func sampleGroups() map[string]client.ContactGroup {
	return map[string]client.ContactGroup{
		"ACC-G-ZZZZZ": {
			ID:         "ACC-G-ZZZZZ",
			Type:       "group",
			CustomerID: "ACC",
			Name:       "Zeta",
			Members:    []string{"K5SP9CQP", "1664V"},
		},
		"ACC-G-AAAAA": {
			ID:         "ACC-G-AAAAA",
			Type:       "group",
			CustomerID: "ACC",
			Name:       "Alpha",
		},
		// Older responses have been seen without the redundant _id field.
		"ACC-G-MMMMM": {
			Type:       "group",
			CustomerID: "ACC",
			Name:       "Mu",
			Members:    []string{"BBBB"},
		},
	}
}

func TestMetadataUsesContactgroupsTypeName(t *testing.T) {
	d := &ContactGroupsDataSource{}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_contactgroups" {
		t.Errorf("unexpected type name %q", resp.TypeName)
	}
}

func TestSchemaExposesOptionalCustomerIDAndComputedList(t *testing.T) {
	s := testSchema(t, context.Background()).Schema

	customerID, ok := s.Attributes["customer_id"]
	if !ok || !customerID.IsOptional() {
		t.Error("customer_id must be an optional attribute")
	}
	groups, ok := s.Attributes["contactgroups"]
	if !ok || !groups.IsComputed() {
		t.Error("contactgroups must be a computed attribute")
	}
}

func TestConfigureRejectsWrongProviderData(t *testing.T) {
	d := &ContactGroupsDataSource{}
	resp := &datasource.ConfigureResponse{}
	d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "nope"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong provider data type")
	}
}

func TestConfigureIgnoresNilProviderDataAndStoresClient(t *testing.T) {
	d := &ContactGroupsDataSource{}
	resp := &datasource.ConfigureResponse{}
	d.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || d.client != nil {
		t.Fatal("nil provider data must be a no-op")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if d.client != c {
		t.Error("expected the provider client to be stored")
	}
}

func TestClientForScopesOnlyWhenCustomerIDIsSet(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &ContactGroupsDataSource{client: c}

	if d.clientFor(types.StringNull()) != c {
		t.Error("a null customer_id must reuse the provider client")
	}
	if d.clientFor(types.StringUnknown()) != c {
		t.Error("an unknown customer_id must reuse the provider client")
	}
	if d.clientFor(types.StringValue("SUB")) == c {
		t.Error("a set customer_id must produce a scoped client")
	}
}

func TestMapContactGroupsIsSortedAndDeterministic(t *testing.T) {
	ctx := context.Background()
	groups := sampleGroups()

	for i := 0; i < 200; i++ {
		var diags diag.Diagnostics
		got := mapContactGroups(ctx, groups, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3 groups, got %d", len(got))
		}
		if got[0].ID.ValueString() != "ACC-G-AAAAA" ||
			got[1].ID.ValueString() != "ACC-G-MMMMM" ||
			got[2].ID.ValueString() != "ACC-G-ZZZZZ" {
			t.Fatalf("groups are not sorted by ID: %v %v %v",
				got[0].ID, got[1].ID, got[2].ID)
		}
	}
}

func TestMapContactGroupsFallsBackToMapKeyForID(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	got := mapContactGroups(ctx, map[string]client.ContactGroup{
		"ACC-G-KEYONLY": {Name: "No ID Field"},
	}, &diags)

	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if got[0].ID.ValueString() != "ACC-G-KEYONLY" {
		t.Errorf("expected the map key to be used as the ID, got %q", got[0].ID.ValueString())
	}
}

func TestMapContactGroupsSortsMembersAndNeverReturnsNullSets(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	got := mapContactGroups(ctx, sampleGroups(), &diags)

	for _, g := range got {
		if g.Members.IsNull() || g.Members.IsUnknown() {
			t.Fatalf("group %s has a null member set", g.ID.ValueString())
		}
	}

	var members []string
	if d := got[2].Members.ElementsAs(ctx, &members, false); d.HasError() {
		t.Fatalf("failed to read members: %v", d)
	}
	if len(members) != 2 || members[0] != "1664V" || members[1] != "K5SP9CQP" {
		t.Errorf("expected sorted members, got %v", members)
	}
}

func TestMapContactGroupsHandlesEmptyResponse(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	got := mapContactGroups(ctx, nil, &diags)
	if got == nil || len(got) != 0 {
		t.Errorf("expected an empty non-nil slice, got %#v", got)
	}
}

func TestReadListsGroupsSortedByID(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if req.URL.Path != "/contactgroups" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(sampleGroups()); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: emptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: buildConfig(t, ctx, types.StringNull())}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "" {
		t.Errorf("no customer_id was configured, so the request must not be scoped; got %q", seen)
	}

	got := decodeState(t, ctx, resp.State)
	if len(got.ContactGroups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(got.ContactGroups))
	}
	if got.ContactGroups[0].Name.ValueString() != "Alpha" {
		t.Errorf("expected the first group to be Alpha, got %q", got.ContactGroups[0].Name.ValueString())
	}
}

func TestReadScopesToConfiguredCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if err := json.NewEncoder(w).Encode(map[string]client.ContactGroup{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: emptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: buildConfig(t, ctx, types.StringValue("SUB"))}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB" {
		t.Errorf("expected customerid=SUB, got %q", seen)
	}

	got := decodeState(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUB" {
		t.Errorf("expected the configured customer_id to be preserved, got %q", got.CustomerID)
	}
	if len(got.ContactGroups) != 0 {
		t.Errorf("expected no groups, got %d", len(got.ContactGroups))
	}
}

func TestReadSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	resp := &datasource.ReadResponse{State: emptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: buildConfig(t, ctx, types.StringNull())}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic when the list call fails")
	}
}
