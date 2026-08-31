package check

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// request building
// ---------------------------------------------------------------------------

func TestBuildCreateRequest_typeSpecificAttributes(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}
	var diags diag.Diagnostics

	plan := baseModel()
	plan.Type = types.StringValue("REDIS")
	plan.Data = mustStringMap(t, ctx, map[string]string{"CHECK-A": "1", "CHECK-B": "0"})
	plan.EDNS = mustStringMap(t, ctx, map[string]string{"8": "0x0008"})
	plan.Fields = mustFieldsMap(t, ctx, map[string]CheckFieldModel{
		"1": {
			Name:  types.StringValue("load"),
			Min:   types.Int64Value(0),
			Max:   types.Int64Value(90),
			Match: types.StringValue("ok"),
		},
		"2": {Name: types.StringValue("free"), Min: types.Int64Null(), Max: types.Int64Null(), Match: types.StringNull()},
	})
	plan.Hosts = mustHostsMap(t, ctx, map[string]RedisHostModel{
		"primary": {Host: types.StringValue("redis1"), Port: types.Int64Value(6379), Password: types.StringValue("shhh")},
	})
	plan.RedisType = types.StringValue("sentinel")
	plan.SentinelName = types.StringValue("mymaster")
	plan.OldResultFail = types.BoolValue(true)
	plan.DoHDoT = types.StringValue("dot")
	plan.WhoisServer = types.StringValue("whois.verisign-grs.com")
	plan.RDAPUrl = types.StringValue("https://rdap.example.com/domain/")
	plan.Ignore = types.StringValue("spamhaus,sorbs")
	plan.VerifyVolume = types.BoolValue(true)
	plan.VolumeMin = types.Int64Value(-45)

	req := r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	data, ok := req.Data.(map[string]string)
	if !ok || data["CHECK-A"] != "1" || data["CHECK-B"] != "0" {
		t.Errorf("data was not forwarded: %#v", req.Data)
	}
	if req.EDNS["8"] != "0x0008" {
		t.Errorf("edns was not forwarded: %#v", req.EDNS)
	}
	if got := req.Fields["1"]; got.Name != "load" || got.Min != int64(0) || got.Max != int64(90) || got.Match != "ok" {
		t.Errorf("fields[1] was not forwarded: %#v", got)
	}
	if got := req.Fields["2"]; got.Name != "free" || got.Min != nil || got.Max != nil || got.Match != "" {
		t.Errorf("unset field bounds must stay absent: %#v", got)
	}
	if got := req.Hosts["primary"]; got.Host != "redis1" || got.Port != 6379 || got.Password != "shhh" {
		t.Errorf("hosts were not forwarded: %#v", got)
	}
	if req.RedisType != "sentinel" || req.SentinelName != "mymaster" {
		t.Errorf("redis settings were not forwarded: %q / %q", req.RedisType, req.SentinelName)
	}
	if req.OldResultFail != true {
		t.Errorf("oldresultfail was not forwarded: %#v", req.OldResultFail)
	}
	if req.WhoisServer == "" || req.RDAPUrl == "" || req.Ignore == "" {
		t.Errorf("whoisserver/rdapurl/ignore were not forwarded: %q %q %q", req.WhoisServer, req.RDAPUrl, req.Ignore)
	}
	if req.VerifyVolume != true || req.VolumeMin != int64(-45) {
		t.Errorf("audio settings were not forwarded: %#v / %#v", req.VerifyVolume, req.VolumeMin)
	}

	// redistype and dohdot carry schema defaults, so they must only be sent to
	// the check types that understand them.
	if req.DoHDoT != "" {
		t.Errorf("dohdot must not be sent for a REDIS check, got %q", req.DoHDoT)
	}
	plan.Type = types.StringValue("DOHDOT")
	req = r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if req.DoHDoT != "dot" {
		t.Errorf("dohdot must be sent for a DOHDOT check, got %q", req.DoHDoT)
	}
	if req.RedisType != "" {
		t.Errorf("redistype must not be sent for a DOHDOT check, got %q", req.RedisType)
	}
}

func TestBuildCreateRequest_volumeMinZeroIsTransmitted(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}
	var diags diag.Diagnostics

	plan := baseModel()
	plan.Type = types.StringValue("AUDIO")
	plan.VolumeMin = types.Int64Value(0)
	plan.VerifyVolume = types.BoolValue(false)

	req := r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("could not marshal the request: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("could not decode the request: %v", err)
	}
	if _, ok := decoded["volumemin"]; !ok {
		t.Error("volumemin 0 is a meaningful value and must not be dropped by omitempty")
	}
	if _, ok := decoded["verifyvolume"]; !ok {
		t.Error("verifyvolume false must not be dropped by omitempty")
	}
}

// ---------------------------------------------------------------------------
// read mapping
// ---------------------------------------------------------------------------

func TestMapCheckToModel_typeSpecificAttributes(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	check := &client.Check{
		ID:   "CHECK-1",
		Type: "SPEC10DNS",
		Parameters: client.CheckParameters{
			Data: map[string]interface{}{
				"a": "text",
				"b": float64(1),
				"c": true,
				"d": nil,
			},
			Fields: map[string]client.CheckField{
				"1": {Name: "load", Min: float64(0), Max: "90", Match: "ok"},
			},
			EDNS:          map[string]string{"8": "0x0008"},
			CheckToken:    "TOKEN-1",
			SentinelName:  "mymaster",
			WhoisServer:   "whois.example.com",
			RDAPUrl:       "https://rdap.example.com",
			Ignore:        "spamhaus",
			OldResultFail: true,
			VerifyVolume:  true,
			VolumeMin:     float64(-45),
		},
	}

	model := &CheckResourceModel{}
	mapCheckToModel(ctx, check, model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	data := make(map[string]string)
	if d := model.Data.ElementsAs(ctx, &data, false); d.HasError() {
		t.Fatalf("could not decode data: %v", d)
	}
	want := map[string]string{"a": "text", "b": "1", "c": "true", "d": ""}
	for key, value := range want {
		if data[key] != value {
			t.Errorf("data[%q] = %q, want %q", key, data[key], value)
		}
	}

	fields := make(map[string]CheckFieldModel)
	if d := model.Fields.ElementsAs(ctx, &fields, false); d.HasError() {
		t.Fatalf("could not decode fields: %v", d)
	}
	field := fields["1"]
	if field.Name.ValueString() != "load" || field.Min.ValueInt64() != 0 || field.Max.ValueInt64() != 90 {
		t.Errorf("fields were not mapped: %+v", field)
	}
	if field.Match.ValueString() != "ok" {
		t.Errorf("field match was not mapped: %v", field.Match)
	}

	if model.CheckToken.ValueString() != "TOKEN-1" {
		t.Errorf("checktoken was not mapped: %v", model.CheckToken)
	}
	if model.SentinelName.ValueString() != "mymaster" {
		t.Errorf("sentinelname was not mapped: %v", model.SentinelName)
	}
	if model.WhoisServer.ValueString() != "whois.example.com" || model.RDAPUrl.ValueString() != "https://rdap.example.com" {
		t.Errorf("whoisserver/rdapurl were not mapped: %v / %v", model.WhoisServer, model.RDAPUrl)
	}
	if model.Ignore.ValueString() != "spamhaus" {
		t.Errorf("ignore was not mapped: %v", model.Ignore)
	}
	if !model.OldResultFail.ValueBool() || !model.VerifyVolume.ValueBool() {
		t.Errorf("oldresultfail/verifyvolume were not mapped: %v / %v", model.OldResultFail, model.VerifyVolume)
	}
	if model.VolumeMin.ValueInt64() != -45 {
		t.Errorf("volumemin was not mapped: %v", model.VolumeMin)
	}
	if model.EDNS.IsNull() {
		t.Error("edns was not mapped")
	}
}

func TestMapCheckToModel_defaultsForOmittedEnums(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	// An HTTP check has no redistype or dohdot in the response. Both attributes
	// carry a schema default, so mapping them to null would produce a diff on
	// every plan.
	model := &CheckResourceModel{}
	mapCheckToModel(ctx, &client.Check{ID: "CHECK-1", Type: "HTTP"}, model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if model.RedisType.ValueString() != "standalone" {
		t.Errorf("redistype should fall back to the schema default, got %v", model.RedisType)
	}
	if model.DoHDoT.ValueString() != "doh" {
		t.Errorf("dohdot should fall back to the schema default, got %v", model.DoHDoT)
	}
	if !model.Data.IsNull() || !model.Fields.IsNull() || !model.Hosts.IsNull() || !model.EDNS.IsNull() {
		t.Error("absent collections must map to typed nulls")
	}
}

// ---------------------------------------------------------------------------
// validators
// ---------------------------------------------------------------------------

func TestEDNSKeysValidator(t *testing.T) {
	ctx := testContext()
	v := ednsKeysValidator{}

	if v.Description(ctx) == "" || v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Error("the validator must describe itself")
	}

	cases := []struct {
		name    string
		value   types.Map
		wantErr bool
	}{
		{"null is skipped", types.MapNull(types.StringType), false},
		{"unknown is skipped", types.MapUnknown(types.StringType), false},
		{"positive codes are accepted", mustStringMap(t, ctx, map[string]string{"8": "x", "12": "y"}), false},
		{"zero is rejected", mustStringMap(t, ctx, map[string]string{"0": "x"}), true},
		{"negative is rejected", mustStringMap(t, ctx, map[string]string{"-1": "x"}), true},
		{"non numeric is rejected", mustStringMap(t, ctx, map[string]string{"edns": "x"}), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.MapResponse{}
			v.ValidateMap(ctx, validator.MapRequest{
				Path:        path.Root("edns"),
				ConfigValue: tc.value,
			}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError = %v, want %v (%v)", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestURLValidator(t *testing.T) {
	ctx := testContext()
	v := urlValidator{}

	if v.Description(ctx) == "" || v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Error("the validator must describe itself")
	}

	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null is skipped", types.StringNull(), false},
		{"unknown is skipped", types.StringUnknown(), false},
		{"https is accepted", types.StringValue("https://rdap.example.com/domain"), false},
		{"http is accepted", types.StringValue("http://rdap.example.com"), false},
		{"a bare host is rejected", types.StringValue("rdap.example.com"), true},
		{"a non http scheme is rejected", types.StringValue("ftp://rdap.example.com"), true},
		{"garbage is rejected", types.StringValue("://"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("rdapurl"),
				ConfigValue: tc.value,
			}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError = %v, want %v (%v)", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestSchemaValidators_enumsAndRanges(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}

	cases := []struct {
		name    string
		mutate  func(*CheckResourceModel)
		wantErr bool
	}{
		{"a valid redis check", func(m *CheckResourceModel) {
			m.Type = types.StringValue("REDIS")
			m.RedisType = types.StringValue("cluster")
		}, false},
		{"sentinel without a master name", func(m *CheckResourceModel) {
			m.Type = types.StringValue("REDIS")
			m.RedisType = types.StringValue("sentinel")
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := baseModel()
			model.SentinelName = types.StringNull()
			tc.mutate(&model)

			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, model)}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError = %v, want %v (%v)", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// round trip
// ---------------------------------------------------------------------------

func TestCreate_roundTripsTypeSpecificAttributes(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.Type = types.StringValue("CLUSTER")
	plan.Target = types.StringNull()
	plan.Data = mustStringMap(t, ctx, map[string]string{"CHECK-A": "1"})
	plan.Ignore = types.StringNull()

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}

	got := stateModel(t, ctx, resp.State)
	data := make(map[string]string)
	if d := got.Data.ElementsAs(ctx, &data, false); d.HasError() {
		t.Fatalf("could not decode data: %v", d)
	}
	if data["CHECK-A"] != "1" {
		t.Errorf("the configured data must survive the apply, got %#v", data)
	}
	if got.CheckToken.IsUnknown() {
		t.Error("checktoken must be resolved by the apply")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustStringMap(t *testing.T, ctx context.Context, elements map[string]string) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(ctx, types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("could not build a string map: %v", diags)
	}
	return value
}

func mustFieldsMap(t *testing.T, ctx context.Context, elements map[string]CheckFieldModel) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(ctx, CheckFieldObjectType(), elements)
	if diags.HasError() {
		t.Fatalf("could not build a fields map: %v", diags)
	}
	return value
}

func mustHostsMap(t *testing.T, ctx context.Context, elements map[string]RedisHostModel) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(ctx, RedisHostObjectType(), elements)
	if diags.HasError() {
		t.Fatalf("could not build a hosts map: %v", diags)
	}
	return value
}
