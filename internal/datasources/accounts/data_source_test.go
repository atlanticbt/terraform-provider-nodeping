package accounts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *AccountsDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &AccountsDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func newEmptyState(t *testing.T, ctx context.Context) tfsdk.State {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	(&AccountsDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema produced diagnostics: %v", resp.Diagnostics)
	}
	return tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) AccountsDataSourceModel {
	t.Helper()
	var out AccountsDataSourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func sampleListing() map[string]client.AccountSummary {
	subaccounts := 2
	return map[string]client.AccountSummary{
		"ZZZ-SUB":  {Name: "Zed", Status: "Suspend", Count: 1},
		"AAA-SUB":  {Name: "Alpha", Status: "Active", Count: 7},
		"MMM-PRIM": {Name: "Primary", Status: "Active", Count: 40, Parent: &subaccounts},
	}
}

func TestMetadataAndSchema(t *testing.T) {
	ctx := context.Background()

	metaResp := &datasource.MetadataResponse{}
	(&AccountsDataSource{}).Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_accounts" {
		t.Errorf("expected 'nodeping_accounts', got %q", metaResp.TypeName)
	}

	schemaResp := &datasource.SchemaResponse{}
	(&AccountsDataSource{}).Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema produced diagnostics: %v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("schema is not a valid implementation: %v", diags)
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()

	d := &AccountsDataSource{}
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
	(&AccountsDataSource{}).Configure(ctx, datasource.ConfigureRequest{ProviderData: struct{}{}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for an unexpected ProviderData type")
	}
}

func TestMapAccountsIsSortedAndDeterministic(t *testing.T) {
	listing := sampleListing()

	// The API returns a map, so ordering has to come from the provider. Go
	// randomizes map iteration, which this loop is designed to expose.
	for i := 0; i < 200; i++ {
		got := mapAccounts(listing)
		if len(got) != 3 {
			t.Fatalf("expected 3 accounts, got %d", len(got))
		}
		if got[0].ID.ValueString() != "AAA-SUB" ||
			got[1].ID.ValueString() != "MMM-PRIM" ||
			got[2].ID.ValueString() != "ZZZ-SUB" {
			t.Fatalf("iteration %d: accounts out of order: %v %v %v",
				i, got[0].ID, got[1].ID, got[2].ID)
		}
	}
}

func TestMapAccountsReportsSubaccountCountOnlyForThePrimary(t *testing.T) {
	got := mapAccounts(sampleListing())

	primary := got[1]
	if primary.SubaccountCount.IsNull() || primary.SubaccountCount.ValueInt64() != 2 {
		t.Errorf("expected the primary account to report 2 subaccounts, got %v", primary.SubaccountCount)
	}
	if primary.ActiveChecks.ValueInt64() != 40 {
		t.Errorf("expected 40 active checks, got %d", primary.ActiveChecks.ValueInt64())
	}

	for _, i := range []int{0, 2} {
		if !got[i].SubaccountCount.IsNull() {
			t.Errorf("subaccount %q must have a null subaccount_count, got %v",
				got[i].ID.ValueString(), got[i].SubaccountCount)
		}
	}
}

func TestMapAccountsHandlesEmptyListing(t *testing.T) {
	got := mapAccounts(map[string]client.AccountSummary{})
	if got == nil || len(got) != 0 {
		t.Errorf("expected an empty, non-nil list, got %v", got)
	}
}

func TestReadPopulatesState(t *testing.T) {
	ctx := context.Background()
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			t.Errorf("expected path /accounts, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(sampleListing()); err != nil {
			t.Fatalf("encode failed: %v", err)
		}
	})

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if len(state.Accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(state.Accounts))
	}
	if state.Accounts[0].Name.ValueString() != "Alpha" {
		t.Errorf("unexpected first account %q", state.Accounts[0].Name.ValueString())
	}
	if state.Accounts[2].Status.ValueString() != "Suspend" {
		t.Errorf("unexpected status %q", state.Accounts[2].Status.ValueString())
	}
}

func TestReadReportsAPIFailure(t *testing.T) {
	ctx := context.Background()
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	resp := &datasource.ReadResponse{State: newEmptyState(t, ctx)}
	d.Read(ctx, datasource.ReadRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if resp.Diagnostics.Errors()[0].Summary() != "Error Reading Accounts" {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}
}
