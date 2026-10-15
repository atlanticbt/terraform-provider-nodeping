package check

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// isSet reports whether a configured value carries data, i.e. it is neither
// null nor unknown.
func isSet(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// interfaceStringOrNull renders a loosely typed API field as a string. NodePing
// returns "false" (the boolean) for a number of optional string fields when
// they are unset.
func interfaceStringOrNull(v interface{}) types.String {
	switch val := v.(type) {
	case string:
		return stringOrNull(val)
	case bool:
		if !val {
			return types.StringNull()
		}
		return types.StringValue(strconv.FormatBool(val))
	default:
		return types.StringNull()
	}
}

func boolOrNull(v interface{}) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(parseBoolInterface(v))
}

func int64OrNull(v interface{}, name, checkID string, diags *diag.Diagnostics) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	if i, ok := toInt64(v); ok {
		return types.Int64Value(i)
	}
	if s, isString := v.(string); isString && s == "" {
		return types.Int64Null()
	}
	if b, isBool := v.(bool); isBool && !b {
		// NodePing uses false to mean "unset" for several numeric fields.
		return types.Int64Null()
	}
	diags.AddError(
		"Unexpected API Response",
		fmt.Sprintf("Could not decode %s value %#v returned for check %s as a number.", name, v, checkID),
	)
	return types.Int64Null()
}

// toInt64 decodes the numeric representations NodePing uses interchangeably:
// JSON numbers, json.Number and decimal strings.
func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	case int:
		return int64(val), true
	case int64:
		return val, true
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i, true
		}
		if f, err := val.Float64(); err == nil {
			return int64(f), true
		}
		return 0, false
	case string:
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return int64(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

// parseBoolInterface converts various interface{} types to bool.
// NodePing API returns booleans as bool, string ("true"/"false"), or numbers (0/1).
func parseBoolInterface(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	case float64:
		return val != 0
	case int:
		return val != 0
	case json.Number:
		return val.String() != "0" && val.String() != ""
	default:
		return false
	}
}

// apiInt renders a number for a check request. NodePing treats a JSON number 0
// as empty: sens 0 fails the create, and statuscode or warningdays 0 is stored
// as "". The string "0" is kept, so zero is sent as a string.
func apiInt(v int64) interface{} {
	if v == 0 {
		return "0"
	}
	return v
}
