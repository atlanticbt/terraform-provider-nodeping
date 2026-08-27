package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) ListContacts(ctx context.Context) (map[string]Contact, error) {
	var result map[string]Contact
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/contacts",
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}
	return result, nil
}

func (c *Client) GetContact(ctx context.Context, id string) (*Contact, error) {
	if err := validateResourceID("contact", id); err != nil {
		return nil, err
	}

	var result Contact
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/contacts/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return nil, &NotFoundError{ResourceType: "contact", ResourceID: id}
		}
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	return &result, nil
}

func (c *Client) CreateContact(ctx context.Context, req ContactCreateRequest) (*Contact, error) {
	var result Contact
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/contacts",
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to create contact: %w", err)
	}
	return &result, nil
}

func (c *Client) UpdateContact(ctx context.Context, id string, req ContactUpdateRequest) (*Contact, error) {
	if err := validateResourceID("contact", id); err != nil {
		return nil, err
	}

	var result Contact
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/contacts/" + url.PathEscape(id),
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to update contact: %w", err)
	}
	return &result, nil
}

// DeleteContact removes a contact. It is idempotent: deleting a contact that is
// already gone reports a *NotFoundError, which callers treat as success.
func (c *Client) DeleteContact(ctx context.Context, id string) error {
	if err := validateResourceID("contact", id); err != nil {
		return err
	}

	var result DeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/contacts/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return &NotFoundError{ResourceType: "contact", ResourceID: id}
		}
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	// ok:false with HTTP 200 means the contact was already absent, which is a
	// successful outcome for a delete.
	return nil
}
