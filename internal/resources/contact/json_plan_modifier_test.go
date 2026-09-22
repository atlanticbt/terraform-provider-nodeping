package contact

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// A first import is the case semantic equality cannot reach. The framework
// applies StringSemanticEquals in ReadResource, Create, Update and
// ReadDataSource, and in neither PlanResourceChange nor ImportResourceState, so
// on import there is no prior value in the configuration's encoding to keep and
// Terraform core diffs the configuration against state without consulting the
// provider. The plan modifier is what reaches that case.
func TestJSONSemanticPlanModifier(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name   string
		state  JSONString
		config JSONString
		want   JSONString
	}{
		{
			name:   "whitespace only difference keeps the state value",
			state:  NewJSONStringValue(`{"body": "hello","msgtype": "m.text"}`),
			config: NewJSONStringValue(`{"body":"hello","msgtype":"m.text"}`),
			want:   NewJSONStringValue(`{"body": "hello","msgtype": "m.text"}`),
		},
		{
			name:   "key order difference keeps the state value",
			state:  NewJSONStringValue(`{"msgtype":"m.text","body":"hello"}`),
			config: NewJSONStringValue(`{"body":"hello","msgtype":"m.text"}`),
			want:   NewJSONStringValue(`{"msgtype":"m.text","body":"hello"}`),
		},
		{
			name:   "a real change is planned",
			state:  NewJSONStringValue(`{"body":"hello"}`),
			config: NewJSONStringValue(`{"body":"goodbye"}`),
			want:   NewJSONStringValue(`{"body":"goodbye"}`),
		},
		{
			name:   "non-JSON bodies compare exactly and an equal one is kept",
			state:  NewJSONStringValue("a=1&b=2"),
			config: NewJSONStringValue("a=1&b=2"),
			want:   NewJSONStringValue("a=1&b=2"),
		},
		{
			name:   "a changed non-JSON body is planned",
			state:  NewJSONStringValue("a=1&b=2"),
			config: NewJSONStringValue("a=1&b=3"),
			want:   NewJSONStringValue("a=1&b=3"),
		},
		{
			name:   "JSON in state against a non-JSON configuration is planned",
			state:  NewJSONStringValue(`{"a":1}`),
			config: NewJSONStringValue("a=1"),
			want:   NewJSONStringValue("a=1"),
		},
		{
			name:   "removing the value is planned",
			state:  NewJSONStringValue(`{"a":1}`),
			config: NewJSONStringNull(),
			want:   NewJSONStringNull(),
		},
		{
			name:   "adding a value to an absent one is planned",
			state:  NewJSONStringNull(),
			config: NewJSONStringValue(`{"a":1}`),
			want:   NewJSONStringValue(`{"a":1}`),
		},
		{
			name:   "an unknown configuration value is left alone",
			state:  NewJSONStringValue(`{"a":1}`),
			config: NewJSONStringUnknown(),
			want:   NewJSONStringUnknown(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.StringRequest{
				StateValue:  tc.state.StringValue,
				ConfigValue: tc.config.StringValue,
				PlanValue:   tc.config.StringValue,
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.config.StringValue}

			JSONSemanticPlanModifier().PlanModifyString(ctx, req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(tc.want.StringValue) {
				t.Fatalf("PlanValue = %s, want %s", resp.PlanValue, tc.want.StringValue)
			}
		})
	}
}

func TestJSONSemanticPlanModifierDescriptions(t *testing.T) {
	ctx := context.Background()
	m := JSONSemanticPlanModifier()

	if m.Description(ctx) == "" {
		t.Fatal("Description must not be empty")
	}
	if m.MarkdownDescription(ctx) == "" {
		t.Fatal("MarkdownDescription must not be empty")
	}
}
