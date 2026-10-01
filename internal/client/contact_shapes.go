package client

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ContactAddresses is a contact's addresses keyed by address ID. GET
// /contacts/:id returns a login contact's addresses as an array of
// {address, status} with no IDs; GET /contacts keys the same entries by the
// address itself, so the array form is keyed the same way.
type ContactAddresses map[string]ContactAddress

func (m *ContactAddresses) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		*m = nil
		return nil
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []ContactAddress
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return fmt.Errorf("contact addresses: %w", err)
		}
		out := make(ContactAddresses, len(list))
		for i, a := range list {
			if a.Address == "" {
				return fmt.Errorf("contact addresses: entry %d has no address", i)
			}
			out[a.Address] = a
		}
		*m = out
		return nil
	}
	var byID map[string]ContactAddress
	if err := json.Unmarshal(trimmed, &byID); err != nil {
		return fmt.Errorf("contact addresses: %w", err)
	}
	*m = byID
	return nil
}

// FlexStringMap holds a webhook's headers or query strings. NodePing allows a
// value to be a JSON object (an OpsGenie integration stores a custom field that
// way). Such values are kept as their JSON text and sent back as JSON, so an
// update does not turn the object into a string. Only text that parses as a
// JSON object or array is sent as JSON; every other value is sent as a string.
type FlexStringMap map[string]string

func (m *FlexStringMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		*m = nil
		return nil
	}
	out := make(FlexStringMap, len(raw))
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
			continue
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, v); err != nil {
			return fmt.Errorf("value for %q: %w", k, err)
		}
		out[k] = compact.String()
	}
	*m = out
	return nil
}

func (m FlexStringMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		t := bytes.TrimSpace([]byte(v))
		if len(t) > 0 && (t[0] == '{' || t[0] == '[') && json.Valid(t) {
			out[k] = json.RawMessage(t)
			continue
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = enc
	}
	return json.Marshal(out)
}
