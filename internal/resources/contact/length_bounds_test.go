package contact

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func validatorsFor(name string) []validator.String {
	if name == "name" {
		return ContactSchema().Attributes["name"].(schema.StringAttribute).Validators
	}
	block := ContactSchema().Blocks["address"].(schema.ListNestedBlock)
	return block.NestedObject.Attributes[name].(schema.StringAttribute).Validators
}

func rejects(t *testing.T, name, value string) bool {
	t.Helper()
	resp := &validator.StringResponse{}
	for _, v := range validatorsFor(name) {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root(name),
			ConfigValue: types.StringValue(value),
		}, resp)
	}
	return resp.Diagnostics.HasError()
}

// NodePing stores contact names, addresses and webhook bodies verbatim at
// 100,000 characters, so these bounds are the provider's own.
func TestSchema_ShouldBoundContactFreeText(t *testing.T) {
	cases := []struct {
		name  string
		limit int
	}{
		{"name", maxContactNameLength},
		{"address", maxAddressLength},
		{"data", maxWebhookDataLength},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rejects(t, c.name, strings.Repeat("a", c.limit)) {
				t.Errorf("%s of %d characters rejected, want accepted", c.name, c.limit)
			}
			if !rejects(t, c.name, strings.Repeat("a", c.limit+1)) {
				t.Errorf("%s of %d characters accepted, want rejected", c.name, c.limit+1)
			}
		})
	}
}

func TestSchema_ShouldRejectEmptyAddress(t *testing.T) {
	if !rejects(t, "address", "") {
		t.Error("empty address accepted, want rejected")
	}
}
