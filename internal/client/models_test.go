package client

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Finding 10: suppress flags must be able to travel as false.
// ---------------------------------------------------------------------------

func TestContactAddressSuppressFlagsMarshalWhenFalse(t *testing.T) {
	addr := ContactAddress{Address: "a@example.com", Type: "email"}

	data, err := json.Marshal(addr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, field := range []string{"suppressup", "suppressdown", "suppressfirst", "suppressdiag", "suppressall"} {
		v, ok := decoded[field]
		if !ok {
			t.Errorf("%s missing from the payload: a false value would never clear the server-side flag", field)
			continue
		}
		if v != false {
			t.Errorf("%s = %v, want false", field, v)
		}
	}
}

func TestNewAddressSuppressFlagsMarshalWhenFalse(t *testing.T) {
	addr := NewAddress{Address: "a@example.com", Type: "email"}

	data, err := json.Marshal(addr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, field := range []string{"suppressup", "suppressdown", "suppressfirst", "suppressdiag", "suppressall"} {
		if v, ok := decoded[field]; !ok || v != false {
			t.Errorf("%s: got %v (present=%v), want false", field, v, ok)
		}
	}

	// Mute is interface{}: nil is omitted, but an explicit false is sent.
	if _, ok := decoded["mute"]; ok {
		t.Error("an unset mute should be omitted")
	}

	addr.Mute = false
	data, _ = json.Marshal(addr)
	if !strings.Contains(string(data), `"mute":false`) {
		t.Errorf("an explicit mute=false must be transmitted, got %s", data)
	}
}

func TestContactAddressMuteRoundTrip(t *testing.T) {
	// nil => omitted (means "leave unchanged")
	data, _ := json.Marshal(ContactAddress{Address: "a@b.c"})
	if strings.Contains(string(data), `"mute"`) {
		t.Errorf("nil mute should be omitted, got %s", data)
	}

	// explicit false => transmitted (this is how a caller clears a mute)
	data, _ = json.Marshal(ContactAddress{Address: "a@b.c", Mute: json.RawMessage("false")})
	if !strings.Contains(string(data), `"mute":false`) {
		t.Errorf("explicit mute=false must be transmitted, got %s", data)
	}

	// NodePing also returns a numeric mute-until timestamp.
	var addr ContactAddress
	if err := json.Unmarshal([]byte(`{"address":"a@b.c","mute":1735689600000}`), &addr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(addr.Mute) != "1735689600000" {
		t.Errorf("mute = %s, want the raw numeric value", addr.Mute)
	}
}

// ---------------------------------------------------------------------------
// Finding 11: type-tolerant decoding.
// ---------------------------------------------------------------------------

func TestCheckIntervalToleratesStringAndNumber(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"number", `{"_id":"x","interval":15}`, "15"},
		{"string", `{"_id":"x","interval":"15"}`, "15"},
		{"float string", `{"_id":"x","interval":"0.5"}`, "0.5"},
		{"absent", `{"_id":"x"}`, ""},
		{"null", `{"_id":"x","interval":null}`, ""},
		{"empty string", `{"_id":"x","interval":""}`, ""},
		{"non numeric string", `{"_id":"x","interval":"soon"}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var check Check
			if err := json.Unmarshal([]byte(tt.body), &check); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.body, err)
			}
			if string(check.Interval) != tt.want {
				t.Errorf("Interval = %q, want %q", check.Interval, tt.want)
			}
			if check.ID != "x" {
				t.Errorf("sibling fields must still decode: ID = %q", check.ID)
			}
		})
	}
}

func TestCheckParametersNumericFieldsNormalizedToFloat64(t *testing.T) {
	body := `{
		"_id": "x",
		"label": "l",
		"parameters": {
			"target": "https://example.com",
			"sens": "3",
			"threshold": "5",
			"port": "8443",
			"warningdays": "30",
			"statuscode": "200",
			"volumemin": "12"
		}
	}`

	var check Check
	if err := json.Unmarshal([]byte(body), &check); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	fields := map[string]struct {
		got  interface{}
		want float64
	}{
		"sens":        {check.Parameters.Sens, 3},
		"threshold":   {check.Parameters.Threshold, 5},
		"port":        {check.Parameters.Port, 8443},
		"warningdays": {check.Parameters.WarningDays, 30},
		"statuscode":  {check.Parameters.StatusCode, 200},
		"volumemin":   {check.Parameters.VolumeMin, 12},
	}

	for name, f := range fields {
		v, ok := f.got.(float64)
		if !ok {
			t.Errorf("%s: got %T (%v), want float64 — downstream type-asserts float64 and would map this to null", name, f.got, f.got)
			continue
		}
		if v != f.want {
			t.Errorf("%s = %v, want %v", name, v, f.want)
		}
	}

	if check.Parameters.Target != "https://example.com" {
		t.Errorf("target = %q", check.Parameters.Target)
	}
}

func TestCheckParametersNumericFieldsStayFloat64WhenNumbers(t *testing.T) {
	body := `{"parameters":{"sens":3,"threshold":5,"port":8443,"warningdays":30}}`

	var check Check
	if err := json.Unmarshal([]byte(body), &check); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for name, v := range map[string]interface{}{
		"sens":        check.Parameters.Sens,
		"threshold":   check.Parameters.Threshold,
		"port":        check.Parameters.Port,
		"warningdays": check.Parameters.WarningDays,
	} {
		if _, ok := v.(float64); !ok {
			t.Errorf("%s: got %T, want float64", name, v)
		}
	}
}

func TestCheckParametersNonNumericValuesPreserved(t *testing.T) {
	body := `{"parameters":{"sens":"","threshold":"high","port":null,"method":"GET"}}`

	var check Check
	if err := json.Unmarshal([]byte(body), &check); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if check.Parameters.Sens != "" {
		t.Errorf("empty string sens should be preserved, got %v", check.Parameters.Sens)
	}
	if check.Parameters.Threshold != "high" {
		t.Errorf("non-numeric threshold should be preserved, got %v", check.Parameters.Threshold)
	}
	if check.Parameters.Port != nil {
		t.Errorf("null port should stay nil, got %v", check.Parameters.Port)
	}
	if check.Parameters.Method != "GET" {
		t.Errorf("method = %q", check.Parameters.Method)
	}
}

func TestNormalizeNumeric(t *testing.T) {
	tests := []struct {
		in   interface{}
		want interface{}
	}{
		{"5", float64(5)},
		{"5.5", 5.5},
		{"-3", float64(-3)},
		{json.Number("42"), float64(42)},
		{int(7), float64(7)},
		{int64(8), float64(8)},
		{float64(9), float64(9)},
		{"", ""},
		{"abc", "abc"},
		{nil, nil},
		{true, true},
	}

	for _, tt := range tests {
		if got := normalizeNumeric(tt.in); got != tt.want {
			t.Errorf("normalizeNumeric(%#v) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestChecksMapDecodesWithMixedTypes(t *testing.T) {
	body := `{
		"A-1": {"_id":"A-1","interval":"5","parameters":{"sens":"2"}},
		"A-2": {"_id":"A-2","interval":5,"parameters":{"sens":2}}
	}`

	var checks map[string]Check
	if err := json.Unmarshal([]byte(body), &checks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for id, check := range checks {
		if string(check.Interval) != "5" {
			t.Errorf("%s: interval = %q, want \"5\"", id, check.Interval)
		}
		if v, ok := check.Parameters.Sens.(float64); !ok || v != 2 {
			t.Errorf("%s: sens = %#v, want float64(2)", id, check.Parameters.Sens)
		}
	}
}
