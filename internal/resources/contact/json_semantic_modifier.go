package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// JSONStringType is a custom string type for the webhook `data` attribute.
//
// It provides *semantic equality*: two values are considered equal when both
// are valid JSON documents that are structurally identical, regardless of
// whitespace, key ordering, or number formatting. Unlike a plan modifier,
// semantic equality is applied by the framework both when computing a plan and
// when checking apply consistency, so a value that the API returns in a
// different-but-equivalent encoding does not produce a "Provider produced
// inconsistent result after apply" error or a phantom diff.
//
// Note this deliberately does *not* use jsontypes.Normalized: the NodePing
// `data` field is an arbitrary webhook request body which is frequently not
// JSON at all (form encoded bodies, plain text templates). jsontypes.Normalized
// rejects non-JSON input at validation time; this type falls back to exact
// string comparison for non-JSON values instead.
type JSONStringType struct {
	basetypes.StringType
}

var (
	_ basetypes.StringTypable                    = JSONStringType{}
	_ basetypes.StringValuableWithSemanticEquals = JSONString{}
)

func (t JSONStringType) Equal(o attr.Type) bool {
	other, ok := o.(JSONStringType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t JSONStringType) String() string {
	return "contact.JSONStringType"
}

func (t JSONStringType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return JSONString{StringValue: in}, nil
}

func (t JSONStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", attrValue)
	}

	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}

	return stringValuable, nil
}

func (t JSONStringType) ValueType(_ context.Context) attr.Value {
	return JSONString{}
}

// JSONString is the value type for JSONStringType.
type JSONString struct {
	basetypes.StringValue
}

// NewJSONStringValue returns a known JSONString.
func NewJSONStringValue(v string) JSONString {
	return JSONString{StringValue: basetypes.NewStringValue(v)}
}

// NewJSONStringNull returns a null JSONString.
func NewJSONStringNull() JSONString {
	return JSONString{StringValue: basetypes.NewStringNull()}
}

// NewJSONStringUnknown returns an unknown JSONString.
func NewJSONStringUnknown() JSONString {
	return JSONString{StringValue: basetypes.NewStringUnknown()}
}

func (v JSONString) Type(_ context.Context) attr.Type {
	return JSONStringType{}
}

func (v JSONString) Equal(o attr.Value) bool {
	other, ok := o.(JSONString)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals implements semantic equality for JSON documents.
func (v JSONString) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(JSONString)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			fmt.Sprintf("expected value type contact.JSONString but got %T. Please report this to the provider developers.", newValuable),
		)
		return false, diags
	}

	if v.IsNull() != newValue.IsNull() || v.IsUnknown() != newValue.IsUnknown() {
		return false, diags
	}
	if v.IsNull() || v.IsUnknown() {
		return true, diags
	}

	return jsonSemanticEqual(v.ValueString(), newValue.ValueString()), diags
}

// jsonSemanticEqual compares two strings for JSON semantic equality.
// If either side is not valid JSON, it falls back to exact string comparison.
func jsonSemanticEqual(a, b string) bool {
	if a == b {
		return true
	}

	var aVal, bVal interface{}

	if err := json.Unmarshal([]byte(a), &aVal); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bVal); err != nil {
		return false
	}

	return reflect.DeepEqual(aVal, bVal)
}
