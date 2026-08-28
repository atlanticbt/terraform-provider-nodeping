package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// AdHocMaintenanceID is the sentinel NodePing accepts in the `id` field of a
// POST to create an ad-hoc (run-immediately) maintenance window.
//
// The provider never sends it. Ad-hoc records are deleted by NodePing as soon
// as the window finishes, so they cannot be represented as a durable resource:
// the next refresh would find nothing and the next apply would open another
// maintenance window, disabling the account's checks again. See
// docs/resources/nodeping_maintenance.md.
const AdHocMaintenanceID = "ad-hoc"

// maxChecklistEntries bounds the number of check IDs accepted in one
// maintenance window. NodePing accounts are far smaller than this; the cap
// exists so a malformed configuration cannot build an unbounded request body.
const maxChecklistEntries = 5000

// maxMaintenanceNameLength bounds the friendly name sent to the API.
const maxMaintenanceNameLength = 255

// maxMaintenanceDuration caps the window length at roughly one year in
// minutes. A longer window is almost certainly a units mistake (seconds or
// milliseconds) that would silently disable monitoring indefinitely.
const maxMaintenanceDuration = 527040

// MaintenanceRun records one completed maintenance run. NodePing keeps the ten
// most recent runs, keyed by the run's millisecond timestamp.
type MaintenanceRun struct {
	DisabledChecklist []string `json:"disabledchecklist,omitempty"`
	SkippedChecklist  []string `json:"skippedchecklist,omitempty"`
}

// MaintenanceUpdateInfo records who last edited the maintenance window.
type MaintenanceUpdateInfo struct {
	Timestamp int64  `json:"timestamp,omitempty"`
	User      string `json:"user,omitempty"`
}

// Maintenance is a NodePing maintenance window.
type Maintenance struct {
	// NodePing returns the identifier as `id` on this endpoint, but several
	// other endpoints use `_id`. Both are decoded so a response from either
	// shape yields an identifier; use Identifier to read it.
	ID           string `json:"id,omitempty"`
	UnderscoreID string `json:"_id,omitempty"`

	CustomerID string   `json:"customerid,omitempty"`
	Name       string   `json:"name,omitempty"`
	Enabled    bool     `json:"enabled"`
	Cron       string   `json:"cron,omitempty"`
	Duration   int      `json:"duration,omitempty"`
	Checklist  []string `json:"checklist,omitempty"`

	// Read-only server state.
	Processing bool                      `json:"processing,omitempty"`
	NextRun    *int64                    `json:"nextrun,omitempty"`
	History    map[string]MaintenanceRun `json:"history,omitempty"`
	Update     *MaintenanceUpdateInfo    `json:"update,omitempty"`
}

// Identifier returns the maintenance ID regardless of which JSON key carried it.
func (m *Maintenance) Identifier() string {
	if m.ID != "" {
		return m.ID
	}
	return m.UnderscoreID
}

// IsAdHoc reports whether the record is an ad-hoc window. Ad-hoc windows are
// removed by NodePing once they finish running.
func (m *Maintenance) IsAdHoc() bool {
	return strings.EqualFold(m.Identifier(), AdHocMaintenanceID)
}

// SortedHistory returns the recorded runs ordered oldest first. The API returns
// history as a map keyed by millisecond timestamp; ranging that map directly
// would produce Go's randomized iteration order and a result that changes on
// every read.
func (m *Maintenance) SortedHistory() []MaintenanceHistoryEntry {
	if len(m.History) == 0 {
		return nil
	}

	entries := make([]MaintenanceHistoryEntry, 0, len(m.History))
	for key, run := range m.History {
		ts, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			// Keep the entry rather than dropping data; an unparseable key
			// sorts before every real timestamp and is still identifiable by
			// its raw form.
			ts = 0
		}
		entries = append(entries, MaintenanceHistoryEntry{
			Key:               key,
			Timestamp:         ts,
			DisabledChecklist: run.DisabledChecklist,
			SkippedChecklist:  run.SkippedChecklist,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp != entries[j].Timestamp {
			return entries[i].Timestamp < entries[j].Timestamp
		}
		return entries[i].Key < entries[j].Key
	})

	return entries
}

// MaintenanceHistoryEntry is one run of a maintenance window, flattened out of
// the API's timestamp-keyed map into a deterministically ordered value.
type MaintenanceHistoryEntry struct {
	Key               string
	Timestamp         int64
	DisabledChecklist []string
	SkippedChecklist  []string
}

// MaintenanceCreateRequest is the body of POST /maintenance.
//
// `duration` and `checklist` are required by the API on every write, so neither
// carries omitempty.
type MaintenanceCreateRequest struct {
	Name      string   `json:"name"`
	Enabled   *bool    `json:"enabled,omitempty"`
	Cron      string   `json:"cron,omitempty"`
	Duration  int      `json:"duration"`
	Checklist []string `json:"checklist"`
}

// MaintenanceUpdateRequest is the body of PUT /maintenance. The API requires
// the id inside the body as well as in the path.
type MaintenanceUpdateRequest struct {
	ID        string   `json:"id"`
	Name      string   `json:"name,omitempty"`
	Enabled   *bool    `json:"enabled,omitempty"`
	Cron      string   `json:"cron,omitempty"`
	Duration  int      `json:"duration"`
	Checklist []string `json:"checklist"`
}

// validateMaintenanceName rejects an empty or oversized name.
func validateMaintenanceName(name string, required bool) error {
	if strings.TrimSpace(name) == "" {
		if required {
			return &ValidationError{Field: "name", Message: "maintenance name must not be empty"}
		}
		return nil
	}
	if len(name) > maxMaintenanceNameLength {
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("maintenance name must be at most %d characters", maxMaintenanceNameLength),
		}
	}
	if strings.ContainsAny(name, "\x00\r\n") {
		return &ValidationError{Field: "name", Message: "maintenance name contains illegal characters"}
	}
	return nil
}

func validateMaintenanceDuration(duration int) error {
	if duration < 1 {
		return &ValidationError{Field: "duration", Message: "duration must be at least 1 minute"}
	}
	if duration > maxMaintenanceDuration {
		return &ValidationError{
			Field:   "duration",
			Message: fmt.Sprintf("duration must be at most %d minutes", maxMaintenanceDuration),
		}
	}
	return nil
}

func validateChecklist(checklist []string) error {
	if len(checklist) == 0 {
		return &ValidationError{Field: "checklist", Message: "checklist must contain at least one check ID"}
	}
	if len(checklist) > maxChecklistEntries {
		return &ValidationError{
			Field:   "checklist",
			Message: fmt.Sprintf("checklist must contain at most %d check IDs", maxChecklistEntries),
		}
	}
	for _, id := range checklist {
		if err := validateResourceID("check", id); err != nil {
			return err
		}
	}
	return nil
}

// validateCron performs a cheap structural check. The authoritative validation
// lives in the resource schema; this guards direct client callers.
func validateCron(cron string) error {
	if strings.TrimSpace(cron) == "" {
		return nil
	}
	if strings.ContainsAny(cron, "\x00\r\n") {
		return &ValidationError{Field: "cron", Message: "cron expression contains illegal characters"}
	}
	if len(strings.Fields(cron)) != 5 {
		return &ValidationError{
			Field:   "cron",
			Message: "cron expression must have exactly 5 whitespace-separated fields",
		}
	}
	return nil
}

// normalizeChecklist returns a sorted copy with duplicates removed, and never
// returns nil so the required `checklist` key marshals as [] rather than null.
func normalizeChecklist(checklist []string) []string {
	seen := make(map[string]struct{}, len(checklist))
	out := make([]string, 0, len(checklist))
	for _, id := range checklist {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ListMaintenance returns every maintenance window on the account, keyed by ID.
func (c *Client) ListMaintenance(ctx context.Context) (map[string]Maintenance, error) {
	var result map[string]Maintenance
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/maintenance",
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to list maintenance windows: %w", err)
	}
	return result, nil
}

// GetMaintenance fetches a single maintenance window.
func (c *Client) GetMaintenance(ctx context.Context, id string) (*Maintenance, error) {
	if err := validateResourceID("maintenance", id); err != nil {
		return nil, err
	}

	var result Maintenance
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/maintenance/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return nil, &NotFoundError{ResourceType: "maintenance", ResourceID: id}
		}
		return nil, fmt.Errorf("failed to get maintenance window: %w", err)
	}

	// An empty body with a 200 is how NodePing reports "no such maintenance"
	// on some deployments. Treat it as absent rather than returning a hollow
	// record that would look like drift.
	if result.Identifier() == "" && result.Name == "" && result.Duration == 0 {
		return nil, &NotFoundError{ResourceType: "maintenance", ResourceID: id}
	}

	if result.Identifier() == "" {
		result.ID = id
	}

	return &result, nil
}

// CreateMaintenance creates a scheduled maintenance window.
//
// It deliberately provides no way to send `id: "ad-hoc"`. Ad-hoc windows are
// auto-deleted by NodePing when they finish, so they cannot be tracked in
// Terraform state without producing a create/destroy loop that repeatedly
// disables monitoring.
func (c *Client) CreateMaintenance(ctx context.Context, req MaintenanceCreateRequest) (*Maintenance, error) {
	if err := validateMaintenanceName(req.Name, true); err != nil {
		return nil, err
	}
	if err := validateMaintenanceDuration(req.Duration); err != nil {
		return nil, err
	}
	if err := validateChecklist(req.Checklist); err != nil {
		return nil, err
	}
	if err := validateCron(req.Cron); err != nil {
		return nil, err
	}
	req.Checklist = normalizeChecklist(req.Checklist)

	var result Maintenance
	if err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/maintenance",
		body:   req,
	}, &result); err != nil {
		return nil, fmt.Errorf("failed to create maintenance window: %w", err)
	}

	if result.Identifier() == "" {
		return nil, fmt.Errorf("failed to create maintenance window: API returned no id")
	}
	if result.IsAdHoc() {
		return nil, fmt.Errorf("failed to create maintenance window: API returned an ad-hoc record, which cannot be managed as a resource")
	}

	return &result, nil
}

// UpdateMaintenance modifies an existing maintenance window. `duration` and
// `checklist` are required by the API on PUT, so callers must always supply
// the full desired value rather than a partial patch.
func (c *Client) UpdateMaintenance(ctx context.Context, id string, req MaintenanceUpdateRequest) (*Maintenance, error) {
	if err := validateResourceID("maintenance", id); err != nil {
		return nil, err
	}
	if strings.EqualFold(id, AdHocMaintenanceID) {
		return nil, &ValidationError{Field: "id", Message: "ad-hoc maintenance windows cannot be updated"}
	}
	if err := validateMaintenanceName(req.Name, false); err != nil {
		return nil, err
	}
	if err := validateMaintenanceDuration(req.Duration); err != nil {
		return nil, err
	}
	if err := validateChecklist(req.Checklist); err != nil {
		return nil, err
	}
	if err := validateCron(req.Cron); err != nil {
		return nil, err
	}
	req.ID = id
	req.Checklist = normalizeChecklist(req.Checklist)

	var result Maintenance
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/maintenance/" + url.PathEscape(id),
		body:   req,
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return nil, &NotFoundError{ResourceType: "maintenance", ResourceID: id}
		}
		return nil, fmt.Errorf("failed to update maintenance window: %w", err)
	}

	if result.Identifier() == "" {
		result.ID = id
	}

	return &result, nil
}

// DeleteMaintenance removes a maintenance window. It reports a *NotFoundError
// when the window is already gone, which callers treat as success.
func (c *Client) DeleteMaintenance(ctx context.Context, id string) error {
	if err := validateResourceID("maintenance", id); err != nil {
		return err
	}

	var result DeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/maintenance/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return &NotFoundError{ResourceType: "maintenance", ResourceID: id}
		}
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}

	return nil
}
