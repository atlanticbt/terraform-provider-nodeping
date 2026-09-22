package contact

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// suppressDiagValidator rejects `suppress_diag = true`.
//
// NodePing does not store suppressdiag on a contact address. The field is
// accepted by POST /contacts and PUT /contacts/:id, discarded, and never echoed
// back for any address type — verified live against email, sms and webhook
// addresses, where every sibling suppress* flag round trips. Without this the
// value planned as true and came back false, and OpenTofu aborted the apply
// with "Provider produced inconsistent result after apply ... This is a bug in
// the provider", which sends the practitioner looking in the wrong place for a
// limitation of the upstream API.
//
// The attribute itself stays, so configurations and state that set it to false
// keep working, and the check can be dropped unchanged if NodePing ever starts
// honouring the field.
type suppressDiagValidator struct{}

// SuppressDiagValidator returns the validator for the `suppress_diag` address
// attribute.
func SuppressDiagValidator() validator.Bool {
	return suppressDiagValidator{}
}

func (v suppressDiagValidator) Description(_ context.Context) string {
	return "Rejects true, which the NodePing API discards"
}

func (v suppressDiagValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v suppressDiagValidator) ValidateBool(_ context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Unsupported Attribute Value",
		"NodePing does not store suppress_diag on a contact address: the value is accepted by the "+
			"API, discarded, and never returned. Setting it to true would leave state claiming a "+
			"suppression that is not in effect. Remove the attribute or set it to false, and use "+
			"suppress_all to stop notifications to this address.",
	)
}
