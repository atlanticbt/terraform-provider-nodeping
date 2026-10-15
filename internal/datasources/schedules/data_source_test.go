package schedules

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

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

func newTestDataSource(t *testing.T, handler http.HandlerFunc) *SchedulesDataSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &SchedulesDataSource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func schemaFor(t *testing.T, ctx context.Context) datasource.SchemaResponse {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	(&SchedulesDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return *resp
}

// testConfig builds a datasource config holding the given model. tfsdk.Config
// has no Set method, so the raw value is produced through a tfsdk.State first.
func testConfig(t *testing.T, ctx context.Context, model SchedulesDataSourceModel) tfsdk.Config {
	t.Helper()
	s := schemaFor(t, ctx).Schema

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("config diagnostics: %v", diags)
	}

	return tfsdk.Config{Schema: s, Raw: state.Raw}
}

func newResponse(t *testing.T, ctx context.Context) *datasource.ReadResponse {
	t.Helper()
	s := schemaFor(t, ctx).Schema
	return &datasource.ReadResponse{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
	}
}

func TestMetadata(t *testing.T) {
	resp := &datasource.MetadataResponse{}
	(&SchedulesDataSource{}).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)
	if resp.TypeName != "nodeping_schedules" {
		t.Fatalf("unexpected type name %q", resp.TypeName)
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()
	d := &SchedulesDataSource{}

	d.Configure(ctx, datasource.ConfigureRequest{}, &datasource.ConfigureResponse{})
	if d.client != nil {
		t.Fatal("a nil ProviderData must leave the data source unconfigured")
	}

	badResp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: "nope"}, badResp)
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

func TestMapSchedules_IsSortedAndDeterministic(t *testing.T) {
	api := map[string]client.Schedule{
		"Weekends":      {Data: map[string]client.ScheduleDay{"saturday": {AllDay: true}}},
		"BusinessHours": {ID: "BusinessHours", Data: map[string]client.ScheduleDay{"monday": {Time1: "8:00", Time2: "18:00"}}},
		"Nights":        {Data: map[string]client.ScheduleDay{"monday": {Time1: "18:00", Time2: "8:00", Exclude: true}}},
	}

	for i := 0; i < 200; i++ {
		got := MapSchedules(api)
		if len(got) != 3 {
			t.Fatalf("got %d schedules", len(got))
		}
		if got[0].ID.ValueString() != "BusinessHours" ||
			got[1].ID.ValueString() != "Nights" ||
			got[2].ID.ValueString() != "Weekends" {
			t.Fatalf("iteration %d: unsorted schedules %v %v %v",
				i, got[0].ID.ValueString(), got[1].ID.ValueString(), got[2].ID.ValueString())
		}
	}
}

func TestMapSchedules_FillsTheIDFromTheMapKeyAndNullsUnscheduledDays(t *testing.T) {
	got := MapSchedules(map[string]client.Schedule{
		"Weekends": {Data: map[string]client.ScheduleDay{"saturday": {AllDay: true}}},
	})

	if len(got) != 1 || got[0].ID.ValueString() != "Weekends" {
		t.Fatalf("unexpected result %+v", got)
	}
	if got[0].Saturday == nil || !got[0].Saturday.AllDay.ValueBool() {
		t.Fatalf("saturday mapped as %+v", got[0].Saturday)
	}
	if got[0].Monday != nil || got[0].Sunday != nil {
		t.Fatal("unscheduled days must be null")
	}
}

func TestMapSchedules_HandlesAnEmptyAccount(t *testing.T) {
	got := MapSchedules(map[string]client.Schedule{})
	if got == nil || len(got) != 0 {
		t.Fatalf("expected an empty list, got %v", got)
	}
}

func TestRead_ListsSchedules(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/schedules" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if _, err := io.WriteString(w, `{"Days":{"data":{"monday":{"time1":"6:00","time2":"18:00"}}},"Nights":{"data":{"monday":{"time1":"6:00","time2":"18:00","exclude":1}}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, SchedulesDataSourceModel{})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got SchedulesDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}

	if len(got.Schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(got.Schedules))
	}
	if got.Schedules[0].ID.ValueString() != "Days" || got.Schedules[1].ID.ValueString() != "Nights" {
		t.Fatalf("unexpected order: %v", got.Schedules)
	}
	if !got.Schedules[1].Monday.Exclude.ValueBool() {
		t.Fatalf("exclude not decoded: %+v", got.Schedules[1].Monday)
	}
}

func TestRead_ScopesTheListingToTheSubAccount(t *testing.T) {
	ctx := context.Background()

	var query string
	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		if _, err := io.WriteString(w, `{}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, SchedulesDataSourceModel{
		CustomerID: types.StringValue("SUB1"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	if !strings.Contains(query, "customerid=SUB1") {
		t.Fatalf("expected the SubAccount in the query, got %q", query)
	}
}

func TestRead_ReportsAPIFailures(t *testing.T) {
	ctx := context.Background()

	d := newTestDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	resp := newResponse(t, ctx)
	d.Read(ctx, datasource.ReadRequest{Config: testConfig(t, ctx, SchedulesDataSourceModel{})}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
}

func TestClientFor_OnlyScopesWhenACustomerIDIsSet(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	d := &SchedulesDataSource{client: c}

	for _, value := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
		if d.clientFor(value) != c {
			t.Fatalf("expected the provider client for %v", value)
		}
	}
	if d.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client")
	}
}
