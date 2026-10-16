package schedule

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *ScheduleDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ScheduleDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

// testConfig builds a datasource config holding the given model. tfsdk.Config
// has no Set method, so the raw value is produced through a tfsdk.State first.
func testConfig(t *testing.T, ctx context.Context, model ScheduleDataSourceModel) tfsdk.Config {
	t.Helper()
	d := &ScheduleDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}

	return tfsdk.Config{Schema: resp.Schema, Raw: state.Raw}
}

func newResponse(t *testing.T, ctx context.Context) *datasource.ReadResponse {
	t.Helper()
	d := &ScheduleDataSource{}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	return &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
}

func TestMetadata(t *testing.T) {
	resp := &datasource.MetadataResponse{}
	(&ScheduleDataSource{}).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)
	if resp.TypeName != "nodeping_schedule" {
		t.Fatalf("unexpected type name %q", resp.TypeName)
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()
	d := &ScheduleDataSource{}

	d.Configure(ctx, datasource.ConfigureRequest{}, &datasource.ConfigureResponse{})
	if d.client != nil {
		t.Fatal("a nil ProviderData must leave the data source unconfigured")
	}

	badResp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, badResp)
	if !badResp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong ProviderData type")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	okResp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, okResp)
	if okResp.Diagnostics.HasError() || d.client != c {
		t.Fatalf("expected the client to be stored: %v", okResp.Diagnostics)
	}
}

func TestRead_PopulatesConfiguredDaysAndLeavesTheRestNull(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"},"saturday":{"disabled":1},"sunday":{"allday":true}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, ScheduleDataSourceModel{
		ID: types.StringValue("BusinessHours"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ScheduleDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}

	if got.Monday == nil || got.Monday.Time1.ValueString() != "8:00" || got.Monday.Time2.ValueString() != "18:00" {
		t.Fatalf("monday mapped as %+v", got.Monday)
	}
	if got.Monday.Exclude.ValueBool() || got.Monday.Disabled.ValueBool() || got.Monday.AllDay.ValueBool() {
		t.Fatalf("monday flags should all be false: %+v", got.Monday)
	}
	if got.Saturday == nil || !got.Saturday.Disabled.ValueBool() {
		t.Fatalf("saturday mapped as %+v", got.Saturday)
	}
	if got.Sunday == nil || !got.Sunday.AllDay.ValueBool() || !got.Sunday.Time1.IsNull() {
		t.Fatalf("sunday mapped as %+v", got.Sunday)
	}
	if got.Tuesday != nil || got.Wednesday != nil || got.Thursday != nil || got.Friday != nil {
		t.Fatal("unscheduled days must be null")
	}
	if !got.CustomerID.IsNull() {
		t.Fatalf("customer_id should be null, got %v", got.CustomerID)
	}
}

func TestRead_ScopesTheLookupToTheSubAccount(t *testing.T) {
	ctx := context.Background()

	var query string
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		if _, err := io.WriteString(w, `{"id":"Days","data":{"monday":{"allday":true}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, ScheduleDataSourceModel{
		ID:         types.StringValue("Days"),
		CustomerID: types.StringValue("SUB1"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	if !strings.Contains(query, "customerid=SUB1") {
		t.Fatalf("expected the SubAccount in the query, got %q", query)
	}

	var got ScheduleDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("unexpected customer_id %v", got.CustomerID)
	}
}

func TestRead_AdoptsTheCustomerIDTheAPIReports(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"id":"Days","customer_id":"CUSTFROMAPI","data":{"monday":{"allday":true}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, ScheduleDataSourceModel{
		ID: types.StringValue("Days"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ScheduleDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if got.CustomerID.ValueString() != "CUSTFROMAPI" {
		t.Fatalf("unexpected customer_id %v", got.CustomerID)
	}
}

func TestRead_ReportsAMissingSchedule(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, ScheduleDataSourceModel{
		ID: types.StringValue("Missing"),
	})}, resp)

	// Unlike a resource refresh, a data source lookup for a schedule that does
	// not exist is a configuration error and must fail loudly.
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
}

func TestMapDay_ConvertsTimesAndFlags(t *testing.T) {
	got := MapDay(client.ScheduleDay{Time1: "6:00", Time2: "18:00", Exclude: true})
	if got.Time1.ValueString() != "6:00" || got.Time2.ValueString() != "18:00" {
		t.Fatalf("times mapped as %+v", got)
	}
	if !got.Exclude.ValueBool() || got.Disabled.ValueBool() || got.AllDay.ValueBool() {
		t.Fatalf("flags mapped as %+v", got)
	}

	empty := MapDay(client.ScheduleDay{})
	if !empty.Time1.IsNull() || !empty.Time2.IsNull() {
		t.Fatalf("absent times must be null, got %+v", empty)
	}
}

func TestClientFor_OnlyScopesWhenACustomerIDIsSet(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &ScheduleDataSource{client: c}

	for _, value := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
		if d.clientFor(value) != c {
			t.Fatalf("expected the provider client for %v", value)
		}
	}
	if d.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client")
	}
}
