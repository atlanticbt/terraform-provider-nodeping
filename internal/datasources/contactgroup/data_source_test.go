package contactgroup

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

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *ContactGroupDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ContactGroupDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func buildConfig(t *testing.T, ctx context.Context, model ContactGroupDataSourceModel) tfsdk.Config {
	t.Helper()
	d := &ContactGroupDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)

	// tfsdk.Config has no Set method, so the value is built through a State and
	// the raw value is carried across.
	st := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := st.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	return tfsdk.Config{Schema: resp.Schema, Raw: st.Raw}
}

func emptyDataSourceState(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	d := &ContactGroupDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)

	return tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
}

func decodeDataSourceState(t *testing.T, ctx context.Context, st tfsdk.State) ContactGroupDataSourceModel {
	t.Helper()
	var out ContactGroupDataSourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func groupJSON(t *testing.T, id, customerID, name string, members []string) []byte {
	t.Helper()
	body, err := json.Marshal(client.ContactGroup{
		ID:         id,
		Type:       "group",
		CustomerID: customerID,
		Name:       name,
		Members:    members,
	})
	if err != nil {
		t.Fatalf("failed to encode group: %v", err)
	}
	return body
}

func TestMetadataUsesContactgroupTypeName(t *testing.T) {
	d := &ContactGroupDataSource{}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_contactgroup" {
		t.Errorf("unexpected type name %q", resp.TypeName)
	}
}

func TestSchemaRequiresIDAndComputesMembers(t *testing.T) {
	d := &ContactGroupDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	id, ok := resp.Schema.Attributes["id"]
	if !ok || !id.IsRequired() {
		t.Error("id must be a required attribute")
	}
	members, ok := resp.Schema.Attributes["members"]
	if !ok || !members.IsComputed() {
		t.Error("members must be a computed attribute")
	}
	if _, isSet := members.GetType().(types.SetType); !isSet {
		t.Errorf("members must be a set, got %T", members.GetType())
	}
	customerID, ok := resp.Schema.Attributes["customer_id"]
	if !ok || !customerID.IsOptional() {
		t.Error("customer_id must be optional")
	}
}

func TestConfigureRejectsWrongProviderData(t *testing.T) {
	d := &ContactGroupDataSource{}
	resp := &datasource.ConfigureResponse{}
	d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong provider data type")
	}
}

func TestConfigureIgnoresNilProviderDataAndStoresClient(t *testing.T) {
	d := &ContactGroupDataSource{}
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
	d := &ContactGroupDataSource{client: c}

	if d.clientFor(types.StringNull()) != c {
		t.Error("a null customer_id must reuse the provider client")
	}
	if d.clientFor(types.StringValue("")) != c {
		t.Error("an empty customer_id must reuse the provider client")
	}
	if d.clientFor(types.StringValue("SUB")) == c {
		t.Error("a set customer_id must produce a scoped client")
	}
}

func TestReadPopulatesNameAndSortedMembers(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if req.URL.Path != "/contactgroups/ACC-G-ISYB7" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", []string{"K5SP9CQP", "1664V"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: emptyDataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{
		Config: buildConfig(t, ctx, ContactGroupDataSourceModel{
			ID:         types.StringValue("ACC-G-ISYB7"),
			CustomerID: types.StringNull(),
			Name:       types.StringNull(),
			Members:    types.SetNull(types.StringType),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "" {
		t.Errorf("no customer_id was configured, so the request must not be scoped; got %q", seen)
	}

	got := decodeDataSourceState(t, ctx, resp.State)
	if got.Name.ValueString() != "Alerts" {
		t.Errorf("unexpected name %q", got.Name.ValueString())
	}
	if got.CustomerID.ValueString() != "ACC" {
		t.Errorf("expected customer_id from the response, got %q", got.CustomerID.ValueString())
	}

	var members []string
	if diags := got.Members.ElementsAs(ctx, &members, false); diags.HasError() {
		t.Fatalf("failed to read members: %v", diags)
	}
	if len(members) != 2 || members[0] != "1664V" || members[1] != "K5SP9CQP" {
		t.Errorf("expected sorted members, got %v", members)
	}
}

func TestReadScopesToConfiguredCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if _, err := w.Write(groupJSON(t, "SUB-G-1", "SUB", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: emptyDataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{
		Config: buildConfig(t, ctx, ContactGroupDataSourceModel{
			ID:         types.StringValue("SUB-G-1"),
			CustomerID: types.StringValue("SUB"),
			Name:       types.StringNull(),
			Members:    types.SetNull(types.StringType),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB" {
		t.Errorf("expected customerid=SUB, got %q", seen)
	}

	got := decodeDataSourceState(t, ctx, resp.State)
	if got.Members.IsNull() {
		t.Error("a group with no members must produce an empty set, not null")
	}
}

func TestReadLeavesCustomerIDNullWhenAPIOmitsIt(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-1", "", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: emptyDataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{
		Config: buildConfig(t, ctx, ContactGroupDataSourceModel{
			ID:         types.StringValue("ACC-G-1"),
			CustomerID: types.StringNull(),
			Name:       types.StringNull(),
			Members:    types.SetNull(types.StringType),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got := decodeDataSourceState(t, ctx, resp.State); !got.CustomerID.IsNull() {
		t.Errorf("expected a null customer_id, got %v", got.CustomerID)
	}
}

func TestReadSurfacesNotFound(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp := &datasource.ReadResponse{State: emptyDataSourceState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{
		Config: buildConfig(t, ctx, ContactGroupDataSourceModel{
			ID:         types.StringValue("GONE-G-1"),
			CustomerID: types.StringNull(),
			Name:       types.StringNull(),
			Members:    types.SetNull(types.StringType),
		}),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a missing group must be an error for a data source")
	}
}

func TestMembersToSetIsSortedAndNeverNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	set := membersToSet(ctx, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if set.IsNull() {
		t.Fatal("an empty member list must produce a known empty set")
	}

	set = membersToSet(ctx, []string{"ZZZ", "AAA", "MMM"}, &diags)
	var out []string
	if d := set.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("failed to read set: %v", d)
	}
	if len(out) != 3 || out[0] != "AAA" || out[2] != "ZZZ" {
		t.Errorf("expected sorted members, got %v", out)
	}
}
