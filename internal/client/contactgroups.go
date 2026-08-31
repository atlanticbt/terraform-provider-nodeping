package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// ContactGroup is a NodePing contact group as returned by /contactgroups.
//
// Members holds contact *address* IDs (the 8-character IDs of individual
// contact addresses), not contact IDs. A contact with three addresses can have
// any subset of those addresses in a group.
type ContactGroup struct {
	ID         string   `json:"_id"`
	Type       string   `json:"type"`
	CustomerID string   `json:"customer_id"`
	Name       string   `json:"name"`
	Members    []string `json:"members"`
}

// ContactGroupCreateRequest is the POST body for /contactgroups. The API
// ignores any `id` supplied on create, so the field is absent here.
type ContactGroupCreateRequest struct {
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members"`
}

// ContactGroupUpdateRequest is the PUT body for /contactgroups/:id. The API
// requires the group ID in the body as well as in the path.
type ContactGroupUpdateRequest struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members"`
}

// normalizeMembers guarantees an empty JSON array rather than `null` on the
// wire. Sending `null` leaves the existing membership untouched, so clearing a
// group would silently do nothing.
func normalizeMembers(members []string) []string {
	if members == nil {
		return []string{}
	}
	return members
}

func (c *Client) ListContactGroups(ctx context.Context) (map[string]ContactGroup, error) {
	var result map[string]ContactGroup
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/contactgroups",
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to list contact groups: %w", err)
	}
	return result, nil
}

func (c *Client) GetContactGroup(ctx context.Context, id string) (*ContactGroup, error) {
	if err := validateResourceID("contact group", id); err != nil {
		return nil, err
	}

	var result ContactGroup
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/contactgroups/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get contact group", "contact group", id)
	}

	// A GET for a non-existent group can come back 200 with an empty document
	// instead of 404. Treat a response with no ID as absent so Read removes the
	// resource from state rather than writing an empty group.
	if err := absentAsNotFound(result.ID == "", "contact group", id); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) CreateContactGroup(ctx context.Context, req ContactGroupCreateRequest) (*ContactGroup, error) {
	req.Members = normalizeMembers(req.Members)

	var result ContactGroup
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/contactgroups",
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to create contact group: %w", err)
	}
	return &result, nil
}

func (c *Client) UpdateContactGroup(ctx context.Context, id string, req ContactGroupUpdateRequest) (*ContactGroup, error) {
	if err := validateResourceID("contact group", id); err != nil {
		return nil, err
	}

	req.ID = id
	req.Members = normalizeMembers(req.Members)

	var result ContactGroup
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/contactgroups/" + url.PathEscape(id),
		body:   req,
	}, &result)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to update contact group", "contact group", id)
	}
	return &result, nil
}

// DeleteContactGroup removes a contact group. It is idempotent: deleting a
// group that is already gone reports a *NotFoundError, which callers treat as
// success.
func (c *Client) DeleteContactGroup(ctx context.Context, id string) error {
	if err := validateResourceID("contact group", id); err != nil {
		return err
	}

	var result DeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/contactgroups/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		return notFoundOrWrap(err, "failed to delete contact group", "contact group", id)
	}

	// ok:false with HTTP 200 means the group was already absent, which is a
	// successful outcome for a delete.
	return nil
}
