package check

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func stringAttrRejects(t *testing.T, name, value string) bool {
	t.Helper()
	attr := CheckSchema().Attributes[name].(schema.StringAttribute)
	resp := &validator.StringResponse{}
	for _, v := range attr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root(name),
			ConfigValue: types.StringValue(value),
		}, resp)
	}
	return resp.Diagnostics.HasError()
}

// NodePing stores these values verbatim at 100,000 characters and enforces no
// limit of its own, so the bounds are the provider's: far above any real value,
// low enough to stop a pathological one before it is sent.
func TestSchema_ShouldBoundFreeTextAttributes(t *testing.T) {
	cases := []struct {
		name  string
		limit int
	}{
		{"label", maxLabelLength},
		{"target", maxTargetLength},
		{"contentstring", maxContentStringLength},
		{"postdata", maxPostDataLength},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if stringAttrRejects(t, c.name, strings.Repeat("a", c.limit)) {
				t.Errorf("%s of %d characters rejected, want accepted", c.name, c.limit)
			}
			if !stringAttrRejects(t, c.name, strings.Repeat("a", c.limit+1)) {
				t.Errorf("%s of %d characters accepted, want rejected", c.name, c.limit+1)
			}
		})
	}
}

// A PUSH check has no target and NodePing returns it as "", so an empty target
// must stay valid.
func TestSchema_ShouldAcceptEmptyTarget(t *testing.T) {
	if stringAttrRejects(t, "target", "") {
		t.Error("empty target rejected, want accepted")
	}
}
