package contact

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestJSONStringTypeBasics(t *testing.T) {
	ctx := context.Background()
	typ := JSONStringType{}

	if typ.String() != "contact.JSONStringType" {
		t.Fatalf("String() = %q", typ.String())
	}
	if !typ.Equal(JSONStringType{}) {
		t.Fatal("expected the type to equal itself")
	}
	if typ.Equal(basetypes.StringType{}) {
		t.Fatal("expected the type not to equal basetypes.StringType")
	}
	if _, ok := typ.ValueType(ctx).(JSONString); !ok {
		t.Fatal("ValueType must return a JSONString")
	}

	valuable, diags := typ.ValueFromString(ctx, basetypes.NewStringValue("x"))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if valuable.(JSONString).ValueString() != "x" {
		t.Fatal("unexpected value")
	}

	val, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(tftypes.String, "y"))
	if err != nil {
		t.Fatalf("ValueFromTerraform: %v", err)
	}
	js, ok := val.(JSONString)
	if !ok || js.ValueString() != "y" {
		t.Fatalf("ValueFromTerraform = %#v", val)
	}
	if !js.Type(ctx).Equal(JSONStringType{}) {
		t.Fatal("Type() mismatch")
	}

	if _, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(tftypes.Bool, true)); err == nil {
		t.Fatal("expected an error for a non-string value")
	}
}

func TestJSONStringEqual(t *testing.T) {
	a := NewJSONStringValue("x")
	if !a.Equal(NewJSONStringValue("x")) {
		t.Fatal("expected equality")
	}
	if a.Equal(NewJSONStringValue("y")) {
		t.Fatal("expected inequality")
	}
	if a.Equal(basetypes.NewStringValue("x")) {
		t.Fatal("expected inequality with a plain string value")
	}
	if !NewJSONStringNull().IsNull() || !NewJSONStringUnknown().IsUnknown() {
		t.Fatal("null/unknown constructors are broken")
	}
}

func TestJSONStringSemanticEquals(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		a, b  JSONString
		equal bool
	}{
		{
			name:  "formatting differences ignored",
			a:     NewJSONStringValue("{\n  \"a\": 1,\n  \"b\": [1, 2]\n}"),
			b:     NewJSONStringValue(`{"b":[1,2],"a":1}`),
			equal: true,
		},
		{
			name:  "different documents",
			a:     NewJSONStringValue(`{"a":1}`),
			b:     NewJSONStringValue(`{"a":2}`),
			equal: false,
		},
		{
			name:  "array order matters",
			a:     NewJSONStringValue(`[1,2]`),
			b:     NewJSONStringValue(`[2,1]`),
			equal: false,
		},
		{
			name:  "non json compared literally, equal",
			a:     NewJSONStringValue("a=1&b=2"),
			b:     NewJSONStringValue("a=1&b=2"),
			equal: true,
		},
		{
			name:  "non json compared literally, different",
			a:     NewJSONStringValue("a=1&b=2"),
			b:     NewJSONStringValue("a=1&b=3"),
			equal: false,
		},
		{
			name:  "one side not json",
			a:     NewJSONStringValue(`{"a":1}`),
			b:     NewJSONStringValue("a=1"),
			equal: false,
		},
		{
			name:  "both null",
			a:     NewJSONStringNull(),
			b:     NewJSONStringNull(),
			equal: true,
		},
		{
			name:  "null vs value",
			a:     NewJSONStringNull(),
			b:     NewJSONStringValue("x"),
			equal: false,
		},
		{
			name:  "both unknown",
			a:     NewJSONStringUnknown(),
			b:     NewJSONStringUnknown(),
			equal: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := tc.a.StringSemanticEquals(ctx, tc.b)
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			if got != tc.equal {
				t.Fatalf("StringSemanticEquals = %v, want %v", got, tc.equal)
			}
		})
	}

	if _, diags := NewJSONStringValue("x").StringSemanticEquals(ctx, basetypes.NewStringValue("x")); !diags.HasError() {
		t.Fatal("expected an error diagnostic for a mismatched value type")
	}
}
