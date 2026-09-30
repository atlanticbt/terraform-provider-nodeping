package check

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Import is the case the Read side collapse cannot reach. It has no prior value
// in the configuration's spelling, so state receives the API's normalised
// target and core diffs the configuration against it without asking the
// provider. The plan modifier is what makes that plan empty.
func TestTargetNormalizationPlanModifier(t *testing.T) {
	ctx := testContext()

	cases := []struct {
		name      string
		checkType types.String
		state     types.String
		config    types.String
		want      types.String
	}{
		{
			name:      "a root path added by the API keeps the state value",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringValue("https://example.com"),
			want:      types.StringValue("https://example.com/"),
		},
		{
			name:      "host case folded by the API keeps the state value",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/x"),
			config:    types.StringValue("https://EXAMPLE.COM/x"),
			want:      types.StringValue("https://example.com/x"),
		},
		{
			name:      "surrounding whitespace keeps the state value",
			checkType: types.StringValue("PING"),
			state:     types.StringValue("example.com"),
			config:    types.StringValue("  example.com  "),
			want:      types.StringValue("example.com"),
		},
		{
			name:      "SSL is normalised like HTTP",
			checkType: types.StringValue("SSL"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringValue("https://example.com"),
			want:      types.StringValue("https://example.com/"),
		},
		{
			name:      "a real target change is planned",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringValue("https://example.org/"),
			want:      types.StringValue("https://example.org/"),
		},
		{
			name:      "a path change is planned",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/a"),
			config:    types.StringValue("https://example.com/b"),
			want:      types.StringValue("https://example.com/b"),
		},
		{
			name:      "a scheme change is planned",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringValue("http://example.com/"),
			want:      types.StringValue("http://example.com/"),
		},
		{
			name:      "host case is a real change for a type the API does not normalise",
			checkType: types.StringValue("WEBSOCKET"),
			state:     types.StringValue("wss://example.com"),
			config:    types.StringValue("wss://EXAMPLE.COM"),
			want:      types.StringValue("wss://EXAMPLE.COM"),
		},
		{
			name:      "host case is a real change for a hostname type",
			checkType: types.StringValue("PING"),
			state:     types.StringValue("example.com"),
			config:    types.StringValue("EXAMPLE.COM"),
			want:      types.StringValue("EXAMPLE.COM"),
		},
		{
			name:      "an unknown configuration value is left alone",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringUnknown(),
			want:      types.StringUnknown(),
		},
		{
			name:      "removing the target is planned",
			checkType: types.StringValue("HTTP"),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringNull(),
			want:      types.StringNull(),
		},
		{
			name:      "a create with no prior state is left alone",
			checkType: types.StringValue("HTTP"),
			state:     types.StringNull(),
			config:    types.StringValue("https://example.com"),
			want:      types.StringValue("https://example.com"),
		},
		{
			name:      "an unknown check type is left alone",
			checkType: types.StringUnknown(),
			state:     types.StringValue("https://example.com/"),
			config:    types.StringValue("https://example.com"),
			want:      types.StringValue("https://example.com"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := knownify(baseModel())
			config.Type = tc.checkType
			config.Target = tc.config

			req := planmodifier.StringRequest{
				Config:      newConfig(t, ctx, config),
				StateValue:  tc.state,
				ConfigValue: tc.config,
				PlanValue:   tc.config,
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.config}

			TargetNormalizationPlanModifier().PlanModifyString(ctx, req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(tc.want) {
				t.Fatalf("PlanValue = %s, want %s", resp.PlanValue, tc.want)
			}
		})
	}
}

func TestTargetNormalizationPlanModifierDescriptions(t *testing.T) {
	ctx := context.Background()
	m := TargetNormalizationPlanModifier()

	if m.Description(ctx) == "" {
		t.Fatal("Description must not be empty")
	}
	if m.MarkdownDescription(ctx) == "" {
		t.Fatal("MarkdownDescription must not be empty")
	}
}

// The framework marks every null computed attribute unknown as soon as the
// proposed plan differs from prior state, and it does so before any plan
// modifier runs. Collapsing the target is therefore not enough on its own: the
// blanked attributes keep the plan non-empty, which is what made an imported
// check plan an update it did not need.
func TestCollapseNoOpPlan(t *testing.T) {
	ctx := testContext()

	base := knownify(baseModel())
	base.Type = types.StringValue("HTTP")
	base.Target = types.StringValue("https://example.com/")
	base.Label = types.StringValue("original")

	stateOf := func(m CheckResourceModel) tfsdk.State {
		return tfsdk.State{Schema: CheckSchema(), Raw: newPlan(t, ctx, m).Raw}
	}
	collapse := func(state tfsdk.State, plan CheckResourceModel) tfsdk.Plan {
		resp := &resource.ModifyPlanResponse{Plan: newPlan(t, ctx, plan)}
		collapseNoOpPlan(ctx, resource.ModifyPlanRequest{State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
		return resp.Plan
	}

	t.Run("blanked computed attributes alone are collapsed onto prior state", func(t *testing.T) {
		state := stateOf(base)
		plan := base
		plan.Modified = types.Int64Unknown()
		plan.State = types.Int64Unknown()

		if got := collapse(state, plan); !got.Raw.Equal(state.Raw) {
			t.Error("a plan whose only difference is blanked attributes must become prior state")
		}
	})

	t.Run("a real change is left alone", func(t *testing.T) {
		state := stateOf(base)
		plan := base
		plan.Label = types.StringValue("renamed")
		plan.Modified = types.Int64Unknown()

		if got := collapse(state, plan); got.Raw.Equal(state.Raw) {
			t.Error("a label change must survive the collapse")
		}
	})

	t.Run("a removed attribute is left alone", func(t *testing.T) {
		withValue := base
		withValue.Description = types.StringValue("notes")
		state := stateOf(withValue)

		plan := withValue
		plan.Description = types.StringNull()
		plan.Modified = types.Int64Unknown()

		if got := collapse(state, plan); got.Raw.Equal(state.Raw) {
			t.Error("removing an attribute must survive the collapse")
		}
	})

	t.Run("an identical plan is untouched", func(t *testing.T) {
		state := stateOf(base)
		if got := collapse(state, base); !got.Raw.Equal(state.Raw) {
			t.Error("an identical plan must stay identical")
		}
	})

	t.Run("create has no prior state to collapse onto", func(t *testing.T) {
		plan := base
		plan.Modified = types.Int64Unknown()
		resp := &resource.ModifyPlanResponse{Plan: newPlan(t, ctx, plan)}
		collapseNoOpPlan(ctx, resource.ModifyPlanRequest{State: emptyState(ctx)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
		if !resp.Plan.Raw.Equal(newPlan(t, ctx, plan).Raw) {
			t.Error("a create plan must be left alone")
		}
	})
}
