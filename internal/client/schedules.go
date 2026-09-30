package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// maxScheduleIDLength bounds the caller-chosen schedule identifier. NodePing
// schedule IDs are short labels; anything longer is either a mistake or an
// attempt to abuse the URL path.
const maxScheduleIDLength = 100

// ScheduleDayNames lists the seven keys the NodePing schedules API accepts
// inside the `data` object, in week order. Iterate this slice instead of
// ranging a day map so every derived ordering is deterministic.
var ScheduleDayNames = []string{
	"monday",
	"tuesday",
	"wednesday",
	"thursday",
	"friday",
	"saturday",
	"sunday",
}

// scheduleTimePattern matches the H:MM / HH:MM 24-hour form NodePing uses for
// schedule spans.
var scheduleTimePattern = regexp.MustCompile(`^(?:[01]?[0-9]|2[0-3]):[0-5][0-9]$`)

// IsScheduleDayName reports whether name is one of the seven day keys.
func IsScheduleDayName(name string) bool {
	for _, day := range ScheduleDayNames {
		if day == name {
			return true
		}
	}
	return false
}

// ScheduleFlexBool is a boolean that survives NodePing's inconsistent encoding.
// The API accepts 0/1 on write and has been observed returning `true`, `1` and
// `"1"` for the same field, so decoding is permissive and encoding is always
// the documented 0/1 integer form.
type ScheduleFlexBool bool

func (b *ScheduleFlexBool) UnmarshalJSON(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to decode boolean field: %w", err)
	}

	switch v := raw.(type) {
	case nil:
		*b = false
	case bool:
		*b = ScheduleFlexBool(v)
	case float64:
		*b = ScheduleFlexBool(v != 0)
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "no", "off":
			*b = false
		default:
			*b = true
		}
	default:
		return fmt.Errorf("cannot decode %T as boolean", raw)
	}

	return nil
}

func (b ScheduleFlexBool) MarshalJSON() ([]byte, error) {
	if b {
		return []byte("1"), nil
	}
	return []byte("0"), nil
}

// Bool returns the plain Go boolean.
func (b ScheduleFlexBool) Bool() bool { return bool(b) }

// ScheduleDay is one day's notification window inside a schedule.
//
// time1/time2 delimit the span. Exclude inverts it, so notifications are sent
// all day *except* between time1 and time2. Disabled silences the day entirely
// and AllDay opens it entirely; the three are mutually exclusive in practice.
//
// The booleans are always serialised, never omitted: dropping `exclude: 0` from
// the payload means turning exclusion off never reaches the API and the
// resource drifts forever.
type ScheduleDay struct {
	Time1    string           `json:"time1,omitempty"`
	Time2    string           `json:"time2,omitempty"`
	Exclude  ScheduleFlexBool `json:"exclude"`
	Disabled ScheduleFlexBool `json:"disabled"`
	AllDay   ScheduleFlexBool `json:"allday"`
}

// Schedule is a named notification schedule. The ID is chosen by the caller,
// not by NodePing.
//
// ID is excluded from JSON on purpose. No schedules response carries a usable
// schedule identifier: GET /schedules/:id returns only the day map, and
// PUT /schedules/:id answers `{"ok":true,"id":"<account id>"}` — an `id` that
// names the account, not the schedule. Decoding it produced a schedule whose ID
// was the account ID. The request path and the list map key are the only
// authorities, and scheduleWithID is the one place either is applied.
type Schedule struct {
	ID         string                 `json:"-"`
	CustomerID string                 `json:"customer_id,omitempty"`
	Data       map[string]ScheduleDay `json:"data,omitempty"`
}

// SchedulePutRequest is the body of a PUT /schedules/:id. The same request
// performs both create and update — the schedules endpoint has no POST.
type SchedulePutRequest struct {
	ID   string                 `json:"id"`
	Data map[string]ScheduleDay `json:"data"`
}

// UnmarshalJSON tolerates the shapes NodePing has been observed returning for a
// schedule: the documented `{"id":..., "data":{...}}`, the Mongo-style `_id`,
// and a bare day map with no `data` wrapper. The `id`/`_id` field is read by
// nobody — see the note on Schedule.ID.
func (s *Schedule) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to decode schedule: %w", err)
	}

	s.CustomerID = firstString(raw, "customer_id", "customerid")

	if wrapped, ok := raw["data"]; ok {
		days, err := decodeScheduleDays(wrapped)
		if err != nil {
			return err
		}
		s.Data = days
		return nil
	}

	days := make(map[string]ScheduleDay, len(ScheduleDayNames))
	for key, value := range raw {
		name := strings.ToLower(strings.TrimSpace(key))
		if !IsScheduleDayName(name) {
			continue
		}
		var day ScheduleDay
		if err := json.Unmarshal(value, &day); err != nil {
			return fmt.Errorf("failed to decode schedule day %q: %w", name, err)
		}
		days[name] = day
	}
	if len(days) > 0 {
		s.Data = days
	}

	return nil
}

func firstString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var out string
		if err := json.Unmarshal(value, &out); err == nil && out != "" {
			return out
		}
	}
	return ""
}

// decodeScheduleDays normalises day keys to lower case and drops keys that are
// not day names, so a malformed response cannot inject unexpected entries.
func decodeScheduleDays(raw json.RawMessage) (map[string]ScheduleDay, error) {
	var decoded map[string]ScheduleDay
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode schedule data: %w", err)
	}
	if decoded == nil {
		return nil, nil
	}

	days := make(map[string]ScheduleDay, len(decoded))
	for key, day := range decoded {
		name := strings.ToLower(strings.TrimSpace(key))
		if !IsScheduleDayName(name) {
			continue
		}
		days[name] = day
	}
	return days, nil
}

// scheduleWithID decodes a schedules response body and stamps on the
// identifier the caller already knows. Routing every response through here is
// what keeps a stray `id` in the body from retargeting the resource.
func scheduleWithID(raw json.RawMessage, id string) (*Schedule, error) {
	var schedule Schedule
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &schedule); err != nil {
			return nil, err
		}
	}
	schedule.ID = id
	return &schedule, nil
}

func validateScheduleID(id string) error {
	if err := validateResourceID("schedule", id); err != nil {
		return err
	}
	if len(id) > maxScheduleIDLength {
		return &ValidationError{
			Field:   "id",
			Message: fmt.Sprintf("schedule ID must be at most %d characters", maxScheduleIDLength),
		}
	}
	if strings.ContainsAny(id, "/?#") {
		return &ValidationError{
			Field:   "id",
			Message: "schedule ID must not contain '/', '?' or '#'",
		}
	}
	return nil
}

func validateScheduleData(data map[string]ScheduleDay) error {
	if len(data) == 0 {
		return &ValidationError{
			Field:   "data",
			Message: "schedule must define at least one day",
		}
	}

	// Collect unknown keys first and report the alphabetically first one, so a
	// typo produces the same message on every run despite Go's randomised map
	// iteration order.
	unknown := ""
	for name := range data {
		if IsScheduleDayName(name) {
			continue
		}
		if unknown == "" || name < unknown {
			unknown = name
		}
	}
	if unknown != "" {
		return &ValidationError{
			Field:   "data",
			Message: fmt.Sprintf("%q is not a valid day name; expected one of monday..sunday", unknown),
		}
	}

	// Iterate the fixed day list rather than the map so the error reported for
	// a multi-day mistake is stable.
	for _, name := range ScheduleDayNames {
		day, ok := data[name]
		if !ok {
			continue
		}
		times := []struct {
			field string
			value string
		}{
			{"time1", day.Time1},
			{"time2", day.Time2},
		}
		for _, t := range times {
			if t.value == "" || scheduleTimePattern.MatchString(t.value) {
				continue
			}
			return &ValidationError{
				Field:   fmt.Sprintf("data.%s.%s", name, t.field),
				Message: fmt.Sprintf("%q is not a valid time; expected H:MM or HH:MM in 24-hour form", t.value),
			}
		}
	}

	return nil
}

// ListSchedules returns every schedule on the account, keyed by schedule ID.
func (c *Client) ListSchedules(ctx context.Context) (map[string]Schedule, error) {
	var result map[string]Schedule
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/schedules",
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	if result == nil {
		return map[string]Schedule{}, nil
	}

	// The map key is the schedule name, and it is the only identifier the list
	// response carries that means anything.
	for key, schedule := range result {
		schedule.ID = key
		result[key] = schedule
	}

	return result, nil
}

// GetSchedule fetches a single schedule. A missing schedule is reported as a
// *NotFoundError whether NodePing signals it with a 404 or with a 200 carrying
// an error body.
func (c *Client) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	if err := validateScheduleID(id); err != nil {
		return nil, err
	}

	var raw json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/schedules/" + url.PathEscape(id),
	}, &raw)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get schedule", "schedule", id)
	}

	schedule, err := scheduleWithID(raw, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	// A schedule with no days is how the API reports "no such schedule" on some
	// deployments; treat it the same as a 404 so Read can drop the resource
	// instead of writing an empty schedule to state.
	if err := absentAsNotFound(len(schedule.Data) == 0, "schedule", id); err != nil {
		return nil, err
	}

	return schedule, nil
}

// PutSchedule creates or replaces a schedule. The schedules endpoint exposes no
// POST: PUT is both the create and the update verb, and the ID is supplied by
// the caller rather than generated by NodePing.
func (c *Client) PutSchedule(ctx context.Context, id string, req SchedulePutRequest) (*Schedule, error) {
	if err := validateScheduleID(id); err != nil {
		return nil, err
	}
	if err := validateScheduleData(req.Data); err != nil {
		return nil, err
	}

	// The path is authoritative; never let a mismatched body ID retarget the
	// write.
	req.ID = id

	var raw json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/schedules/" + url.PathEscape(id),
		body:   req,
	}, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to save schedule: %w", err)
	}

	schedule, err := scheduleWithID(raw, id)
	if err != nil {
		return nil, fmt.Errorf("failed to save schedule: %w", err)
	}

	return schedule, nil
}

// DeleteSchedule removes a schedule. Deleting a schedule that is already gone
// reports a *NotFoundError, which callers treat as success.
func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	if err := validateScheduleID(id); err != nil {
		return err
	}

	var raw json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/schedules/" + url.PathEscape(id),
	}, &raw)
	if err != nil {
		return notFoundOrWrap(err, "failed to delete schedule", "schedule", id)
	}

	return nil
}
