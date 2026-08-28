package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// NotificationProfile is a reusable, named notification list. Its ID has the
// form "<accountid>-P-3QJWG".
//
// Notifications uses the same wire shape as a check's notifications: an array
// of single-key objects, where the key is a contact ID *or* a contact group ID
// and the value carries `delay` (minutes) and `schedule` (a schedule name):
//
//	[{"ABC-XYZ": {"delay": 0, "schedule": "Days"}}]
type NotificationProfile struct {
	ID            string                   `json:"_id,omitempty"`
	Type          string                   `json:"type,omitempty"`
	CustomerID    string                   `json:"customer_id,omitempty"`
	Name          string                   `json:"name,omitempty"`
	Notifications []map[string]interface{} `json:"notifications,omitempty"`
}

// NotificationProfileCreateRequest is the POST body. Both fields are required
// by the API, so neither carries omitempty.
type NotificationProfileCreateRequest struct {
	Name          string                   `json:"name"`
	Notifications []map[string]interface{} `json:"notifications"`
}

// NotificationProfileUpdateRequest is the PUT body. NodePing replaces the whole
// notifications array on every update - it never merges - so the complete list
// must be sent every time, including when it is empty.
type NotificationProfileUpdateRequest struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Notifications []map[string]interface{} `json:"notifications"`
}

// notificationsOrEmpty guarantees the array is serialised as `[]` rather than
// `null`. A null notifications field is rejected by the API, and it would also
// make "remove every notification" untransmittable.
func notificationsOrEmpty(n []map[string]interface{}) []map[string]interface{} {
	if n == nil {
		return []map[string]interface{}{}
	}
	return n
}

// ListNotificationProfiles returns every notification profile visible to the
// configured account, keyed by profile ID.
//
// NodePing's collection endpoints are documented as returning an object keyed
// by ID, but the notification profile endpoint was added later than the rest of
// the API and is not explicit about it, so an array response is accepted too.
func (c *Client) ListNotificationProfiles(ctx context.Context) (map[string]NotificationProfile, error) {
	var raw json.RawMessage
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/notificationprofiles",
	}, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification profiles: %w", err)
	}

	return decodeNotificationProfileCollection(raw)
}

func decodeNotificationProfileCollection(raw json.RawMessage) (map[string]NotificationProfile, error) {
	if len(raw) == 0 {
		return map[string]NotificationProfile{}, nil
	}

	var asMap map[string]NotificationProfile
	if err := json.Unmarshal(raw, &asMap); err == nil {
		if asMap == nil {
			asMap = map[string]NotificationProfile{}
		}
		for key, profile := range asMap {
			if profile.ID == "" {
				profile.ID = key
				asMap[key] = profile
			}
		}
		return asMap, nil
	}

	var asList []NotificationProfile
	if err := json.Unmarshal(raw, &asList); err != nil {
		return nil, fmt.Errorf("failed to decode notification profile collection: %w", err)
	}

	result := make(map[string]NotificationProfile, len(asList))
	for _, profile := range asList {
		if profile.ID == "" {
			continue
		}
		result[profile.ID] = profile
	}
	return result, nil
}

func (c *Client) GetNotificationProfile(ctx context.Context, id string) (*NotificationProfile, error) {
	if err := validateResourceID("notification profile", id); err != nil {
		return nil, err
	}

	var result NotificationProfile
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/notificationprofiles/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return nil, &NotFoundError{ResourceType: "notification profile", ResourceID: id}
		}
		return nil, fmt.Errorf("failed to get notification profile: %w", err)
	}

	// A profile that does not exist is reported by some NodePing endpoints as a
	// 200 with an empty body rather than a 404.
	if result.ID == "" {
		return nil, &NotFoundError{ResourceType: "notification profile", ResourceID: id}
	}

	return &result, nil
}

func (c *Client) CreateNotificationProfile(ctx context.Context, req NotificationProfileCreateRequest) (*NotificationProfile, error) {
	req.Notifications = notificationsOrEmpty(req.Notifications)

	var result NotificationProfile
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/notificationprofiles",
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification profile: %w", err)
	}
	return &result, nil
}

func (c *Client) UpdateNotificationProfile(ctx context.Context, id string, req NotificationProfileUpdateRequest) (*NotificationProfile, error) {
	if err := validateResourceID("notification profile", id); err != nil {
		return nil, err
	}

	req.ID = id
	req.Notifications = notificationsOrEmpty(req.Notifications)

	var result NotificationProfile
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/notificationprofiles/" + url.PathEscape(id),
		body:   req,
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return nil, &NotFoundError{ResourceType: "notification profile", ResourceID: id}
		}
		return nil, fmt.Errorf("failed to update notification profile: %w", err)
	}
	return &result, nil
}

// DeleteNotificationProfile removes a notification profile. It reports a
// *NotFoundError when the profile is already gone, which callers treat as
// success.
func (c *Client) DeleteNotificationProfile(ctx context.Context, id string) error {
	if err := validateResourceID("notification profile", id); err != nil {
		return err
	}

	var result DeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/notificationprofiles/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return &NotFoundError{ResourceType: "notification profile", ResourceID: id}
		}
		return fmt.Errorf("failed to delete notification profile: %w", err)
	}

	return nil
}
