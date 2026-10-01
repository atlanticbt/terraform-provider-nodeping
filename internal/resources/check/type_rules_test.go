package check

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NodePing replaces a WHOIS interval below 240 minutes, or an RDAP interval
// below 60, with 1440, on create and on update (probed live, 2026-09-30). A
// lower configured value would never settle, so it is refused.
func TestValidateConfig_ShouldRejectIntervalsNodePingRewrites(t *testing.T) {
	cases := []struct {
		checkType string
		interval  float64
		wantErr   bool
	}{
		{"WHOIS", 15, true},
		{"WHOIS", 239, true},
		{"WHOIS", 240, false},
		{"WHOIS", 1440, false},
		{"RDAP", 59, true},
		{"RDAP", 60, false},
		{"HTTP", 1, false},
	}
	for _, c := range cases {
		m := configModel()
		m.Type = types.StringValue(c.checkType)
		m.Target = types.StringValue("example.com")
		m.Interval = types.Float64Value(c.interval)
		if got := validateConfig(t, m).HasError(); got != c.wantErr {
			t.Errorf("%s interval %v: error = %v, want %v", c.checkType, c.interval, got, c.wantErr)
		}
	}
}

// With interval unset, the schema default of 15 is below both minimums, so
// NodePing would store 1440 and every plan would try to set 15 again. The plan
// takes 1440, which is what NodePing stores.
func TestModifyPlan_ShouldDefaultIntervalTo1440ForWhoisAndRdap(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	for checkType, want := range map[string]float64{"WHOIS": 1440, "RDAP": 1440, "HTTP": 15} {
		config := knownify(baseModel())
		config.Type = types.StringValue(checkType)
		config.Interval = types.Float64Null()
		plan := config
		plan.Interval = types.Float64Value(15)
		plan.TagsAll = types.SetUnknown(types.StringType)
		resp := &resource.ModifyPlanResponse{Plan: newPlan(t, ctx, plan)}
		api.resource().ModifyPlan(ctx, resource.ModifyPlanRequest{
			Config: newConfig(t, ctx, config),
			Plan:   newPlan(t, ctx, plan),
			State:  emptyState(ctx),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: unexpected diagnostics: %v", checkType, resp.Diagnostics)
		}
		var out CheckResourceModel
		resp.Plan.Get(ctx, &out)
		if out.Interval.ValueFloat64() != want {
			t.Errorf("%s: interval = %v, want %v", checkType, out.Interval, want)
		}
	}
}

// SPEC10DNS keeps none of dnstype, dnstoresolve, dnssection or dnsrd (probed
// live), so setting them is flagged rather than accepted and silently dropped.
func TestValidateConfig_ShouldFlagDNSAttributesOnSpec10DNS(t *testing.T) {
	m := configModel()
	m.Type = types.StringValue("SPEC10DNS")
	m.DNSToResolve = types.StringValue("example.com")
	if diags := validateConfig(t, m); diags.WarningsCount() == 0 {
		t.Error("expected a warning for dnstoresolve on SPEC10DNS")
	}
	m = configModel()
	m.Type = types.StringValue("SPEC10DNS")
	m.DNSRD = types.BoolValue(true)
	if !validateConfig(t, m).HasError() {
		t.Error("expected an error for dnsrd = true on SPEC10DNS")
	}
}
