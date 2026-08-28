package maintenance

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateCronExpressionAcceptsWellFormedLines(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 3 * * 0",
		"*/15 * * * *",
		"0,30 8-17 * * 1-5",
		"0 0 1 1 *",
		"0 0 1 JAN SUN",
		"0 0 1 jan-mar mon,fri",
		"0 0 ? * 3",
		"0 0 1-15/2 * *",
		"  0   3   *   *   0  ",
		"0 0 * * 7",
	}

	for _, expr := range valid {
		if err := ValidateCronExpression(expr); err != nil {
			t.Errorf("%q should be valid: %v", expr, err)
		}
	}
}

func TestValidateCronExpressionRejectsMalformedLines(t *testing.T) {
	invalid := map[string]string{
		"too few fields":        "0 3 * *",
		"too many fields":       "0 3 * * 0 7",
		"empty":                 "",
		"minute out of range":   "60 3 * * 0",
		"hour out of range":     "0 24 * * 0",
		"dom out of range":      "0 0 32 * *",
		"dom zero":              "0 0 0 * *",
		"month out of range":    "0 0 1 13 *",
		"dow out of range":      "0 0 1 1 8",
		"unknown month name":    "0 0 1 XYZ *",
		"reversed range":        "0 0 1 1 5-2",
		"non numeric":           "abc 3 * * 0",
		"empty list entry":      "0,,30 3 * * 0",
		"step not a number":     "*/x * * * *",
		"step out of range":     "*/60 * * * *",
		"step zero":             "*/0 * * * *",
		"step without base":     "/5 * * * *",
		"question in minute":    "? 3 * * 0",
		"embedded newline":      "0 3 *\n* 0",
		"embedded tab":          "0\t3 * * 0",
		"range missing operand": "0 -5 * * 0",
	}

	for name, expr := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateCronExpression(expr); err == nil {
				t.Fatalf("%q should be rejected", expr)
			}
		})
	}
}

func TestCronValidatorSkipsNullAndUnknown(t *testing.T) {
	ctx := context.Background()
	v := CronValidator()

	for _, value := range []types.String{types.StringNull(), types.StringUnknown()} {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        path.Root("cron"),
			ConfigValue: value,
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics for %v: %v", value, resp.Diagnostics)
		}
	}
}

func TestCronValidatorReportsAttributeError(t *testing.T) {
	ctx := context.Background()
	v := CronValidator()

	resp := &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		Path:        path.Root("cron"),
		ConfigValue: types.StringValue("0 3 * *"),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a 4-field cron")
	}

	resp = &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		Path:        path.Root("cron"),
		ConfigValue: types.StringValue("0 3 * * 0"),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if v.Description(ctx) == "" || v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Fatal("validator descriptions are inconsistent")
	}
}
