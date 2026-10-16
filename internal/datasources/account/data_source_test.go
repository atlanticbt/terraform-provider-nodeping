package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

const accountID = "201205102227VZ6XU"

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *AccountDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &AccountDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func newConfig(t *testing.T, ctx context.Context, model AccountDataSourceModel) tfsdk.Config {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	(&AccountDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema produced diagnostics: %v", resp.Diagnostics)
	}

	// tfsdk.Config has no Set, so the raw value is built through a State and
	// then handed to the Config.
	st := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := st.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	return tfsdk.Config{Schema: resp.Schema, Raw: st.Raw}
}

func newEmptyState(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	(&AccountDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	return tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) AccountDataSourceModel {
	t.Helper()
	var out AccountDataSourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func nullConfigModel() AccountDataSourceModel {
	return AccountDataSourceModel{
		ID:               types.StringNull(),
		Name:             types.StringNull(),
		Type:             types.StringNull(),
		Status:           types.StringNull(),
		Timezone:         types.StringNull(),
		CreationDate:     types.Int64Null(),
		NextBillingDate:  types.StringNull(),
		DefaultLocations: types.ListNull(types.StringType),
	}
}

func TestMetadataAndSchema(t *testing.T) {
	ctx := context.Background()

	metaResp := &datasource.MetadataResponse{}
	(&AccountDataSource{}).Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_account" {
		t.Errorf("expected 'nodeping_account', got %q", metaResp.TypeName)
	}

	schemaResp := &datasource.SchemaResponse{}
	(&AccountDataSource{}).Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema produced diagnostics: %v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("schema is not a valid implementation: %v", diags)
	}

	id, ok := schemaResp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected an id attribute")
	}
	if !id.IsOptional() || !id.IsComputed() {
		t.Error("id must be optional and computed so it can be omitted for the primary account")
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()

	d := &AccountDataSource{}
	resp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || d.client != nil {
		t.Error("a nil ProviderData must be ignored")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || d.client != c {
		t.Errorf("expected the client to be stored: %v", resp.Diagnostics)
	}

	resp = &datasource.ConfigureResponse{}
	(&AccountDataSource{}).Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for an unexpected ProviderData type")
	}
}

func TestReadByID(t *testing.T) {
	ctx := context.Background()

	var paths []string
	var mu sync.Mutex
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_id":"` + accountID + `","type":"subcustomer",` +
			`"customer_name":"Example Client","creation_date":1332535680000,"status":"Active",` +
			`"timezone":"-7","nextBillingDate":"2026-09-01","defaultlocations":["nam","eur"]}`))
	})

	config := nullConfigModel()
	config.ID = types.StringValue(accountID)

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: newConfig(t, ctx, config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if len(paths) != 1 || paths[0] != "/accounts/"+accountID {
		t.Fatalf("expected a single GET on the account, got %v", paths)
	}

	state := decodeState(t, ctx, resp.State)
	if state.Name.ValueString() != "Example Client" {
		t.Errorf("unexpected name %q", state.Name.ValueString())
	}
	if state.Type.ValueString() != "subcustomer" {
		t.Errorf("unexpected type %q", state.Type.ValueString())
	}
	if state.CreationDate.ValueInt64() != 1332535680000 {
		t.Errorf("unexpected creation_date %d", state.CreationDate.ValueInt64())
	}
	if state.NextBillingDate.ValueString() != "2026-09-01" {
		t.Errorf("unexpected next_billing_date %q", state.NextBillingDate.ValueString())
	}
	if len(state.DefaultLocations.Elements()) != 2 {
		t.Errorf("unexpected default_locations %v", state.DefaultLocations)
	}
}

func TestReadWithoutIDResolvesThePrimaryAccount(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/accounts" {
			_, _ = w.Write([]byte(`{"PRIMARY":{"name":"Primary","status":"Active","count":3,"parent":1},` +
				`"SUB":{"name":"Sub","status":"Active","count":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"_id":"PRIMARY","type":"customer","customer_name":"Primary","status":"Active","timezone":"0"}`))
	})

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: newConfig(t, ctx, nullConfigModel())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if state.ID.ValueString() != "PRIMARY" {
		t.Errorf("expected the primary account id to be filled in, got %q", state.ID.ValueString())
	}
	if state.Timezone.ValueString() != "0" {
		t.Errorf("unexpected timezone %q", state.Timezone.ValueString())
	}
}

func TestReadReportsErrorForTheRequestedAccount(t *testing.T) {
	ctx := context.Background()
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	config := nullConfigModel()
	config.ID = types.StringValue(accountID)

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: newConfig(t, ctx, config)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "account ID "+accountID) {
		t.Errorf("the diagnostic must name the account: %v", resp.Diagnostics)
	}
}

func TestReadReportsErrorForThePrimaryAccount(t *testing.T) {
	ctx := context.Background()
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{Config: newConfig(t, ctx, nullConfigModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "the primary account") {
		t.Errorf("the diagnostic must say which account failed: %v", resp.Diagnostics)
	}
}
