package check

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NodePing treats a JSON number 0 as empty: sens 0 fails the create with
// "Unable to create check", and statuscode or warningdays 0 is stored as "".
// The string "0" is kept (probed live against the test account, 2026-09-30).
// Zeros are therefore sent as strings, and every other value as a number.
func TestBuildCreateRequest_ShouldSendZeroNumbersAsStrings(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics
	plan := baseModel()
	plan.Type = types.StringValue("HTTPADV")
	plan.Sens = types.Int64Value(0)
	plan.StatusCode = types.Int64Value(0)
	plan.WarningDays = types.Int64Value(0)
	plan.VolumeMin = types.Int64Value(0)
	plan.Port = types.Int64Value(0)

	req := (&CheckResource{}).buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	json.Unmarshal(body, &got)
	for _, k := range []string{"sens", "statuscode", "warningdays", "volumemin", "port"} {
		if got[k] != "0" {
			t.Errorf("%s = %#v, want the string \"0\"", k, got[k])
		}
	}
}

func TestBuildCreateRequest_ShouldSendNonZeroNumbersAsNumbers(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics
	plan := baseModel()
	plan.Type = types.StringValue("HTTPADV")
	plan.Sens = types.Int64Value(2)
	plan.StatusCode = types.Int64Value(403)
	plan.WarningDays = types.Int64Value(14)
	plan.VolumeMin = types.Int64Value(-45)
	plan.Port = types.Int64Value(443)

	req := (&CheckResource{}).buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	body, _ := json.Marshal(req)
	var got map[string]interface{}
	json.Unmarshal(body, &got)
	want := map[string]float64{"sens": 2, "statuscode": 403, "warningdays": 14, "volumemin": -45, "port": 443}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %v", k, got[k], v)
		}
	}
}
