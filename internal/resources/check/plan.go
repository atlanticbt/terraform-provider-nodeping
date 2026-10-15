package check

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ModifyPlan computes tags_all, the union of the configured tags and the
// provider level default_tags, and it is tags_all that is sent to NodePing.
//
// The merge cannot be written into "tags" itself. Terraform requires the
// planned value of an Optional attribute to equal a known configuration value,
// so planning [team-devops, tf-acc] against a configured [team-devops] is
// rejected as an invalid plan; doing the same merge in Read instead only moves
// the same rule one step later, to the consistency check after apply. Splitting
// the attribute in two is what the AWS provider does for the same reason:
// "tags" holds what the practitioner wrote and always matches the
// configuration, and the computed "tags_all" holds what is actually applied.
//
// The configured tags are read from the config rather than the plan so that a
// removal is visible. That mattered more when "tags" was Optional+Computed and
// the framework copied prior state into the plan for an absent attribute; with
// "tags" now Optional only the plan already tracks config, and reading the
// config keeps the merge correct either way.
func (r *CheckResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip if destroying or client not configured.
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}

	planDefaultInterval(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	var configTags types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tags"), &configTags)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An unknown config value cannot be merged at plan time. Leaving tags_all
	// unknown lets the apply path fill it in from the API response.
	if configTags.IsUnknown() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), types.SetUnknown(types.StringType))...)
		return
	}

	var explicitTags []string
	if !configTags.IsNull() {
		resp.Diagnostics.Append(configTags.ElementsAs(ctx, &explicitTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	merged := mergeTags(r.client.GetDefaultTags(), explicitTags)

	// NodePing omits the tags key on a check that has never had any, which Read
	// stores as null. An empty set means "send [] to clear every tag", and there
	// is nothing to clear, so planning null keeps an untagged check from planning
	// an update on every run.
	if len(merged) == 0 && !req.State.Raw.IsNull() {
		var priorAll types.Set
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("tags_all"), &priorAll)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if priorAll.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), types.SetNull(types.StringType))...)
			if resp.Diagnostics.HasError() {
				return
			}
			collapseNoOpPlan(ctx, req, resp)
			return
		}
	}

	tagsAll, diags := types.SetValueFrom(ctx, types.StringType, merged)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), tagsAll)...)
	if resp.Diagnostics.HasError() {
		return
	}

	collapseNoOpPlan(ctx, req, resp)
}

// collapseNoOpPlan replaces the plan with prior state when nothing the provider
// can see has actually changed.
//
// TargetNormalizationPlanModifier keeps "target" itself out of the diff when
// the configuration spells it differently from the form NodePing stores, but by
// the time any plan modifier runs the damage is done: the framework has already
// compared the proposed plan with prior state and, finding them different,
// marked every null computed attribute unknown so the provider can fill it in.
// A plan carrying unknowns is not an empty plan. After an import, where state
// holds NodePing's canonical target and the configuration holds the
// practitioner's, that left an in-place update planned against a check with
// nothing wrong with it.
//
// Resource level ModifyPlan is the last hook in the sequence and the only one
// that can undo it. Filling the unknowns back from prior state is what decides
// the question: if every value the plan actually knows already matches state
// then the blanked attributes are the whole of the difference, and planning
// prior state verbatim is what Terraform asks a provider to do when it
// considers the configured and prior values equivalent. Any real change leaves
// a known value differing, and the plan is left alone.
func collapseNoOpPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || resp.Plan.Raw.IsNull() || resp.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	filled, err := fillUnknownsFromState(resp.Plan.Raw, req.State.Raw)
	if err != nil || !filled.Equal(req.State.Raw) {
		return
	}

	tflog.Debug(ctx, "Plan differs from state only in values the framework blanked; keeping prior state")

	resp.Plan.Raw = req.State.Raw
}

// fillUnknownsFromState returns plan with every unknown value replaced by the
// value at the same path in state. A path that state does not have is left
// unknown, which keeps the caller's comparison unequal rather than silently
// treating a missing value as a match.
func fillUnknownsFromState(plan, state tftypes.Value) (tftypes.Value, error) {
	return tftypes.Transform(plan, func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		if v.IsKnown() {
			return v, nil
		}
		found, remaining, err := tftypes.WalkAttributePath(state, p)
		if err != nil || len(remaining.Steps()) > 0 {
			return v, nil
		}
		stateValue, ok := found.(tftypes.Value)
		if !ok {
			return v, nil
		}
		return stateValue, nil
	})
}

// planDefaultInterval replaces the schema's default interval of 15 on check
// types whose minimum is higher. NodePing would store rewrittenInterval, so
// planning 15 would show a change on every run.
func planDefaultInterval(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var checkType types.String
	var configInterval types.Float64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &checkType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("interval"), &configInterval)...)
	if resp.Diagnostics.HasError() || !configInterval.IsNull() || !isSet(checkType) {
		return
	}
	if _, ok := minIntervalByType[checkType.ValueString()]; ok {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("interval"), types.Float64Value(rewrittenInterval))...)
	}
}
