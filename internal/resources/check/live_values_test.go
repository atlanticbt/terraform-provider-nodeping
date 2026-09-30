package check

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func int64AttrDiags(t *testing.T, name string, v int64) bool {
	t.Helper()
	attr := CheckSchema().Attributes[name].(schema.Int64Attribute)
	resp := &validator.Int64Response{}
	for _, val := range attr.Validators {
		val.ValidateInt64(testContext(), validator.Int64Request{
			Path:        path.Root(name),
			ConfigValue: types.Int64Value(v),
		}, resp)
	}
	return resp.Diagnostics.HasError()
}

// Values NodePing stores on live checks must be accepted, or the check cannot
// be imported: a live PUSH check has sens = 0, and two HTTPADV checks have
// statuscode = 0.
func TestSchema_AcceptsValuesNodePingStores(t *testing.T) {
	cases := []struct {
		name    string
		value   int64
		wantErr bool
	}{
		{"sens", 0, false},
		{"sens", -1, true},
		{"statuscode", 0, false},
		{"statuscode", 200, false},
		{"statuscode", 599, false},
		{"statuscode", 99, true},
		{"statuscode", 600, true},
		{"statuscode", -1, true},
	}
	for _, c := range cases {
		if got := int64AttrDiags(t, c.name, c.value); got != c.wantErr {
			t.Errorf("%s = %d: error = %v, want %v", c.name, c.value, got, c.wantErr)
		}
	}
}
