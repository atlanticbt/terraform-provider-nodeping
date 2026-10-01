package contact

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression test for the defect the first live acceptance run of
// nodeping_contact found: NodePing discards suppressdiag on a contact address,
// so a configured true came back false and OpenTofu aborted the apply with
// "Provider produced inconsistent result after apply". The validator turns that
// into an actionable message at plan time.
func TestSuppressDiagValidator(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		value     types.Bool
		wantError bool
	}{
		{name: "true is rejected because the API discards it", value: types.BoolValue(true), wantError: true},
		{name: "false is accepted", value: types.BoolValue(false)},
		{name: "null is accepted", value: types.BoolNull()},
		{name: "unknown is accepted", value: types.BoolUnknown()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.BoolRequest{
				Path:        path.Root("address").AtListIndex(0).AtName("suppress_diag"),
				ConfigValue: tc.value,
			}
			resp := &validator.BoolResponse{}

			SuppressDiagValidator().ValidateBool(ctx, req, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("HasError() = %t, want %t (diagnostics: %v)", got, tc.wantError, resp.Diagnostics)
			}
			if tc.wantError && resp.Diagnostics[0].Summary() != "Unsupported Attribute Value" {
				t.Errorf("unexpected diagnostic summary %q", resp.Diagnostics[0].Summary())
			}
		})
	}
}

func TestSuppressDiagValidatorDescription(t *testing.T) {
	ctx := context.Background()
	v := SuppressDiagValidator()

	if v.Description(ctx) == "" {
		t.Error("Description() is empty")
	}
	if v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Error("MarkdownDescription() and Description() disagree")
	}
}
