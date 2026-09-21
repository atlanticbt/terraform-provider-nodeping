package check

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchemaCoversEveryCheckType(t *testing.T) {
	want := []string{
		"AGENT", "AUDIO", "CLUSTER", "DOHDOT", "DNS", "FTP", "HTTP", "HTTPCONTENT",
		"HTTPPARSE", "HTTPADV", "IMAP4", "MONGODB", "MTR", "MYSQL", "NTP", "PGSQL",
		"PING", "POP3", "PORT", "PUSH", "RBL", "RDAP", "RDP", "REDIS", "SIP", "SMTP",
		"SNMP", "SPEC10DNS", "SPEC10RDDS", "SSH", "SSL", "WEBSOCKET", "WHOIS",
	}
	if len(ValidCheckTypes) != len(want) {
		t.Fatalf("ValidCheckTypes has %d entries, want %d", len(ValidCheckTypes), len(want))
	}
	have := map[string]bool{}
	for _, tp := range ValidCheckTypes {
		have[tp] = true
	}
	for _, tp := range want {
		if !have[tp] {
			t.Errorf("check type %s is missing from ValidCheckTypes", tp)
		}
	}
}

func TestSchemaAttributeFlags(t *testing.T) {
	s := CheckSchema()

	sensitive := []string{"password", "password_wo", "snmpcom", "checktoken"}
	for _, name := range sensitive {
		if !s.Attributes[name].IsSensitive() {
			t.Errorf("%s must be marked sensitive", name)
		}
	}

	// tofu plan -generate-config-out writes `null # sensitive` for every
	// sensitive attribute, so marking sendheaders sensitive made an import of a
	// check that sends headers propose deleting them. See A-015.
	if s.Attributes["sendheaders"].IsSensitive() {
		t.Error("sendheaders must not be sensitive: config generation would emit null and propose deleting live headers")
	}

	// An attribute is Optional + Computed only when NodePing supplies a value
	// the practitioner never set; otherwise the framework substitutes prior
	// state when the attribute is deleted from configuration and the removal
	// never produces a diff. Everything listed here is pure user supplied
	// content that the API only ever echoes back, so it stays Optional-only.
	for name := range optionalOnlyAttributes {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Errorf("%s is no longer a schema attribute", name)
			continue
		}
		if !attribute.IsOptional() {
			t.Errorf("%s must stay Optional", name)
		}
		if attribute.IsComputed() {
			t.Errorf("%s must not be Computed: removing it from configuration would produce no diff", name)
		}
	}

	// Everything else that is configurable must be Computed, so that a value
	// the API supplies on its own does not read back as a diff. Write-only
	// attributes are the exception: the framework forbids Computed on them, and
	// password_wo_version is a pure client side trigger the API has no opinion
	// about.
	writeOnlyPair := map[string]bool{"password_wo": true, "password_wo_version": true}
	for name, attribute := range s.Attributes {
		if !attribute.IsOptional() || optionalOnlyAttributes[name] || writeOnlyPair[name] {
			continue
		}
		if !attribute.IsComputed() {
			t.Errorf("%s is Optional but not Computed", name)
		}
	}

	if !s.Attributes["customer_id"].IsOptional() || !s.Attributes["customer_id"].IsComputed() {
		t.Error("customer_id must be Optional+Computed to support subaccounts")
	}

	if _, ok := s.Attributes["tags"].(schema.SetAttribute); !ok {
		t.Errorf("tags should be a set, got %T", s.Attributes["tags"])
	}
}

func TestWarningDaysAcceptsTheValuesTheAPIReturns(t *testing.T) {
	ctx := testContext()
	attribute, ok := CheckSchema().Attributes["warningdays"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("warningdays is not an int64 attribute")
	}

	// A live SSL check stores warningdays = 0. A floor the API does not enforce
	// makes that check impossible to import. See A-016.
	for _, days := range []int64{0, 1, 14, 365} {
		resp := &validator.Int64Response{}
		for _, v := range attribute.Validators {
			v.ValidateInt64(ctx, validator.Int64Request{
				Path:        path.Root("warningdays"),
				ConfigValue: types.Int64Value(days),
			}, resp)
		}
		if resp.Diagnostics.HasError() {
			t.Errorf("warningdays %d should be accepted: %v", days, resp.Diagnostics)
		}
	}
}

func TestSchemaPlanModifiers(t *testing.T) {
	s := CheckSchema()

	created, ok := s.Attributes["created"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("created is not an int64 attribute")
	}
	if len(created.PlanModifiers) != 1 {
		t.Fatalf("created should keep UseStateForUnknown")
	}

	for _, name := range []string{"modified", "state"} {
		attribute := s.Attributes[name].(schema.Int64Attribute)
		if len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s should have exactly one plan modifier", name)
		}
		if _, ok := attribute.PlanModifiers[0].(unknownOnChange); !ok {
			t.Errorf("%s should use the unknownOnChange modifier, got %T", name, attribute.PlanModifiers[0])
		}
	}
}

func TestUnknownOnChange(t *testing.T) {
	ctx := testContext()
	m := unknownOnChange{}
	if m.Description(ctx) == "" || m.MarkdownDescription(ctx) == "" {
		t.Error("plan modifier descriptions must not be empty")
	}

	state := knownify(baseModel())
	state.ID = types.StringValue("CHK-1")
	state.Modified = types.Int64Value(100)

	t.Run("no change keeps the state value", func(t *testing.T) {
		resp := &planmodifier.Int64Response{PlanValue: types.Int64Value(100)}
		m.PlanModifyInt64(ctx, planmodifier.Int64Request{
			State: newState(t, ctx, state),
			Plan:  newPlan(t, ctx, state),
		}, resp)
		if resp.PlanValue.IsUnknown() {
			t.Error("value should stay known when nothing changed")
		}
	})

	t.Run("change marks the value unknown", func(t *testing.T) {
		changed := state
		changed.Label = types.StringValue("new label")
		resp := &planmodifier.Int64Response{PlanValue: types.Int64Value(100)}
		m.PlanModifyInt64(ctx, planmodifier.Int64Request{
			State: newState(t, ctx, state),
			Plan:  newPlan(t, ctx, changed),
		}, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Error("value should be unknown when the resource changes")
		}
	})

	t.Run("create and destroy are no-ops", func(t *testing.T) {
		resp := &planmodifier.Int64Response{PlanValue: types.Int64Unknown()}
		m.PlanModifyInt64(ctx, planmodifier.Int64Request{
			State: emptyState(ctx),
			Plan:  newPlan(t, ctx, state),
		}, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Error("create should be left alone")
		}

		resp = &planmodifier.Int64Response{PlanValue: types.Int64Value(1)}
		m.PlanModifyInt64(ctx, planmodifier.Int64Request{
			State: newState(t, ctx, state),
			Plan:  tfsdkPlanNull(ctx),
		}, resp)
		if resp.PlanValue.IsUnknown() {
			t.Error("destroy should be left alone")
		}
	})
}

func TestIntervalValidator(t *testing.T) {
	ctx := testContext()
	v := intervalValidator{}
	if v.Description(ctx) == "" || v.MarkdownDescription(ctx) == "" {
		t.Error("validator descriptions must not be empty")
	}

	valid := []float64{0.25, 0.5, 1, 5, 1440}
	for _, f := range valid {
		resp := &validator.Float64Response{}
		v.ValidateFloat64(ctx, validator.Float64Request{
			Path:        path.Root("interval"),
			ConfigValue: types.Float64Value(f),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("interval %v should be valid: %v", f, resp.Diagnostics)
		}
	}

	invalid := []float64{0, 0.1, 0.75, 2.5, -1}
	for _, f := range invalid {
		resp := &validator.Float64Response{}
		v.ValidateFloat64(ctx, validator.Float64Request{
			Path:        path.Root("interval"),
			ConfigValue: types.Float64Value(f),
		}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("interval %v should be rejected", f)
		}
	}

	for _, value := range []types.Float64{types.Float64Null(), types.Float64Unknown()} {
		resp := &validator.Float64Response{}
		v.ValidateFloat64(ctx, validator.Float64Request{ConfigValue: value}, resp)
		if resp.Diagnostics.HasError() {
			t.Error("null and unknown intervals must be skipped")
		}
	}
}

func TestRunLocationValidator(t *testing.T) {
	ctx := testContext()
	v := runLocationValidator{}
	if v.Description(ctx) == "" || v.MarkdownDescription(ctx) == "" {
		t.Error("validator descriptions must not be empty")
	}

	valid := []string{"nam", "NAM", "lam", "eur", "eao", "wlw", "nj", "TX", "201205050153W2Q4C-0J2HSIRF"}
	for _, s := range valid {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        path.Root("runlocations"),
			ConfigValue: types.StringValue(s),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%q should be a valid run location: %v", s, resp.Diagnostics)
		}
	}

	invalid := []string{"", "usa", "north-am", "n"}
	for _, s := range invalid {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        path.Root("runlocations"),
			ConfigValue: types.StringValue(s),
		}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("%q should be rejected", s)
		}
	}

	resp := &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{ConfigValue: types.StringNull()}, resp)
	if resp.Diagnostics.HasError() {
		t.Error("null run locations must be skipped")
	}

	if runLocationsValidator() == nil {
		t.Error("runlocations list validator must be constructed")
	}
}
