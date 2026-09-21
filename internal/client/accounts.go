package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Account is the NodePing `/accounts/:id` document.
//
// Only the fields listed here are returned by a single-account GET. Notably
// absent are `contactname`, `email`, `emailme` and `autodiagnotifications`:
// those are write-only as far as the API is concerned and must never be
// refreshed from a response.
type Account struct {
	ID               string
	Type             string
	CustomerName     string
	CreationDate     int64
	Status           string
	Timezone         string
	NextBillingDate  string
	DefaultLocations []string
}

// accountWire mirrors Account with lenient field types. NodePing is
// inconsistent about whether `timezone` and `creation_date` arrive as JSON
// numbers or as strings, so both are decoded from the raw token.
type accountWire struct {
	ID               string          `json:"_id"`
	Type             string          `json:"type"`
	CustomerName     string          `json:"customer_name"`
	CreationDate     json.RawMessage `json:"creation_date"`
	Status           string          `json:"status"`
	Timezone         json.RawMessage `json:"timezone"`
	NextBillingDate  json.RawMessage `json:"nextBillingDate"`
	DefaultLocations []string        `json:"defaultlocations"`
}

func (a *Account) UnmarshalJSON(data []byte) error {
	var wire accountWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	a.ID = wire.ID
	a.Type = wire.Type
	a.CustomerName = wire.CustomerName
	a.Status = wire.Status
	a.DefaultLocations = wire.DefaultLocations
	a.Timezone = rawToString(wire.Timezone)
	a.NextBillingDate = rawToString(wire.NextBillingDate)

	created, err := rawToInt64(wire.CreationDate)
	if err != nil {
		return fmt.Errorf("account %q: creation_date: %w", wire.ID, err)
	}
	a.CreationDate = created

	return nil
}

// rawToString renders a JSON token as a plain string. A quoted string is
// unquoted; a number keeps its literal text so that a timezone of -7 and "-7"
// both become "-7". Anything else (null, object, array) becomes "".
func rawToString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	}
	if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return trimmed
	}
	return ""
}

// rawToInt64 accepts a JSON number, a numeric string, or null.
func rawToInt64(raw json.RawMessage) (int64, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, nil
	}
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("value %q is not numeric", trimmed)
	}
	return int64(value), nil
}

// AccountSummary is one entry of the `/accounts` listing. The listing is a JSON
// object keyed by account ID, not an array.
type AccountSummary struct {
	// Parent holds the number of subaccounts and is present only on the
	// primary account, which is what makes it usable to identify it.
	Parent *int   `json:"parent,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Count is the number of active checks in the account.
	Count int `json:"count"`
}

// SubaccountCreateRequest is the POST body for `/accounts`.
//
// POST creates subaccounts only. `id` and `customerid` are not supported on
// POST, and there is no API path that creates a top-level account.
type SubaccountCreateRequest struct {
	Name        string `json:"name"`
	ContactName string `json:"contactname"`
	Email       string `json:"email"`
	Timezone    string `json:"timezone"`
	Location    string `json:"location"`
	EmailMe     string `json:"emailme,omitempty"`
}

// AccountUpdateRequest is the PUT body for `/accounts/:id`.
//
// Every field is omitempty: a PUT that omits a field leaves it untouched, so a
// partial update is expressed by populating only what changed.
type AccountUpdateRequest struct {
	Name     string `json:"name,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Location string `json:"location,omitempty"`
	// AutodiagNotifications is the string "true" or "false", not a bool.
	AutodiagNotifications string `json:"autodiagnotifications,omitempty"`
	// EmailMe is the string "yes" or "no", not a bool.
	EmailMe string `json:"emailme,omitempty"`
	// Status is "Active" or "Suspend". It is rejected on a primary account.
	Status string `json:"status,omitempty"`
}

// IsEmpty reports whether the request would be a no-op PUT.
func (r AccountUpdateRequest) IsEmpty() bool {
	return r == AccountUpdateRequest{}
}

// subaccountCreateResponse covers both shapes NodePing has been observed to
// return from POST /accounts: a bare `{"id": ...}` acknowledgement and the
// created account document keyed by `_id`.
type subaccountCreateResponse struct {
	ID           string `json:"id"`
	UnderscoreID string `json:"_id"`
}

// AccountDeleteResponse is the DELETE /accounts acknowledgement. Deletion is
// queued, so a success here does not mean the account is gone yet.
type AccountDeleteResponse struct {
	Success string `json:"success"`
}

// ListAccounts returns the account listing keyed by account ID. Callers must
// sort the keys before producing ordered output.
//
// NodePing returns the ID-keyed listing only when the request carries no
// `customerid`. The shared client appends one whenever the provider is
// configured with a customer ID, and the API then answers with that single
// account document instead. Both shapes are normalised to the listing here so
// callers see one contract.
func (c *Client) ListAccounts(ctx context.Context) (map[string]AccountSummary, error) {
	var raw map[string]json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/accounts",
	}, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	// `_id` marks the single-account document; a listing is keyed by account ID
	// and never carries it at the top level. GetAccount detects it the same way.
	if _, ok := raw["_id"]; ok {
		id, summary, err := accountSummaryFromDocument(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to list accounts: %w", err)
		}
		return map[string]AccountSummary{id: summary}, nil
	}

	listing := make(map[string]AccountSummary, len(raw))
	for id, entry := range raw {
		var summary AccountSummary
		if err := json.Unmarshal(entry, &summary); err != nil {
			return nil, fmt.Errorf("failed to list accounts: account %q: %w", id, err)
		}
		listing[id] = summary
	}
	return listing, nil
}

// accountSummaryFromDocument turns the single-account document into a listing
// entry.
//
// The document names the account `customer_name` where the listing uses `name`,
// and it has no check count, so Count stays zero. Its `parent` holds the parent
// account's ID as a string, which is not what AccountSummary.Parent means:
// there Parent is the subaccount count that appears only on the primary
// account. Copying one into the other would make every subaccount look like the
// primary account, so the document's parent is deliberately dropped.
func accountSummaryFromDocument(raw map[string]json.RawMessage) (string, AccountSummary, error) {
	var id string
	if err := json.Unmarshal(raw["_id"], &id); err != nil {
		return "", AccountSummary{}, fmt.Errorf("account document: _id: %w", err)
	}
	if id == "" {
		return "", AccountSummary{}, fmt.Errorf("account document: _id is empty")
	}

	var summary AccountSummary
	if err := decodeStringField(raw, "customer_name", &summary.Name); err != nil {
		return "", AccountSummary{}, fmt.Errorf("account document %q: %w", id, err)
	}
	if err := decodeStringField(raw, "status", &summary.Status); err != nil {
		return "", AccountSummary{}, fmt.Errorf("account document %q: %w", id, err)
	}

	return id, summary, nil
}

// decodeStringField reads one optional string field out of a decoded JSON
// object, leaving the target untouched when the field is absent.
func decodeStringField(raw map[string]json.RawMessage, field string, target *string) error {
	entry, ok := raw[field]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(entry, target); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

// GetAccount fetches a single account. An empty id means "the account this API
// token belongs to", which the API exposes only through the listing, so the
// primary account is resolved from the listing first.
func (c *Client) GetAccount(ctx context.Context, id string) (*Account, error) {
	if id != "" {
		return c.getAccountByID(ctx, id)
	}

	var raw map[string]json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/accounts",
	}, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Some deployments answer a bare GET /accounts with the account document
	// itself rather than the listing. Detect that by the `_id` marker.
	if _, ok := raw["_id"]; ok {
		body, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to get account: %w", err)
		}
		var account Account
		if err := json.Unmarshal(body, &account); err != nil {
			return nil, fmt.Errorf("failed to get account: %w", err)
		}
		return &account, nil
	}

	primaryID, err := primaryAccountID(raw)
	if err != nil {
		return nil, err
	}
	return c.getAccountByID(ctx, primaryID)
}

func (c *Client) getAccountByID(ctx context.Context, id string) (*Account, error) {
	if err := validateResourceID("account", id); err != nil {
		return nil, err
	}

	var account Account
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/accounts/" + url.PathEscape(id),
	}, &account)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get account", "account", id)
	}

	// A deleted account can answer 200 with an empty body while the deletion
	// queue drains. Treat a document with no ID as absent.
	if err := absentAsNotFound(account.ID == "", "account", id); err != nil {
		return nil, err
	}

	return &account, nil
}

// primaryAccountID picks the primary account out of a listing. The primary
// account is the only entry that carries a `parent` (subaccount count) field.
func primaryAccountID(listing map[string]json.RawMessage) (string, error) {
	ids := make([]string, 0, len(listing))
	for id := range listing {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var withParent []string
	for _, id := range ids {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(listing[id], &entry); err != nil {
			continue
		}
		if _, ok := entry["parent"]; ok {
			withParent = append(withParent, id)
		}
	}

	switch {
	case len(withParent) == 1:
		return withParent[0], nil
	case len(withParent) == 0 && len(ids) == 1:
		return ids[0], nil
	case len(ids) == 0:
		return "", &NotFoundError{ResourceType: "account", ResourceID: ""}
	default:
		return "", &ValidationError{
			Field: "id",
			Message: "could not determine the primary account from the account listing; " +
				"set an explicit account id",
		}
	}
}

// CreateSubaccount creates a subaccount and returns its account ID.
//
// The shared client appends `customerid` to every request when the provider is
// configured with one. POST /accounts documents `customerid` as unsupported, so
// NodePing ignores it; a provider scoped to a subaccount cannot create further
// subaccounts regardless.
func (c *Client) CreateSubaccount(ctx context.Context, req SubaccountCreateRequest) (string, error) {
	if err := validateSubaccountCreateRequest(req); err != nil {
		return "", err
	}

	var result subaccountCreateResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/accounts",
		body:   req,
	}, &result)
	if err != nil {
		return "", fmt.Errorf("failed to create subaccount: %w", err)
	}

	id := result.ID
	if id == "" {
		id = result.UnderscoreID
	}
	if id == "" {
		return "", fmt.Errorf("failed to create subaccount: API response contained no account ID")
	}
	return id, nil
}

func validateSubaccountCreateRequest(req SubaccountCreateRequest) error {
	required := []struct {
		field string
		value string
	}{
		{"name", req.Name},
		{"contactname", req.ContactName},
		{"email", req.Email},
		{"timezone", req.Timezone},
		{"location", req.Location},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return &ValidationError{
				Field:   r.field,
				Message: r.field + " is required to create a subaccount",
			}
		}
	}
	return nil
}

// UpdateAccount applies a PUT to the given account. An empty id targets the
// primary account, which does not accept a `status` change.
func (c *Client) UpdateAccount(ctx context.Context, id string, req AccountUpdateRequest) error {
	path := "/accounts"
	if id != "" {
		if err := validateResourceID("account", id); err != nil {
			return err
		}
		path += "/" + url.PathEscape(id)
	}

	var result json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   path,
		body:   req,
	}, &result)
	if err != nil {
		return notFoundOrWrap(err, "failed to update account", "account", id)
	}
	return nil
}

// DeleteAccount queues an account for deletion. NodePing performs the removal
// asynchronously, so a subsequent GET may still return the account for a short
// while. Deleting an account destroys every check, contact and stored result it
// owns.
//
// Deleting the account the API token belongs to is not permitted by NodePing;
// this method refuses it locally rather than relying on the server.
func (c *Client) DeleteAccount(ctx context.Context, id string) error {
	if err := validateResourceID("account", id); err != nil {
		return err
	}
	if c.customerID != "" && c.customerID == id {
		return &ValidationError{
			Field:   "id",
			Message: "refusing to delete the account the provider is configured to operate as",
		}
	}

	var result AccountDeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method:     http.MethodDelete,
		path:       "/accounts/" + url.PathEscape(id),
		customerID: id,
	}, &result)
	if err != nil {
		return notFoundOrWrap(err, "failed to delete account", "account", id)
	}

	return nil
}
