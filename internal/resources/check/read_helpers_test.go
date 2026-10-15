package check

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func TestStringMapOrNull(t *testing.T) {
	var diags diag.Diagnostics
	ctx := context.Background()

	for name, in := range map[string]map[string]string{"nil": nil, "empty": {}} {
		if got := stringMapOrNull(ctx, in, &diags); !got.IsNull() {
			t.Errorf("%s map should render null, got %v", name, got)
		}
	}

	got := stringMapOrNull(ctx, map[string]string{"User-Agent": "monitor"}, &diags)
	if got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf("a populated map should render its elements, got %v", got)
	}
	if diags.HasError() {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}
