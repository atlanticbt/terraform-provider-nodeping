package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) ListChecks(ctx context.Context) (map[string]Check, error) {
	var result map[string]Check
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/checks",
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to list checks: %w", err)
	}
	return result, nil
}

func (c *Client) GetCheck(ctx context.Context, id string) (*Check, error) {
	if err := validateResourceID("check", id); err != nil {
		return nil, err
	}

	var result Check
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodGet,
		path:   "/checks/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		return nil, notFoundOrWrap(err, "failed to get check", "check", id)
	}

	// A check that does not exist comes back as 200 with an empty document on
	// some deployments. Every real check carries an ID, a type and a target, so
	// a document with none of the three describes nothing.
	if err := absentAsNotFound(
		result.ID == "" && result.Type == "" && result.Parameters.Target == "",
		"check", id,
	); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) CreateCheck(ctx context.Context, req CheckCreateRequest) (*Check, error) {
	var result Check
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPost,
		path:   "/checks",
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to create check: %w", err)
	}
	return &result, nil
}

func (c *Client) UpdateCheck(ctx context.Context, id string, req CheckUpdateRequest) (*Check, error) {
	if err := validateResourceID("check", id); err != nil {
		return nil, err
	}

	var result Check
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodPut,
		path:   "/checks/" + url.PathEscape(id),
		body:   req,
	}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to update check: %w", err)
	}
	return &result, nil
}

// DeleteCheck removes a check. It is idempotent: deleting a check that is
// already gone reports a *NotFoundError, which callers treat as success.
func (c *Client) DeleteCheck(ctx context.Context, id string) error {
	if err := validateResourceID("check", id); err != nil {
		return err
	}

	var result DeleteResponse
	err := c.doRequest(ctx, requestOptions{
		method: http.MethodDelete,
		path:   "/checks/" + url.PathEscape(id),
	}, &result)
	if err != nil {
		return notFoundOrWrap(err, "failed to delete check", "check", id)
	}

	// NodePing returns ok:false (with HTTP 200) when the check did not exist.
	// A delete of an already-absent resource is a success for our purposes, so
	// this is deliberately not an error.
	return nil
}
