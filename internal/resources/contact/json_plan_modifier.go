package contact

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// jsonSemanticPlanModifier keeps the state value for the webhook `data`
// attribute when the configured value is the same JSON document written
// differently.
//
// This exists because semantic equality does not reach the import case.
// terraform-plugin-framework applies StringSemanticEquals from ReadResource,
// Create, Update and ReadDataSource, and from neither PlanResourceChange nor
// ImportResourceState. In steady state that is enough: prior state holds the
// value in the configuration's encoding, so a read that returns a different
// encoding is collapsed back onto it. A first import has no such prior value —
// state receives whatever the API sent — and Terraform core then diffs the
// configuration against state without consulting the provider, which produced a
// permanent "whitespace changes" diff on every imported webhook contact.
//
// A plan modifier does run during PlanResourceChange, so it is the hook that
// reaches this. The two mechanisms are complementary rather than alternatives:
// semantic equality still guards refresh and apply consistency.
type jsonSemanticPlanModifier struct{}

// JSONSemanticPlanModifier returns the plan modifier for the webhook `data`
// attribute.
func JSONSemanticPlanModifier() planmodifier.String {
	return jsonSemanticPlanModifier{}
}

func (m jsonSemanticPlanModifier) Description(_ context.Context) string {
	return "Keeps the prior value when the configured value is an equivalent JSON document."
}

func (m jsonSemanticPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m jsonSemanticPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// An unknown configuration value resolves during apply, and a null on
	// either side is a genuine addition or removal rather than a difference in
	// encoding. Neither is ours to collapse.
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// jsonSemanticEqual falls back to exact comparison when either side is not
	// valid JSON, so a form-encoded or plain-text body is unaffected. It is
	// shared with StringSemanticEquals so the two paths cannot disagree about
	// what "equal" means.
	if jsonSemanticEqual(req.StateValue.ValueString(), req.ConfigValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}
