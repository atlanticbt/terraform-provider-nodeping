package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// insecureURLDetail explains why an http:// base URL is refused. NodePing
// authenticates with HTTP basic auth, so the API token is sent on every single
// request; over plain http it travels in cleartext.
const insecureURLDetail = "The NodePing API token is sent as HTTP basic auth on every request, so a plain http:// base URL " +
	"transmits it in cleartext. Use https://, or a loopback host (localhost, 127.0.0.1, ::1) when pointing at a local " +
	"mock server."

// validateAPIURL reports why raw is unsuitable as the NodePing API base URL, or
// nil if it is acceptable. https is required everywhere except loopback hosts,
// which stay usable for local mock servers.
func validateAPIURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not a valid URL: %w", raw, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%q is not an absolute URL; it must include a scheme and a host, for example https://api.nodeping.com/api/1", raw)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return nil
		}

		return errors.New("the scheme must be https for a non-loopback host")
	default:
		return fmt.Errorf("the scheme must be https, but %q was given", parsed.Scheme)
	}
}

// isLoopbackHost reports whether host addresses the local machine. url.Hostname
// has already stripped the port and any IPv6 brackets.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// apiURLValidator enforces validateAPIURL at plan time. It complements the
// identical check in Configure, which also covers the NODEPING_API_URL
// environment variable that no attribute validator can see.
type apiURLValidator struct{}

func (v apiURLValidator) Description(_ context.Context) string {
	return "must be an absolute https URL, or an http URL on a loopback host"
}

func (v apiURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v apiURLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	// Null means "fall back to the environment variable or the built-in
	// default"; unknown means the value comes from a resource that has not
	// been applied yet. Neither is something the practitioner can fix here.
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	raw := req.ConfigValue.ValueString()
	if raw == "" {
		return
	}

	if err := validateAPIURL(raw); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid NodePing API URL",
			err.Error()+". "+insecureURLDetail,
		)
	}
}

// positiveFloat64Validator requires a strictly positive value.
// terraform-plugin-framework-validators only offers an inclusive AtLeast bound
// for float64, which cannot express "greater than zero" without a second
// NoneOf validator and a second, confusing diagnostic.
type positiveFloat64Validator struct{}

func (v positiveFloat64Validator) Description(_ context.Context) string {
	return "must be greater than 0"
}

func (v positiveFloat64Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v positiveFloat64Validator) ValidateFloat64(_ context.Context, req validator.Float64Request, resp *validator.Float64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if value := req.ConfigValue.ValueFloat64(); value <= 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Attribute Value",
			fmt.Sprintf("Attribute %s must be greater than 0, got: %g.", req.Path, value),
		)
	}
}
