package check

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func testContext() context.Context { return context.Background() }

func nullRaw(ctx context.Context) tftypes.Value {
	return tftypes.NewValue(CheckSchema().Type().TerraformType(ctx), nil)
}

// withTypedNulls fills in the element type of any collection attribute left at
// its Go zero value. A zero types.Map/Set/List carries no element type, which
// the framework rejects when writing it back into a plan or state.
func withTypedNulls(m CheckResourceModel) CheckResourceModel {
	if m.Tags.IsNull() && m.Tags.ElementType(context.Background()) == nil {
		m.Tags = types.SetNull(types.StringType)
	}
	if m.TagsAll.IsNull() && m.TagsAll.ElementType(context.Background()) == nil {
		m.TagsAll = types.SetNull(types.StringType)
	}
	for _, f := range []*types.List{&m.RunLocations} {
		if f.IsNull() && f.ElementType(context.Background()) == nil {
			*f = types.ListNull(types.StringType)
		}
	}
	stringMaps := []*types.Map{&m.SendHeaders, &m.ReceiveHeaders, &m.Data, &m.EDNS}
	for _, f := range stringMaps {
		if f.IsNull() && f.ElementType(context.Background()) == nil {
			*f = types.MapNull(types.StringType)
		}
	}
	if m.Fields.IsNull() && m.Fields.ElementType(context.Background()) == nil {
		m.Fields = types.MapNull(CheckFieldObjectType())
	}
	if m.Hosts.IsNull() && m.Hosts.ElementType(context.Background()) == nil {
		m.Hosts = types.MapNull(RedisHostObjectType())
	}
	return m
}

func newPlan(t *testing.T, ctx context.Context, m CheckResourceModel) tfsdk.Plan {
	t.Helper()
	m = withTypedNulls(m)
	p := tfsdk.Plan{Schema: CheckSchema(), Raw: nullRaw(ctx)}
	if diags := p.Set(ctx, &m); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func newConfig(t *testing.T, ctx context.Context, m CheckResourceModel) tfsdk.Config {
	t.Helper()
	p := newPlan(t, ctx, m)
	return tfsdk.Config{Schema: CheckSchema(), Raw: p.Raw}
}

// withoutWriteOnly strips every write-only attribute, which is what Terraform
// core does to the plan and the state before they reach the provider.
func withoutWriteOnly(m CheckResourceModel) CheckResourceModel {
	m.PasswordWO = types.StringNull()
	return m
}

// newCreateRequest mirrors what the framework hands to Create: the configured
// values in Config, the same values minus the write-only ones in Plan.
func newCreateRequest(t *testing.T, ctx context.Context, m CheckResourceModel) resource.CreateRequest {
	t.Helper()
	return resource.CreateRequest{
		Plan:   newPlan(t, ctx, withoutWriteOnly(m)),
		Config: newConfig(t, ctx, m),
	}
}

func newUpdateRequest(t *testing.T, ctx context.Context, plan, state CheckResourceModel) resource.UpdateRequest {
	t.Helper()
	return resource.UpdateRequest{
		Plan:   newPlan(t, ctx, withoutWriteOnly(plan)),
		Config: newConfig(t, ctx, plan),
		State:  newState(t, ctx, withoutWriteOnly(state)),
	}
}

func newState(t *testing.T, ctx context.Context, m CheckResourceModel) tfsdk.State {
	t.Helper()
	m = withTypedNulls(m)
	s := tfsdk.State{Schema: CheckSchema(), Raw: nullRaw(ctx)}
	if diags := s.Set(ctx, &m); diags.HasError() {
		t.Fatalf("failed to build state: %v", diags)
	}
	return s
}

func emptyState(ctx context.Context) tfsdk.State {
	return tfsdk.State{Schema: CheckSchema(), Raw: nullRaw(ctx)}
}

func stateModel(t *testing.T, ctx context.Context, s tfsdk.State) CheckResourceModel {
	t.Helper()
	var m CheckResourceModel
	if diags := s.Get(ctx, &m); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return m
}

// baseModel returns a model where every computed-only attribute is unknown, as
// Terraform would present it during Create.
func baseModel() CheckResourceModel {
	return CheckResourceModel{
		ID:             types.StringUnknown(),
		CustomerID:     types.StringUnknown(),
		Type:           types.StringValue("HTTP"),
		Target:         types.StringValue("https://example.com"),
		Label:          types.StringUnknown(),
		Enabled:        types.BoolValue(true),
		Public:         types.BoolValue(false),
		Interval:       types.Float64Value(5),
		Threshold:      types.Int64Value(5),
		Sens:           types.Int64Value(2),
		Mute:           types.BoolValue(false),
		Dep:            types.StringNull(),
		Description:    types.StringNull(),
		RunLocations:   types.ListUnknown(types.StringType),
		HomeLoc:        types.StringUnknown(),
		AutoDiag:       types.BoolValue(false),
		Tags:           types.SetUnknown(types.StringType),
		State:          types.Int64Unknown(),
		Created:        types.Int64Unknown(),
		Modified:       types.Int64Unknown(),
		ContentString:  types.StringNull(),
		Regex:          types.BoolUnknown(),
		Invert:         types.BoolUnknown(),
		Follow:         types.BoolUnknown(),
		Method:         types.StringUnknown(),
		StatusCode:     types.Int64Unknown(),
		SendHeaders:    types.MapNull(types.StringType),
		ReceiveHeaders: types.MapNull(types.StringType),
		PostData:       types.StringNull(),
		Port:           types.Int64Unknown(),
		Username:       types.StringNull(),
		Password:       types.StringNull(),
		Secure:         types.StringUnknown(),
		Verify:         types.BoolUnknown(),
		IPv6:           types.BoolUnknown(),
		DNSType:        types.StringUnknown(),
		DNSToResolve:   types.StringNull(),
		DNSSection:     types.StringUnknown(),
		DNSRD:          types.BoolUnknown(),
		Transport:      types.StringUnknown(),
		WarningDays:    types.Int64Unknown(),
		ServerName:     types.StringNull(),
		Email:          types.StringNull(),
		Database:       types.StringNull(),
		Query:          types.StringNull(),
		Namespace:      types.StringNull(),
		SSHKey:         types.StringNull(),
		ClientCert:     types.StringNull(),
		SNMPv:          types.StringUnknown(),
		SNMPCom:        types.StringUnknown(),

		// Write-only attributes are null in the plan by construction.
		PasswordWO:        types.StringNull(),
		PasswordWOVersion: types.Int64Null(),

		Data:          types.MapNull(types.StringType),
		Fields:        types.MapNull(CheckFieldObjectType()),
		Hosts:         types.MapNull(RedisHostObjectType()),
		RedisType:     types.StringValue("standalone"),
		SentinelName:  types.StringNull(),
		CheckToken:    types.StringUnknown(),
		OldResultFail: types.BoolUnknown(),
		EDNS:          types.MapNull(types.StringType),
		DoHDoT:        types.StringValue("doh"),
		WhoisServer:   types.StringNull(),
		RDAPUrl:       types.StringNull(),
		Ignore:        types.StringNull(),
		VerifyVolume:  types.BoolUnknown(),
		VolumeMin:     types.Int64Unknown(),
	}
}

// mockAPI is a tiny stand-in for the NodePing check API. The transform hook
// lets a test express how the server "normalises" what it was sent.
type mockAPI struct {
	t         *testing.T
	server    *httptest.Server
	stored    map[string]map[string]interface{}
	lastBody  map[string]interface{}
	lastQuery string
	transform func(body map[string]interface{}, stored map[string]interface{})
	notFound  bool
}

func newMockAPI(t *testing.T) *mockAPI {
	t.Helper()
	m := &mockAPI{t: t, stored: map[string]map[string]interface{}{}}

	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		m.lastQuery = r.URL.RawQuery
		id := ""
		if len(r.URL.Path) > len("/checks/") && r.URL.Path[:8] == "/checks/" {
			id = r.URL.Path[8:]
		}

		if m.notFound {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}

		switch r.Method {
		case http.MethodGet:
			if id == "" {
				writeJSON(w, m.stored)
				return
			}
			check, ok := m.stored[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			writeJSON(w, check)

		case http.MethodPost, http.MethodPut:
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			m.lastBody = body
			if id == "" {
				id = "MOCK-CHECK-1"
			}
			check := m.checkFromBody(id, body)
			if m.transform != nil {
				m.transform(body, check)
			}
			m.stored[id] = check
			writeJSON(w, check)

		case http.MethodDelete:
			delete(m.stored, id)
			writeJSON(w, map[string]interface{}{"ok": true, "id": id})
		}
	}
	mux.HandleFunc("/checks", handler)
	mux.HandleFunc("/checks/", handler)

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockAPI) checkFromBody(id string, body map[string]interface{}) map[string]interface{} {
	params := map[string]interface{}{}
	for _, k := range []string{
		"target", "threshold", "sens", "contentstring", "regex", "invert", "follow",
		"method", "statuscode", "sendheaders", "receiveheaders", "postdata", "port",
		"username", "secure", "verify", "ipv6", "dnstype", "dnstoresolve", "dnssection",
		"dnsrd", "transport", "warningdays", "servername", "email", "database", "query",
		"namespace", "sshkey", "clientcert", "snmpv", "snmpcom",
		// NodePing echoes the password back in plaintext; the mock does the
		// same so the read path can be tested for leaks.
		"password",
		"data", "fields", "hosts", "redistype", "sentinelname", "checktoken",
		"oldresultfail", "edns", "dohdot", "whoisserver", "rdapurl", "ignore",
		"verifyvolume", "volumemin",
	} {
		if v, ok := body[k]; ok {
			params[k] = v
		}
	}

	check := map[string]interface{}{
		"_id":         id,
		"customer_id": "ACCT-1",
		"type":        body["type"],
		"label":       body["label"],
		"enable":      body["enabled"],
		"interval":    body["interval"],
		"created":     float64(1000),
		"modified":    float64(time.Now().UnixMilli()),
		"state":       float64(1),
		"parameters":  params,
	}
	for _, k := range []string{"public", "autodiag", "mute", "dep", "description", "tags", "runlocations", "homeloc", "notifications"} {
		if v, ok := body[k]; ok {
			check[k] = v
		}
	}
	return check
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (m *mockAPI) resource(defaultTags ...string) *CheckResource {
	c := client.NewClient(client.ClientConfig{
		APIToken:     "token",
		BaseURL:      m.server.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		DefaultTags:  defaultTags,
	})
	return &CheckResource{client: c}
}

// ---------------------------------------------------------------------------
// mapping: read path
// ---------------------------------------------------------------------------

func fullAPICheck() *client.Check {
	return &client.Check{
		ID:           "CHK-1",
		CustomerID:   "ACCT-1",
		Type:         "HTTPADV",
		Label:        "web",
		Interval:     json.Number("5"),
		Enabled:      "active",
		Public:       true,
		Modified:     222,
		Created:      111,
		State:        1,
		Dep:          "CHK-DEP",
		Mute:         false,
		Description:  "notes",
		RunLocations: []interface{}{"nam", "eur"},
		HomeLoc:      "nam",
		AutoDiag:     true,
		Tags:         []string{"a", "b"},
		Notifications: []map[string]interface{}{
			{"CONTACT-1": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
		},
		Parameters: client.CheckParameters{
			Target:         "https://example.com/",
			Threshold:      "10",
			Sens:           float64(3),
			ContentString:  "ok",
			Regex:          "true",
			Invert:         float64(1),
			Follow:         true,
			Method:         "POST",
			StatusCode:     "204",
			SendHeaders:    map[string]string{"Authorization": "Bearer x"},
			ReceiveHeaders: map[string]string{"Server": "nginx"},
			PostData:       "a=b",
			Port:           "443",
			Username:       "user",
			Password:       "",
			Secure:         "ssl",
			Verify:         true,
			IPv6:           false,
			DNSType:        "A",
			DNSToResolve:   "example.com",
			DNSSection:     "answer",
			DNSRD:          true,
			Transport:      "udp",
			WarningDays:    "30",
			ServerName:     "example.com",
			Email:          "a@example.com",
			Database:       "db",
			Query:          "select 1",
			Namespace:      "ns",
			SSHKey:         "KEY-1",
			ClientCert:     "CERT-1",
			SNMPv:          "2c",
			SNMPCom:        "public",
		},
	}
}

func TestMapCheckToModel_MapsEveryAttribute(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics
	model := &CheckResourceModel{}
	mapCheckToModel(ctx, fullAPICheck(), model, &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"id", model.ID.ValueString(), "CHK-1"},
		{"customer_id", model.CustomerID.ValueString(), "ACCT-1"},
		{"type", model.Type.ValueString(), "HTTPADV"},
		{"label", model.Label.ValueString(), "web"},
		{"enabled", model.Enabled.ValueBool(), true},
		{"public", model.Public.ValueBool(), true},
		{"autodiag", model.AutoDiag.ValueBool(), true},
		{"interval", model.Interval.ValueFloat64(), float64(5)},
		{"state", model.State.ValueInt64(), int64(1)},
		{"created", model.Created.ValueInt64(), int64(111)},
		{"modified", model.Modified.ValueInt64(), int64(222)},
		{"description", model.Description.ValueString(), "notes"},
		{"dep", model.Dep.ValueString(), "CHK-DEP"},
		{"homeloc", model.HomeLoc.ValueString(), "nam"},
		{"target", model.Target.ValueString(), "https://example.com/"},
		{"threshold", model.Threshold.ValueInt64(), int64(10)},
		{"sens", model.Sens.ValueInt64(), int64(3)},
		{"contentstring", model.ContentString.ValueString(), "ok"},
		{"regex", model.Regex.ValueBool(), true},
		{"invert", model.Invert.ValueBool(), true},
		{"follow", model.Follow.ValueBool(), true},
		{"method", model.Method.ValueString(), "POST"},
		{"statuscode", model.StatusCode.ValueInt64(), int64(204)},
		{"postdata", model.PostData.ValueString(), "a=b"},
		{"port", model.Port.ValueInt64(), int64(443)},
		{"username", model.Username.ValueString(), "user"},
		{"secure", model.Secure.ValueString(), "ssl"},
		{"verify", model.Verify.ValueBool(), true},
		{"ipv6", model.IPv6.ValueBool(), false},
		{"dnstype", model.DNSType.ValueString(), "A"},
		{"dnstoresolve", model.DNSToResolve.ValueString(), "example.com"},
		{"dnssection", model.DNSSection.ValueString(), "answer"},
		{"dnsrd", model.DNSRD.ValueBool(), true},
		{"transport", model.Transport.ValueString(), "udp"},
		{"warningdays", model.WarningDays.ValueInt64(), int64(30)},
		{"servername", model.ServerName.ValueString(), "example.com"},
		{"email", model.Email.ValueString(), "a@example.com"},
		{"database", model.Database.ValueString(), "db"},
		{"query", model.Query.ValueString(), "select 1"},
		{"namespace", model.Namespace.ValueString(), "ns"},
		{"sshkey", model.SSHKey.ValueString(), "KEY-1"},
		{"clientcert", model.ClientCert.ValueString(), "CERT-1"},
		{"snmpv", model.SNMPv.ValueString(), "2c"},
		{"snmpcom", model.SNMPCom.ValueString(), "public"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	if model.Password.IsUnknown() || !model.Password.IsNull() {
		t.Errorf("password should be null when the API does not return it, got %v", model.Password)
	}
	if l := len(model.RunLocations.Elements()); l != 2 {
		t.Errorf("runlocations length = %d, want 2", l)
	}
	if l := len(model.Tags.Elements()); l != 2 {
		t.Errorf("tags length = %d, want 2", l)
	}
	if l := len(model.SendHeaders.Elements()); l != 1 {
		t.Errorf("sendheaders length = %d, want 1", l)
	}
	if l := len(model.ReceiveHeaders.Elements()); l != 1 {
		t.Errorf("receiveheaders length = %d, want 1", l)
	}
	if len(model.Notifications) != 1 || model.Notifications[0].ContactID.ValueString() != "CONTACT-1" {
		t.Errorf("notifications = %#v", model.Notifications)
	}
}

func TestMapCheckToModel_UnsetOptionalsAreNull(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics
	model := &CheckResourceModel{}
	mapCheckToModel(ctx, &client.Check{
		ID:           "CHK-2",
		Type:         "PING",
		Enabled:      "false",
		Dep:          false,
		HomeLoc:      false,
		RunLocations: false,
		Parameters: client.CheckParameters{
			Target:      "example.com",
			WarningDays: false,
			Port:        "",
			SSHKey:      false,
			ClientCert:  false,
		},
	}, model, &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	nulls := map[string]bool{
		"dep":          model.Dep.IsNull(),
		"homeloc":      model.HomeLoc.IsNull(),
		"runlocations": model.RunLocations.IsNull(),
		"warningdays":  model.WarningDays.IsNull(),
		"port":         model.Port.IsNull(),
		"sshkey":       model.SSHKey.IsNull(),
		"clientcert":   model.ClientCert.IsNull(),
		"regex":        model.Regex.IsNull(),
		"tags":         model.Tags.IsNull(),
		"sendheaders":  model.SendHeaders.IsNull(),
	}
	for name, isNull := range nulls {
		if !isNull {
			t.Errorf("%s should be null", name)
		}
	}
	if model.Enabled.ValueBool() {
		t.Error("enabled should be false")
	}
	if model.Notifications != nil {
		t.Errorf("notifications should be nil, got %#v", model.Notifications)
	}
}

func TestInt64Decoding(t *testing.T) {
	cases := []struct {
		name    string
		in      interface{}
		want    int64
		null    bool
		wantErr bool
	}{
		{name: "float", in: float64(10), want: 10},
		{name: "string", in: "10", want: 10},
		{name: "string with spaces", in: " 10 ", want: 10},
		{name: "json.Number", in: json.Number("10"), want: 10},
		{name: "float string", in: "10.9", want: 10},
		{name: "int", in: 10, want: 10},
		{name: "nil", in: nil, null: true},
		{name: "empty string", in: "", null: true},
		{name: "false means unset", in: false, null: true},
		{name: "garbage", in: "abc", null: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := int64OrNull(tc.in, "threshold", "CHK-1", &diags)
			if diags.HasError() != tc.wantErr {
				t.Fatalf("diagnostics error = %v, want %v (%v)", diags.HasError(), tc.wantErr, diags)
			}
			if tc.null {
				if !got.IsNull() {
					t.Fatalf("expected null, got %v", got)
				}
				return
			}
			if got.ValueInt64() != tc.want {
				t.Fatalf("got %v, want %v", got.ValueInt64(), tc.want)
			}
		})
	}
}

func TestParseBoolInterface(t *testing.T) {
	cases := []struct {
		in   interface{}
		want bool
	}{
		{nil, false}, {true, true}, {false, false}, {"true", true}, {"1", true},
		{"false", false}, {float64(1), true}, {float64(0), false}, {1, true},
		{json.Number("1"), true}, {json.Number("0"), false}, {struct{}{}, false},
	}
	for _, c := range cases {
		if got := parseBoolInterface(c.in); got != c.want {
			t.Errorf("parseBoolInterface(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRunLocationsToList(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	if l := runLocationsToList(ctx, []string{"nam"}, &diags); len(l.Elements()) != 1 {
		t.Errorf("[]string not handled: %v", l)
	}
	if l := runLocationsToList(ctx, "nam", &diags); len(l.Elements()) != 1 {
		t.Errorf("string not handled: %v", l)
	}
	if l := runLocationsToList(ctx, []interface{}{"nam", 3}, &diags); len(l.Elements()) != 1 {
		t.Errorf("mixed slice not handled: %v", l)
	}
	if l := runLocationsToList(ctx, false, &diags); !l.IsNull() {
		t.Errorf("false should map to null, got %v", l)
	}
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// ---------------------------------------------------------------------------
// mapping: apply path
// ---------------------------------------------------------------------------

func TestMapForApply_PreservesPlannedValuesAndFillsUnknowns(t *testing.T) {
	ctx := testContext()

	plan := baseModel()
	plan.Password = types.StringValue("s3cret")
	plan.RunLocations = mustList(t, ctx, "nam")
	plan.ContentString = types.StringValue("hello")
	plan.Port = types.Int64Value(443)
	plan.WarningDays = types.Int64Value(30)
	plan.Follow = types.BoolValue(true)
	plan.Method = types.StringValue("POST")
	plan.Description = types.StringValue("planned description")
	plan.Dep = types.StringValue("CHK-DEP")
	plan.Target = types.StringValue("https://example.com")

	// The API contradicts every one of those values.
	apiCheck := &client.Check{
		ID:           "CHK-1",
		CustomerID:   "ACCT-1",
		Type:         "HTTP",
		Label:        "server-label",
		Enabled:      "active",
		Interval:     json.Number("5"),
		Created:      111,
		Modified:     222,
		State:        1,
		RunLocations: false,
		Dep:          "",
		Description:  "",
		Parameters: client.CheckParameters{
			Target:        "https://example.com/",
			ContentString: "",
			Port:          "80",
			WarningDays:   false,
			Follow:        float64(0),
			Method:        "post",
			Password:      "",
		},
	}

	var diags diag.Diagnostics
	got := mapForApply(ctx, apiCheck, &plan, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.Password.ValueString() != "s3cret" {
		t.Errorf("password = %v, want s3cret", got.Password)
	}
	if len(got.RunLocations.Elements()) != 1 {
		t.Errorf("runlocations = %v, want the planned [nam]", got.RunLocations)
	}
	if got.ContentString.ValueString() != "hello" {
		t.Errorf("contentstring = %v, want hello", got.ContentString)
	}
	if got.Port.ValueInt64() != 443 {
		t.Errorf("port = %v, want 443", got.Port)
	}
	if got.WarningDays.ValueInt64() != 30 {
		t.Errorf("warningdays = %v, want 30", got.WarningDays)
	}
	if !got.Follow.ValueBool() {
		t.Errorf("follow = %v, want true", got.Follow)
	}
	if got.Method.ValueString() != "POST" {
		t.Errorf("method = %v, want POST", got.Method)
	}
	if got.Description.ValueString() != "planned description" {
		t.Errorf("description = %v", got.Description)
	}
	if got.Dep.ValueString() != "CHK-DEP" {
		t.Errorf("dep = %v", got.Dep)
	}
	if got.Target.ValueString() != "https://example.com" {
		t.Errorf("target = %v, want the planned value", got.Target)
	}

	// Unknown attributes must come from the API.
	if got.ID.ValueString() != "CHK-1" {
		t.Errorf("id = %v", got.ID)
	}
	if got.CustomerID.ValueString() != "ACCT-1" {
		t.Errorf("customer_id = %v", got.CustomerID)
	}
	if got.Label.ValueString() != "server-label" {
		t.Errorf("label = %v", got.Label)
	}
	if got.Created.ValueInt64() != 111 || got.Modified.ValueInt64() != 222 || got.State.ValueInt64() != 1 {
		t.Errorf("computed timestamps not filled: %v %v %v", got.Created, got.Modified, got.State)
	}
	// Optional attributes that were neither configured nor returned stay null.
	if !got.Username.IsNull() {
		t.Errorf("username = %v, want null", got.Username)
	}
}

func mustList(t *testing.T, ctx context.Context, values ...string) types.List {
	t.Helper()
	l, diags := types.ListValueFrom(ctx, types.StringType, values)
	if diags.HasError() {
		t.Fatalf("bad list: %v", diags)
	}
	return l
}

func mustSet(t *testing.T, ctx context.Context, values ...string) types.Set {
	t.Helper()
	s, diags := types.SetValueFrom(ctx, types.StringType, values)
	if diags.HasError() {
		t.Fatalf("bad set: %v", diags)
	}
	return s
}

func TestReconcileNotifications(t *testing.T) {
	planned := []NotificationModel{
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(0), Schedule: types.StringUnknown()},
		{ContactID: types.StringValue("B"), Delay: types.Int64Value(5), Schedule: types.StringValue("Days")},
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(10), Schedule: types.StringValue("Nights")},
	}
	api := []NotificationModel{
		{ContactID: types.StringValue("B"), Delay: types.Int64Value(5), Schedule: types.StringValue("Days")},
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(10), Schedule: types.StringValue("Nights")},
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(0), Schedule: types.StringValue("All")},
	}

	got := reconcileNotifications(planned, api)
	if len(got) != len(planned) {
		t.Fatalf("length = %d, want %d (duplicate blocks must not be dropped)", len(got), len(planned))
	}
	want := []string{"A", "B", "A"}
	for i, w := range want {
		if got[i].ContactID.ValueString() != w {
			t.Errorf("element %d contact = %v, want %v", i, got[i].ContactID, w)
		}
	}
	if got[0].Schedule.ValueString() != "All" {
		t.Errorf("unknown schedule should be filled from the API, got %v", got[0].Schedule)
	}
	if got[2].Schedule.ValueString() != "Nights" {
		t.Errorf("planned schedule must be preserved, got %v", got[2].Schedule)
	}
}

func TestReconcileNotifications_NoAPIMatch(t *testing.T) {
	planned := []NotificationModel{
		{ContactID: types.StringValue("A"), Delay: types.Int64Unknown(), Schedule: types.StringUnknown()},
	}
	got := reconcileNotifications(planned, nil)
	if len(got) != 1 {
		t.Fatalf("length = %d", len(got))
	}
	if got[0].Delay.ValueInt64() != 0 || got[0].Schedule.ValueString() != "All" {
		t.Errorf("unmatched unknowns not defaulted: %#v", got[0])
	}
	if reconcileNotifications(nil, nil) != nil {
		t.Error("nil plan must stay nil")
	}
	if got := reconcileNotifications([]NotificationModel{}, api1()); len(got) != 0 {
		t.Errorf("empty plan must stay empty, got %#v", got)
	}
}

func api1() []NotificationModel {
	return []NotificationModel{{ContactID: types.StringValue("A"), Delay: types.Int64Value(1), Schedule: types.StringValue("All")}}
}

func TestAlignNotifications(t *testing.T) {
	prior := []NotificationModel{
		{ContactID: types.StringValue("A")},
		{ContactID: types.StringValue("B")},
	}
	api := []NotificationModel{
		{ContactID: types.StringValue("C")},
		{ContactID: types.StringValue("B")},
		{ContactID: types.StringValue("A")},
	}
	got := alignNotifications(prior, api)
	want := []string{"A", "B", "C"}
	if len(got) != 3 {
		t.Fatalf("length = %d", len(got))
	}
	for i, w := range want {
		if got[i].ContactID.ValueString() != w {
			t.Errorf("element %d = %v, want %v", i, got[i].ContactID, w)
		}
	}
	if got := alignNotifications(nil, api); len(got) != 3 {
		t.Errorf("nil prior should pass the API order through")
	}
	if got := alignNotifications(prior, nil); got != nil {
		t.Errorf("nil API should stay nil")
	}
}

func TestMapNotifications_DeterministicAndNoDedup(t *testing.T) {
	in := []map[string]interface{}{
		{
			"Z-CONTACT": map[string]interface{}{"delay": float64(5), "schedule": "Days"},
			"A-CONTACT": map[string]interface{}{"delay": "10", "schedule": ""},
		},
		{"Z-CONTACT": map[string]interface{}{"delay": float64(5), "schedule": "Days"}},
		{"BROKEN": "not-a-map"},
	}

	for i := 0; i < 10; i++ {
		got := mapNotifications(in)
		if len(got) != 3 {
			t.Fatalf("length = %d, want 3 (duplicates preserved, malformed skipped)", len(got))
		}
		if got[0].ContactID.ValueString() != "A-CONTACT" || got[1].ContactID.ValueString() != "Z-CONTACT" {
			t.Fatalf("map iteration is not deterministic: %#v", got)
		}
		if got[0].Delay.ValueInt64() != 10 {
			t.Fatalf("string delay not decoded: %v", got[0].Delay)
		}
		if got[0].Schedule.ValueString() != "All" {
			t.Fatalf("empty schedule should default to All, got %v", got[0].Schedule)
		}
	}
	if mapNotifications(nil) != nil {
		t.Error("nil notifications should map to nil")
	}
	if mapNotifications([]map[string]interface{}{{"X": 1}}) != nil {
		t.Error("only-malformed notifications should map to nil")
	}
}

func TestMergeTags(t *testing.T) {
	got := mergeTags([]string{"managed", "team"}, []string{"team", "prod", ""})
	want := []string{"managed", "team", "prod"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(mergeTags(nil, nil)) != 0 {
		t.Error("empty merge should be empty")
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func TestCreate_PreservesPasswordAndNormalisedValues(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	// The server normalises: it drops the password, echoes runlocations as
	// false and appends a trailing slash to the target.
	api.transform = func(body map[string]interface{}, stored map[string]interface{}) {
		stored["runlocations"] = false
		params := stored["parameters"].(map[string]interface{})
		params["target"] = "https://example.com/"
		delete(params, "password")
		params["method"] = "post"
	}
	r := api.resource()

	plan := baseModel()
	plan.Password = types.StringValue("s3cret")
	plan.Username = types.StringValue("user")
	plan.RunLocations = mustList(t, ctx, "nam")
	plan.Method = types.StringValue("POST")

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}

	got := stateModel(t, ctx, resp.State)
	if got.Password.ValueString() != "s3cret" {
		t.Errorf("password = %v, want s3cret (regression: password was nulled after apply)", got.Password)
	}
	if len(got.RunLocations.Elements()) != 1 {
		t.Errorf("runlocations = %v, want the planned value", got.RunLocations)
	}
	if got.Target.ValueString() != "https://example.com" {
		t.Errorf("target = %v, want the planned value", got.Target)
	}
	if got.Method.ValueString() != "POST" {
		t.Errorf("method = %v, want the planned value", got.Method)
	}
	if got.ID.ValueString() == "" || got.ID.IsUnknown() {
		t.Errorf("id not set: %v", got.ID)
	}
	if got.Created.IsUnknown() || got.Modified.IsUnknown() || got.State.IsUnknown() {
		t.Error("computed attributes left unknown after create")
	}
	// The request must not carry unknown values.
	if _, ok := api.lastBody["regex"]; ok {
		t.Error("unknown attributes must not be sent to the API")
	}
	if api.lastBody["password"] != "s3cret" {
		t.Errorf("password was not sent: %#v", api.lastBody["password"])
	}
}

func TestCreate_RecordsTheCheckIDWhenTheResponseCannotBeMapped(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	// The POST succeeded, so the check exists. Only the mapping of the response
	// into state fails, on a threshold NodePing reported as a non-numeric
	// string. Returning that error without writing state would leave the check
	// running in NodePing with nothing in the state file pointing at it.
	api.transform = func(body map[string]interface{}, stored map[string]interface{}) {
		stored["parameters"].(map[string]interface{})["threshold"] = "not-a-number"
	}
	r := api.resource()

	resp := &resource.CreateResponse{State: emptyState(ctx), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, newCreateRequest(t, ctx, baseModel()), resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the unmappable response to be reported")
	}

	got := stateModel(t, ctx, resp.State)
	if got.ID.ValueString() != "MOCK-CHECK-1" {
		t.Fatalf("state must carry the check ID so the next plan can reconcile it, got %q", got.ID.ValueString())
	}
	// customer_id was never configured, so it stays null and Read falls back to
	// the provider-level client -- the same scope the create used.
	if !got.CustomerID.IsNull() {
		t.Errorf("customer_id must stay null when it was not configured, got %v", got.CustomerID)
	}
	if !resp.Identity.Raw.IsFullyNull() {
		t.Errorf("identity must stay null on a failed create, got %v", resp.Identity.Raw)
	}
}

func TestCreate_RecordsTheConfiguredCustomerIDAlongsideTheCheckID(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	api.transform = func(body map[string]interface{}, stored map[string]interface{}) {
		stored["parameters"].(map[string]interface{})["threshold"] = "not-a-number"
	}
	r := api.resource()

	plan := baseModel()
	plan.CustomerID = types.StringValue("SUBACC1")

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the unmappable response to be reported")
	}

	// Read scopes its lookup with customer_id, so the partial state has to
	// carry the subaccount the check was created in.
	got := stateModel(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUBACC1" {
		t.Fatalf("state must carry the configured customer_id, got %q", got.CustomerID.ValueString())
	}
}

func TestCreate_WritesNoStateWhenTheAPICallFails(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	api.notFound = true
	r := api.resource()

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, baseModel()), resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the failed create to be reported")
	}
	// No ID came back, so there is nothing to record. State must stay empty
	// rather than gain an object OpenTofu would then try to manage.
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state must stay empty when the create itself failed, got %v", resp.State.Raw)
	}
}

func TestCreate_ContentStringPreservedSymmetrically(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	api.transform = func(body map[string]interface{}, stored map[string]interface{}) {
		stored["parameters"].(map[string]interface{})["contentstring"] = ""
	}
	r := api.resource()

	plan := baseModel()
	plan.Type = types.StringValue("HTTPCONTENT")
	plan.ContentString = types.StringValue("hello")

	createResp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", createResp.Diagnostics)
	}
	if got := stateModel(t, ctx, createResp.State); got.ContentString.ValueString() != "hello" {
		t.Errorf("create: contentstring = %v, want hello", got.ContentString)
	}

	// ... and the same must hold for Update.
	state := stateModel(t, ctx, createResp.State)
	updatePlan := state
	updatePlan.Label = types.StringValue("changed")
	updatePlan.Modified = types.Int64Unknown()

	updateResp := &resource.UpdateResponse{State: emptyState(ctx)}
	r.Update(ctx, newUpdateRequest(t, ctx, updatePlan, state), updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update failed: %v", updateResp.Diagnostics)
	}
	got := stateModel(t, ctx, updateResp.State)
	if got.ContentString.ValueString() != "hello" {
		t.Errorf("update: contentstring = %v, want hello", got.ContentString)
	}
	if got.Modified.IsUnknown() {
		t.Error("update: modified left unknown")
	}
	if got.Modified.ValueInt64() == state.Modified.ValueInt64() {
		t.Error("update: modified should have been refreshed from the API")
	}
}

func TestCreate_NotificationsKeepPlanOrderAndDuplicates(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	// Return the notifications in the opposite order, as a map-backed API can.
	api.transform = func(body map[string]interface{}, stored map[string]interface{}) {
		stored["notifications"] = []interface{}{
			map[string]interface{}{"B": map[string]interface{}{"delay": float64(5), "schedule": "Days"}},
			map[string]interface{}{"A": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
			map[string]interface{}{"A": map[string]interface{}{"delay": float64(0), "schedule": "All"}},
		}
	}
	r := api.resource()

	plan := baseModel()
	plan.Notifications = []NotificationModel{
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(0), Schedule: types.StringUnknown()},
		{ContactID: types.StringValue("B"), Delay: types.Int64Value(5), Schedule: types.StringValue("Days")},
		{ContactID: types.StringValue("A"), Delay: types.Int64Value(0), Schedule: types.StringValue("All")},
	}

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}
	got := stateModel(t, ctx, resp.State)
	if len(got.Notifications) != 3 {
		t.Fatalf("notification count = %d, want 3", len(got.Notifications))
	}
	for i, want := range []string{"A", "B", "A"} {
		if got.Notifications[i].ContactID.ValueString() != want {
			t.Errorf("notification %d = %v, want %v", i, got.Notifications[i].ContactID, want)
		}
	}
	if got.Notifications[0].Schedule.ValueString() != "All" {
		t.Errorf("unknown schedule not filled: %v", got.Notifications[0].Schedule)
	}
	if len(api.lastBody["notifications"].([]interface{})) != 3 {
		t.Error("all notification blocks must be sent")
	}
}

func TestRead_RefreshesDriftAndKeepsPassword(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	api.stored["CHK-1"] = map[string]interface{}{
		"_id":         "CHK-1",
		"customer_id": "ACCT-1",
		"type":        "HTTP",
		"label":       "renamed out of band",
		"enable":      "active",
		"interval":    float64(15),
		"parameters": map[string]interface{}{
			"target":   "https://example.com",
			"postdata": "x=1",
			"secure":   "ssl",
		},
	}

	prior := baseModel()
	prior.ID = types.StringValue("CHK-1")
	prior.CustomerID = types.StringValue("ACCT-1")
	prior.Label = types.StringValue("old")
	prior.Password = types.StringValue("s3cret")
	prior.PostData = types.StringNull()
	prior.Secure = types.StringNull()
	prior.State = types.Int64Value(0)
	prior.Created = types.Int64Value(1)
	prior.Modified = types.Int64Value(1)
	prior.Dep = types.StringNull()
	prior.Description = types.StringNull()
	prior.HomeLoc = types.StringNull()
	prior.RunLocations = types.ListNull(types.StringType)
	prior.Tags = types.SetNull(types.StringType)
	prior = knownify(prior)

	resp := &resource.ReadResponse{State: emptyState(ctx)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	got := stateModel(t, ctx, resp.State)

	if got.Label.ValueString() != "renamed out of band" {
		t.Errorf("label = %v, want the API value (drift must be visible)", got.Label)
	}
	if got.PostData.ValueString() != "x=1" {
		t.Errorf("postdata = %v, want x=1 (config attributes must be refreshed)", got.PostData)
	}
	if got.Secure.ValueString() != "ssl" {
		t.Errorf("secure = %v, want ssl", got.Secure)
	}
	if got.Password.ValueString() != "s3cret" {
		t.Errorf("password = %v, want the prior state value", got.Password)
	}
}

// knownify replaces every unknown value in a model with a null one, which is
// what state (as opposed to a plan) can hold.
func knownify(m CheckResourceModel) CheckResourceModel {
	if m.Label.IsUnknown() {
		m.Label = types.StringNull()
	}
	if m.ID.IsUnknown() {
		m.ID = types.StringNull()
	}
	if m.CustomerID.IsUnknown() {
		m.CustomerID = types.StringNull()
	}
	if m.State.IsUnknown() {
		m.State = types.Int64Null()
	}
	if m.Created.IsUnknown() {
		m.Created = types.Int64Null()
	}
	if m.Modified.IsUnknown() {
		m.Modified = types.Int64Null()
	}
	fillNulls(&m)
	return m
}

func TestRead_RemovesResourceWhenMissing(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	prior := knownify(baseModel())
	prior.ID = types.StringValue("GONE")

	resp := &resource.ReadResponse{State: newState(t, ctx, prior)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state should have been removed for a deleted check")
	}
}

func TestDelete(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()
	api.stored["CHK-1"] = map[string]interface{}{"_id": "CHK-1", "type": "HTTP"}

	state := knownify(baseModel())
	state.ID = types.StringValue("CHK-1")

	resp := &resource.DeleteResponse{State: newState(t, ctx, state)}
	r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete failed: %v", resp.Diagnostics)
	}
	if _, ok := api.stored["CHK-1"]; ok {
		t.Error("check was not deleted")
	}
}

func TestImportState_RoundTripsEveryAttribute(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	api.stored["CHK-1"] = map[string]interface{}{
		"_id":          "CHK-1",
		"customer_id":  "SUB-1",
		"type":         "MYSQL",
		"label":        "db",
		"enable":       "active",
		"interval":     float64(5),
		"runlocations": []interface{}{"nam"},
		"homeloc":      "nam",
		"tags":         []interface{}{"prod"},
		"parameters": map[string]interface{}{
			"target":     "db.example.com",
			"threshold":  "10",
			"sens":       "3",
			"port":       "3306",
			"database":   "app",
			"query":      "select 1",
			"username":   "user",
			"secure":     "ssl",
			"transport":  "tcp",
			"postdata":   "x=1",
			"namespace":  "ns",
			"email":      "a@example.com",
			"dnssection": "answer",
			"snmpv":      "2c",
			"snmpcom":    "public",
		},
	}

	resp := &resource.ImportStateResponse{State: emptyState(ctx)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "SUB-1:CHK-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import failed: %v", resp.Diagnostics)
	}
	got := stateModel(t, ctx, resp.State)

	if got.CustomerID.ValueString() != "SUB-1" {
		t.Errorf("customer_id = %v, want SUB-1", got.CustomerID)
	}
	if got.Threshold.ValueInt64() != 10 || got.Sens.ValueInt64() != 3 {
		t.Errorf("string-typed threshold/sens not imported: %v %v", got.Threshold, got.Sens)
	}
	for name, value := range map[string]string{
		"database":   got.Database.ValueString(),
		"query":      got.Query.ValueString(),
		"secure":     got.Secure.ValueString(),
		"transport":  got.Transport.ValueString(),
		"postdata":   got.PostData.ValueString(),
		"namespace":  got.Namespace.ValueString(),
		"email":      got.Email.ValueString(),
		"dnssection": got.DNSSection.ValueString(),
		"snmpv":      got.SNMPv.ValueString(),
		"snmpcom":    got.SNMPCom.ValueString(),
		"homeloc":    got.HomeLoc.ValueString(),
	} {
		if value == "" {
			t.Errorf("%s was not imported", name)
		}
	}
	if got.Port.ValueInt64() != 3306 {
		t.Errorf("port = %v", got.Port)
	}
	if len(got.Tags.Elements()) != 1 || len(got.RunLocations.Elements()) != 1 {
		t.Errorf("tags/runlocations not imported: %v %v", got.Tags, got.RunLocations)
	}

	// A subsequent refresh must not change anything.
	readResp := &resource.ReadResponse{State: emptyState(ctx)}
	r.Read(ctx, resource.ReadRequest{State: resp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read after import failed: %v", readResp.Diagnostics)
	}
	after := stateModel(t, ctx, readResp.State)
	if !after.Threshold.Equal(got.Threshold) || !after.Port.Equal(got.Port) ||
		!after.Secure.Equal(got.Secure) || !after.CustomerID.Equal(got.CustomerID) {
		t.Errorf("import/read round trip is not stable:\n import %#v\n read   %#v", got, after)
	}
	if api.lastQuery == "" {
		t.Error("expected the customer id to be sent as a query parameter")
	}
}

func TestImportState_InvalidID(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	for _, id := range []string{"a:b:c", ":"} {
		resp := &resource.ImportStateResponse{State: emptyState(ctx)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("expected an error for import ID %q", id)
		}
	}
}

func TestImportState_APIError(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	resp := &resource.ImportStateResponse{State: emptyState(ctx)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "MISSING"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error importing a missing check")
	}
}

func TestSubaccountRequestsAreScoped(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.CustomerID = types.StringValue("SUB-9")

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}
	if api.lastQuery != "customerid=SUB-9" {
		t.Errorf("query = %q, want customerid=SUB-9", api.lastQuery)
	}
	got := stateModel(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUB-9" {
		t.Errorf("customer_id = %v, want the configured SUB-9", got.CustomerID)
	}
}

func TestClientForFallsBackToProviderClient(t *testing.T) {
	r := &CheckResource{}
	if r.clientFor(types.StringValue("X")) != nil {
		t.Error("unconfigured resource should return a nil client")
	}
	api := newMockAPI(t)
	r = api.resource()
	if r.clientFor(types.StringNull()) != r.client {
		t.Error("null customer id should reuse the provider client")
	}
	if r.clientFor(types.StringUnknown()) != r.client {
		t.Error("unknown customer id should reuse the provider client")
	}
	if r.clientFor(types.StringValue("SUB")) == r.client {
		t.Error("a customer id should produce a scoped client")
	}
}

func TestConfigure(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}

	resp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || r.client != nil {
		t.Error("nil provider data should be a no-op")
	}

	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for the wrong provider data type")
	}

	resp = &resource.ConfigureResponse{}
	c := client.NewClient(client.ClientConfig{APIToken: "x"})
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || r.client != c {
		t.Error("client was not stored")
	}
}

func TestMetadataAndSchema(t *testing.T) {
	ctx := testContext()
	r := NewCheckResource()

	mResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "nodeping"}, mResp)
	if mResp.TypeName != "nodeping_check" {
		t.Errorf("type name = %q", mResp.TypeName)
	}

	sResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, sResp)
	if len(sResp.Schema.Attributes) == 0 {
		t.Error("schema has no attributes")
	}
}

// ---------------------------------------------------------------------------
// default_tags
// ---------------------------------------------------------------------------

func tagStrings(t *testing.T, s types.Set) []string {
	t.Helper()
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	if diags := s.ElementsAs(testContext(), &out, false); diags.HasError() {
		t.Fatalf("bad tag set: %v", diags)
	}
	return out
}

// The merge must land in tags_all and never in tags. Writing it into tags was
// rejected by Terraform as an invalid plan ("planned value [...] does not match
// config value [...]") the moment a resource declared any tags of its own,
// because the planned value of an Optional attribute has to equal a known
// configuration value.
func TestModifyPlan_DefaultTags(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)

	// planTags returns the planned tags and tags_all together, because the
	// relationship between them is the thing under test.
	planTags := func(t *testing.T, r *CheckResource, config CheckResourceModel) (tags, all types.Set) {
		t.Helper()
		plan := config
		plan.TagsAll = types.SetUnknown(types.StringType)
		resp := &resource.ModifyPlanResponse{Plan: newPlan(t, ctx, plan)}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{
			Config: newConfig(t, ctx, config),
			Plan:   newPlan(t, ctx, plan),
			State:  emptyState(ctx),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		var out CheckResourceModel
		resp.Plan.Get(ctx, &out)
		return out.Tags, out.TagsAll
	}

	t.Run("configured tags are never rewritten", func(t *testing.T) {
		r := api.resource("managed")
		config := knownify(baseModel())
		config.Tags = mustSet(t, ctx, "prod")

		tags, all := planTags(t, r, config)
		if got := tagStrings(t, tags); len(got) != 1 || got[0] != "prod" {
			t.Errorf("tags = %v, want exactly the configured [prod]", got)
		}
		if got := tagStrings(t, all); len(got) != 2 {
			t.Errorf("tags_all = %v, want managed+prod", got)
		}
	})

	t.Run("defaults alone fill tags_all when no tags are configured", func(t *testing.T) {
		r := api.resource("managed")
		config := knownify(baseModel())
		config.Tags = types.SetNull(types.StringType)

		tags, all := planTags(t, r, config)
		if !tags.IsNull() {
			t.Errorf("tags = %v, want null so it still matches the configuration", tags)
		}
		if got := tagStrings(t, all); len(got) != 1 || got[0] != "managed" {
			t.Errorf("tags_all = %v, want managed", got)
		}
	})

	t.Run("a configured tag that is also a default appears once", func(t *testing.T) {
		r := api.resource("managed")
		config := knownify(baseModel())
		config.Tags = mustSet(t, ctx, "managed", "prod")

		tags, all := planTags(t, r, config)
		if got := tagStrings(t, tags); len(got) != 2 {
			t.Errorf("tags = %v, want both configured tags", got)
		}
		if got := tagStrings(t, all); len(got) != 2 {
			t.Errorf("tags_all = %v, want the duplicate collapsed", got)
		}
	})

	t.Run("removing every tag plans an empty tags_all", func(t *testing.T) {
		r := api.resource()
		config := knownify(baseModel())
		config.Tags = emptyTagSet()

		tags, all := planTags(t, r, config)
		if got := tagStrings(t, tags); len(got) != 0 {
			t.Errorf("tags = %v, want empty", got)
		}
		if all.IsNull() || all.IsUnknown() {
			t.Fatalf("tags_all = %v, want a known empty set so the removal is sent", all)
		}
		if got := tagStrings(t, all); len(got) != 0 {
			t.Errorf("tags_all = %v, want empty", got)
		}
	})

	t.Run("dropping default_tags converges without resurrecting them", func(t *testing.T) {
		r := api.resource() // default_tags removed from the provider
		config := knownify(baseModel())
		config.Tags = mustSet(t, ctx, "prod")

		_, all := planTags(t, r, config)
		if got := tagStrings(t, all); len(got) != 1 || got[0] != "prod" {
			t.Errorf("tags_all = %v, want only prod", got)
		}
	})

	t.Run("unknown config tags leave tags_all unknown for the apply path", func(t *testing.T) {
		r := api.resource("managed")
		config := knownify(baseModel())
		config.Tags = types.SetUnknown(types.StringType)

		_, all := planTags(t, r, config)
		if !all.IsUnknown() {
			t.Errorf("tags_all = %v, want unknown", all)
		}
	})

	t.Run("destroy and unconfigured client are no-ops", func(t *testing.T) {
		r := &CheckResource{}
		resp := &resource.ModifyPlanResponse{}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: tfsdk.Plan{Schema: CheckSchema(), Raw: nullRaw(ctx)}}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})
}

func TestCreate_SendsMergedTagsSorted(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.Tags = mustSet(t, ctx, "b", "a")
	plan.TagsAll = mustSet(t, ctx, "b", "a")

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}
	sent, _ := api.lastBody["tags"].([]interface{})
	if len(sent) != 2 || sent[0] != "a" {
		t.Errorf("tags sent = %#v, want a sorted [a b]", sent)
	}
	if len(tagStrings(t, stateModel(t, ctx, resp.State).TagsAll)) != 2 {
		t.Error("tags_all missing from state")
	}
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

func validateConfig(t *testing.T, m CheckResourceModel) diag.Diagnostics {
	t.Helper()
	ctx := testContext()
	r := &CheckResource{}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, m)}, resp)
	return resp.Diagnostics
}

func configModel() CheckResourceModel {
	m := CheckResourceModel{Type: types.StringValue("HTTP"), Target: types.StringValue("https://example.com")}
	fillNulls(&m)
	return m
}

func TestValidateConfig(t *testing.T) {
	t.Run("target required", func(t *testing.T) {
		m := configModel()
		m.Target = types.StringNull()
		if !validateConfig(t, m).HasError() {
			t.Error("expected an error when target is missing for HTTP")
		}
	})

	t.Run("target optional for AGENT and PUSH", func(t *testing.T) {
		for _, checkType := range []string{"AGENT", "PUSH", "DNS", "SPEC10DNS", "SPEC10RDDS"} {
			m := configModel()
			m.Type = types.StringValue(checkType)
			m.Target = types.StringNull()
			if diags := validateConfig(t, m); diags.HasError() {
				t.Errorf("%s: unexpected error: %v", checkType, diags)
			}
		}
	})

	t.Run("contentstring rejected for the wrong type", func(t *testing.T) {
		m := configModel()
		m.ContentString = types.StringValue("x")
		if !validateConfig(t, m).HasError() {
			t.Error("expected an error for contentstring on an HTTP check")
		}
		m.Type = types.StringValue("HTTPCONTENT")
		if diags := validateConfig(t, m); diags.HasError() {
			t.Errorf("unexpected error: %v", diags)
		}
	})

	t.Run("dns and warningdays attributes", func(t *testing.T) {
		m := configModel()
		m.DNSType = types.StringValue("A")
		if !validateConfig(t, m).HasError() {
			t.Error("expected an error for dnstype on an HTTP check")
		}

		m = configModel()
		m.WarningDays = types.Int64Value(30)
		if !validateConfig(t, m).HasError() {
			t.Error("expected an error for warningdays on an HTTP check")
		}
		m.Type = types.StringValue("SSL")
		m.Target = types.StringValue("example.com")
		if diags := validateConfig(t, m); diags.HasError() {
			t.Errorf("unexpected error: %v", diags)
		}
	})

	t.Run("false content flags are accepted for any check type", func(t *testing.T) {
		// NodePing stores regex/invert/follow/ipv6 as false on PUSH checks and
		// returns them on every read, so the provider's own refresh output has
		// to survive its own validator. See A-017.
		m := configModel()
		m.Type = types.StringValue("PUSH")
		m.Target = types.StringNull()
		m.Regex = types.BoolValue(false)
		m.Invert = types.BoolValue(false)
		m.DNSRD = types.BoolValue(false)
		if diags := validateConfig(t, m); diags.HasError() {
			t.Errorf("unexpected error: %v", diags)
		}
	})

	t.Run("true content flags are still rejected for the wrong type", func(t *testing.T) {
		for _, set := range []func(*CheckResourceModel){
			func(m *CheckResourceModel) { m.Regex = types.BoolValue(true) },
			func(m *CheckResourceModel) { m.Invert = types.BoolValue(true) },
			func(m *CheckResourceModel) { m.DNSRD = types.BoolValue(true) },
		} {
			m := configModel()
			m.Type = types.StringValue("PUSH")
			m.Target = types.StringNull()
			set(&m)
			if !validateConfig(t, m).HasError() {
				t.Error("expected an error for a flag enabled on a PUSH check")
			}
		}
	})

	t.Run("unknown type is not validated", func(t *testing.T) {
		m := configModel()
		m.Type = types.StringUnknown()
		if diags := validateConfig(t, m); diags.HasError() {
			t.Errorf("unexpected error: %v", diags)
		}
	})
}

// fillNulls leaves every attr.Value zero value as it is; it exists to document
// that the zero value of the framework types is null, which is what an absent
// configuration attribute looks like.
func fillNulls(m *CheckResourceModel) {
	if m.Tags.ElementType(testContext()) == nil {
		m.Tags = types.SetNull(types.StringType)
	}
	if m.TagsAll.ElementType(testContext()) == nil {
		m.TagsAll = types.SetNull(types.StringType)
	}
	if m.RunLocations.ElementType(testContext()) == nil {
		m.RunLocations = types.ListNull(types.StringType)
	}
	if m.SendHeaders.ElementType(testContext()) == nil {
		m.SendHeaders = types.MapNull(types.StringType)
	}
	if m.ReceiveHeaders.ElementType(testContext()) == nil {
		m.ReceiveHeaders = types.MapNull(types.StringType)
	}
}

// ---------------------------------------------------------------------------
// request building
// ---------------------------------------------------------------------------

func TestBuildCreateRequest(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}
	var diags diag.Diagnostics

	plan := baseModel()
	plan.Enabled = types.BoolValue(false)
	plan.Tags = mustSet(t, ctx, "a")
	plan.TagsAll = mustSet(t, ctx, "a")
	plan.RunLocations = mustList(t, ctx, "nam")
	plan.SendHeaders, _ = types.MapValueFrom(ctx, types.StringType, map[string]string{"X": "1"})
	plan.ReceiveHeaders, _ = types.MapValueFrom(ctx, types.StringType, map[string]string{"Y": "2"})
	plan.Dep = types.StringValue("CHK-DEP")
	plan.SSHKey = types.StringValue("KEY")
	plan.ClientCert = types.StringValue("CERT")
	plan.SNMPv = types.StringValue("2c")
	plan.SNMPCom = types.StringValue("public")
	plan.Verify = types.BoolValue(true)
	plan.IPv6 = types.BoolValue(true)
	plan.DNSRD = types.BoolValue(true)
	plan.Regex = types.BoolValue(true)
	plan.Invert = types.BoolValue(true)
	plan.StatusCode = types.Int64Value(200)
	plan.HomeLoc = types.StringValue("nam")
	plan.Email = types.StringValue("a@example.com")
	plan.Database = types.StringValue("db")
	plan.Query = types.StringValue("q")
	plan.Namespace = types.StringValue("ns")
	plan.Transport = types.StringValue("tcp")
	plan.ServerName = types.StringValue("example.com")
	plan.DNSSection = types.StringValue("answer")
	plan.DNSToResolve = types.StringValue("example.com")
	plan.DNSType = types.StringValue("A")
	plan.Secure = types.StringValue("ssl")
	plan.PostData = types.StringValue("a=b")

	req := r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Enabled != "false" {
		t.Errorf("enabled = %q", req.Enabled)
	}
	if req.Type != "HTTP" || req.Target != "https://example.com" {
		t.Errorf("type/target = %q %q", req.Type, req.Target)
	}
	if req.Dep != "CHK-DEP" || req.SNMPCom != "public" || req.DNSSection != "answer" {
		t.Error("optional attributes were not forwarded")
	}
	if req.Tags == nil || len(*req.Tags) != 1 || len(req.RunLocations.([]string)) != 1 {
		t.Error("collections were not forwarded")
	}
	// Unknown attributes must be omitted entirely.
	if req.Method != "" || req.Password != "" || req.Port != nil || req.Follow != nil {
		t.Errorf("unknown attributes leaked into the request: %#v", req)
	}

	plan.Enabled = types.BoolValue(true)
	if req := r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags); req.Enabled != "active" {
		t.Errorf("enabled = %q, want active", req.Enabled)
	}
}

func tfsdkPlanNull(ctx context.Context) tfsdk.Plan {
	return tfsdk.Plan{Schema: CheckSchema(), Raw: nullRaw(ctx)}
}

func TestAPIErrorsSurfaceAsDiagnostics(t *testing.T) {
	ctx := testContext()

	t.Run("create", func(t *testing.T) {
		api := newMockAPI(t)
		api.notFound = true
		r := api.resource()
		resp := &resource.CreateResponse{State: emptyState(ctx)}
		r.Create(ctx, newCreateRequest(t, ctx, baseModel()), resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected a create error")
		}
	})

	state := knownify(baseModel())
	state.ID = types.StringValue("CHK-1")

	t.Run("update", func(t *testing.T) {
		api := newMockAPI(t)
		api.notFound = true
		r := api.resource()
		resp := &resource.UpdateResponse{State: emptyState(ctx)}
		r.Update(ctx, newUpdateRequest(t, ctx, baseModel(), state), resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected an update error")
		}
	})

	t.Run("read", func(t *testing.T) {
		api := newMockAPI(t)
		r := api.resource()
		api.server.Close()
		resp := &resource.ReadResponse{State: newState(t, ctx, state)}
		r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, state)}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected a read error")
		}
	})

	t.Run("delete of a missing check is not an error", func(t *testing.T) {
		api := newMockAPI(t)
		r := api.resource()
		resp := &resource.DeleteResponse{State: newState(t, ctx, state)}
		r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("deleting an absent check should succeed: %v", resp.Diagnostics)
		}
	})

	t.Run("delete failure", func(t *testing.T) {
		api := newMockAPI(t)
		r := api.resource()
		api.server.Close()
		resp := &resource.DeleteResponse{State: newState(t, ctx, state)}
		r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, state)}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected a delete error")
		}
	})
}

func TestToInt64UnsupportedTypes(t *testing.T) {
	for _, v := range []interface{}{struct{}{}, json.Number("x"), json.Number("1.5"), int64(7), float32(8)} {
		got, ok := toInt64(v)
		switch v.(type) {
		case struct{}:
			if ok {
				t.Errorf("struct should not decode")
			}
		case json.Number:
			if json.Number("1.5") == v && (!ok || got != 1) {
				t.Errorf("1.5 should decode to 1, got %v %v", got, ok)
			}
		default:
			if !ok {
				t.Errorf("%#v should decode", v)
			}
		}
	}
	if v := interfaceStringOrNull(true); v.ValueString() != "true" {
		t.Errorf("true should render as a string, got %v", v)
	}
	if v := interfaceStringOrNull(42); !v.IsNull() {
		t.Errorf("unsupported types should be null, got %v", v)
	}
}
