package check

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// targetNormalizationPlanModifier keeps the state value for "target" when the
// configured value is the same target written in a form NodePing rewrites.
//
// Read already collapses the API's normalisation back onto the prior state
// value, which keeps a refresh stable, but that only works when there is a
// prior value in the configuration's spelling. Import has none: state receives
// whatever the API sent, and Terraform core then diffs the configuration
// against state without consulting the provider. A practitioner who wrote
// "https://example.com" and imported the check got an update planned against a
// check that was already correct.
//
// A plan modifier runs during PlanResourceChange, which is the hook that
// reaches this. Planning the prior value rather than the configured one is
// explicitly permitted: core accepts a planned value that equals a non-null
// prior when the configuration is also non-null, on the grounds that the
// provider is declaring the two functionally equivalent. Planning the
// *normalised* value instead would be rejected, because "target" is
// Optional+Computed and core requires the planned value of such an attribute to
// match a known configuration value.
type targetNormalizationPlanModifier struct{}

// TargetNormalizationPlanModifier returns the plan modifier for the "target"
// attribute.
func TargetNormalizationPlanModifier() planmodifier.String {
	return targetNormalizationPlanModifier{}
}

func (m targetNormalizationPlanModifier) Description(_ context.Context) string {
	return "Keeps the prior value when the configured target differs from it only by normalisation NodePing applies."
}

func (m targetNormalizationPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m targetNormalizationPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// An unknown configuration value resolves during apply, and a null on
	// either side is a genuine addition, removal, or a create with no prior
	// state. None of those is a difference in spelling.
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// Normalisation is per check type, so the type has to come out of the
	// configuration. A change of type is a change of target semantics, so the
	// configured type is the right one to ask about rather than the prior one.
	var checkType types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &checkType)...)
	if resp.Diagnostics.HasError() || checkType.IsNull() || checkType.IsUnknown() {
		return
	}

	state := normalizeTarget(checkType.ValueString(), req.StateValue.ValueString())
	config := normalizeTarget(checkType.ValueString(), req.ConfigValue.ValueString())
	if state == config {
		resp.PlanValue = req.StateValue
	}
}
