package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// runStringValidators feeds value through every validator declared on the named
// string attribute of the provider schema, the way the framework does at plan
// time.
func runStringValidators(t *testing.T, ctx context.Context, name string, value types.String) diag.Diagnostics {
	t.Helper()

	attribute, ok := providerSchema(t, ctx).Schema.Attributes[name].(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q is not a string attribute", name)
	}

	resp := validator.StringResponse{}
	for _, v := range attribute.Validators {
		v.ValidateString(ctx, validator.StringRequest{
			Path:        path.Root(name),
			ConfigValue: value,
		}, &resp)
	}

	return resp.Diagnostics
}

func runFloat64Validators(t *testing.T, ctx context.Context, name string, value types.Float64) diag.Diagnostics {
	t.Helper()

	attribute, ok := providerSchema(t, ctx).Schema.Attributes[name].(schema.Float64Attribute)
	if !ok {
		t.Fatalf("attribute %q is not a float64 attribute", name)
	}

	resp := validator.Float64Response{}
	for _, v := range attribute.Validators {
		v.ValidateFloat64(ctx, validator.Float64Request{
			Path:        path.Root(name),
			ConfigValue: value,
		}, &resp)
	}

	return resp.Diagnostics
}

func runInt64Validators(t *testing.T, ctx context.Context, name string, value types.Int64) diag.Diagnostics {
	t.Helper()

	attribute, ok := providerSchema(t, ctx).Schema.Attributes[name].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("attribute %q is not an int64 attribute", name)
	}

	resp := validator.Int64Response{}
	for _, v := range attribute.Validators {
		v.ValidateInt64(ctx, validator.Int64Request{
			Path:        path.Root(name),
			ConfigValue: value,
		}, &resp)
	}

	return resp.Diagnostics
}

func runListValidators(t *testing.T, ctx context.Context, name string, value types.List) diag.Diagnostics {
	t.Helper()

	attribute, ok := providerSchema(t, ctx).Schema.Attributes[name].(schema.ListAttribute)
	if !ok {
		t.Fatalf("attribute %q is not a list attribute", name)
	}

	resp := validator.ListResponse{}
	for _, v := range attribute.Validators {
		v.ValidateList(ctx, validator.ListRequest{
			Path:           path.Root(name),
			PathExpression: path.MatchRoot(name),
			ConfigValue:    value,
		}, &resp)
	}

	return resp.Diagnostics
}

// validateConfig runs the provider-level ValidateConfig over a provider block.
func validateConfig(t *testing.T, ctx context.Context, attrs configAttrs) fwprovider.ValidateConfigResponse {
	t.Helper()

	p := New("test")().(*NodePingProvider)
	req := fwprovider.ValidateConfigRequest{Config: newProviderConfig(t, ctx, attrs)}
	resp := fwprovider.ValidateConfigResponse{}
	p.ValidateConfig(ctx, req, &resp)

	return resp
}

func assertNoDiagErrors(t *testing.T, diags diag.Diagnostics, context string) {
	t.Helper()

	if diags.HasError() {
		t.Errorf("%s: expected no error diagnostics, got %v", context, diags)
	}
}

func assertDiagErrorContains(t *testing.T, diags diag.Diagnostics, want string) {
	t.Helper()

	if !diags.HasError() {
		t.Fatalf("expected an error diagnostic containing %q, got none", want)
	}

	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), want) || strings.Contains(d.Detail(), want) {
			return
		}
	}

	t.Errorf("expected an error diagnostic containing %q, got %v", want, diags)
}

func TestValidateAPIURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "https is accepted", url: "https://api.nodeping.com/api/1"},
		{name: "https with port is accepted", url: "https://api.example.com:8443/api/1"},
		{name: "scheme is compared case insensitively", url: "HTTPS://api.nodeping.com/api/1"},
		{name: "http on localhost is accepted", url: "http://localhost:8080"},
		{name: "http on 127.0.0.1 is accepted", url: "http://127.0.0.1:64321/api/1"},
		{name: "http on another 127 address is accepted", url: "http://127.9.9.9:8080"},
		{name: "http on ::1 is accepted", url: "http://[::1]:8080/api/1"},
		{name: "http on LOCALHOST is accepted", url: "HTTP://LOCALHOST:8080"},
		{
			name:    "http on a public host is rejected",
			url:     "http://api.nodeping.com/api/1",
			wantErr: "must be https for a non-loopback host",
		},
		{
			name:    "http on a non-loopback IP is rejected",
			url:     "http://10.0.0.5:8080",
			wantErr: "must be https for a non-loopback host",
		},
		{
			name:    "a non-http scheme is rejected",
			url:     "ftp://api.nodeping.com/api/1",
			wantErr: `the scheme must be https, but "ftp" was given`,
		},
		{
			name:    "a bare host is rejected as not absolute",
			url:     "api.nodeping.com/api/1",
			wantErr: "is not an absolute URL",
		},
		{
			name:    "a scheme with no host is rejected as not absolute",
			url:     "https:///api/1",
			wantErr: "is not an absolute URL",
		},
		{
			name:    "an unparseable URL is rejected without panicking",
			url:     "://nodeping",
			wantErr: "is not a valid URL",
		},
		{
			name:    "an unterminated IPv6 literal is rejected without panicking",
			url:     "http://[::1",
			wantErr: "is not a valid URL",
		},
		{
			name:    "a control character in the URL is rejected without panicking",
			url:     "https://api.nodeping.com/\x7f",
			wantErr: "is not a valid URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateAPIURL(tc.url)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected %q to be accepted, got %v", tc.url, err)
				}

				return
			}

			if err == nil {
				t.Fatalf("expected %q to be rejected", tc.url)
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "LocalHost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "127.0.0.53", want: true},
		{host: "::1", want: true},
		{host: "example.com", want: false},
		{host: "10.0.0.1", want: false},
		{host: "2001:db8::1", want: false},
		{host: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()

			if got := isLoopbackHost(tc.host); got != tc.want {
				t.Errorf("isLoopbackHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestAPIURLValidator_descriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	v := apiURLValidator{}

	if v.Description(ctx) == "" {
		t.Error("the api_url validator must describe itself for schema documentation")
	}

	if v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Error("expected the markdown description to match the plain description")
	}
}

func TestAPIURLValidator_acceptsNullUnknownAndEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, value := range map[string]types.String{
		"null falls back to the environment or the default": types.StringNull(),
		"unknown is resolved by a later apply":              types.StringUnknown(),
		"empty falls back to the client default":            types.StringValue(""),
	} {
		t.Run(name, func(t *testing.T) {
			assertNoDiagErrors(t, runStringValidators(t, ctx, "api_url", value), name)
		})
	}
}

func TestAPIURLValidator_schemeEnforcement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("https is accepted", func(t *testing.T) {
		assertNoDiagErrors(t, runStringValidators(t, ctx, "api_url", types.StringValue("https://api.nodeping.com/api/1")), "https")
	})

	t.Run("http on localhost is accepted", func(t *testing.T) {
		assertNoDiagErrors(t, runStringValidators(t, ctx, "api_url", types.StringValue("http://localhost:8080")), "loopback http")
	})

	t.Run("http on a public host is rejected", func(t *testing.T) {
		diags := runStringValidators(t, ctx, "api_url", types.StringValue("http://api.nodeping.com/api/1"))
		assertDiagErrorContains(t, diags, "Invalid NodePing API URL")
		assertDiagErrorContains(t, diags, "cleartext")
	})

	t.Run("a malformed URL is rejected", func(t *testing.T) {
		assertDiagErrorContains(t, runStringValidators(t, ctx, "api_url", types.StringValue("://nodeping")), "is not a valid URL")
	})
}

func TestPositiveFloat64Validator_descriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	v := positiveFloat64Validator{}

	if v.Description(ctx) == "" {
		t.Error("the rate_limit validator must describe itself for schema documentation")
	}

	if v.MarkdownDescription(ctx) != v.Description(ctx) {
		t.Error("expected the markdown description to match the plain description")
	}
}

func TestRateLimitValidator_bounds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		value     types.Float64
		wantError bool
	}{
		{name: "null is accepted", value: types.Float64Null()},
		{name: "unknown is accepted", value: types.Float64Unknown()},
		{name: "smallest positive value is accepted", value: types.Float64Value(0.001)},
		{name: "one is accepted", value: types.Float64Value(1)},
		{name: "ten is accepted", value: types.Float64Value(10)},
		{name: "zero is rejected", value: types.Float64Value(0), wantError: true},
		{name: "negative is rejected", value: types.Float64Value(-1), wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := runFloat64Validators(t, ctx, "rate_limit", tc.value)

			if tc.wantError {
				assertDiagErrorContains(t, diags, "must be greater than 0")
				return
			}

			assertNoDiagErrors(t, diags, tc.name)
		})
	}
}

func TestRetryAttributeValidators_bounds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		attribute string
		value     types.Int64
		wantError bool
	}{
		{name: "max_retries null is accepted", attribute: "max_retries", value: types.Int64Null()},
		{name: "max_retries unknown is accepted", attribute: "max_retries", value: types.Int64Unknown()},
		{name: "max_retries zero disables retries", attribute: "max_retries", value: types.Int64Value(0)},
		{name: "max_retries three is accepted", attribute: "max_retries", value: types.Int64Value(3)},
		{name: "max_retries negative one is rejected", attribute: "max_retries", value: types.Int64Value(-1), wantError: true},

		{name: "retry_wait_min null is accepted", attribute: "retry_wait_min", value: types.Int64Null()},
		{name: "retry_wait_min unknown is accepted", attribute: "retry_wait_min", value: types.Int64Unknown()},
		{name: "retry_wait_min zero is accepted", attribute: "retry_wait_min", value: types.Int64Value(0)},
		{name: "retry_wait_min negative one is rejected", attribute: "retry_wait_min", value: types.Int64Value(-1), wantError: true},

		{name: "retry_wait_max null is accepted", attribute: "retry_wait_max", value: types.Int64Null()},
		{name: "retry_wait_max unknown is accepted", attribute: "retry_wait_max", value: types.Int64Unknown()},
		{name: "retry_wait_max zero is accepted", attribute: "retry_wait_max", value: types.Int64Value(0)},
		{name: "retry_wait_max negative one is rejected", attribute: "retry_wait_max", value: types.Int64Value(-1), wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := runInt64Validators(t, ctx, tc.attribute, tc.value)

			if tc.wantError {
				assertDiagErrorContains(t, diags, "Invalid Attribute Value")
				return
			}

			assertNoDiagErrors(t, diags, tc.name)
		})
	}
}

func TestDefaultTagsValidator_rejectsEmptyStringTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		value     types.List
		wantError bool
	}{
		{
			name:  "null list is accepted",
			value: types.ListNull(types.StringType),
		},
		{
			name:  "unknown list is accepted",
			value: types.ListUnknown(types.StringType),
		},
		{
			name:  "empty list is accepted",
			value: types.ListValueMust(types.StringType, []attr.Value{}),
		},
		{
			name:  "non-empty tags are accepted",
			value: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("production")}),
		},
		{
			name:  "unknown element is accepted",
			value: types.ListValueMust(types.StringType, []attr.Value{types.StringUnknown()}),
		},
		{
			name:      "empty string tag is rejected",
			value:     types.ListValueMust(types.StringType, []attr.Value{types.StringValue("")}),
			wantError: true,
		},
		{
			name: "an empty tag among valid tags is rejected",
			value: types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("production"),
				types.StringValue(""),
			}),
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := runListValidators(t, ctx, "default_tags", tc.value)

			if tc.wantError {
				assertDiagErrorContains(t, diags, "Invalid Attribute Value Length")
				return
			}

			assertNoDiagErrors(t, diags, tc.name)
		})
	}
}

func TestProviderValidateConfig_retryWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		attrs     configAttrs
		wantError bool
	}{
		{
			name:  "both unset",
			attrs: configAttrs{"api_token": "token"},
		},
		{
			name:  "min below max",
			attrs: configAttrs{"retry_wait_min": int64(1), "retry_wait_max": int64(30)},
		},
		{
			name:  "min equal to max is the boundary and is accepted",
			attrs: configAttrs{"retry_wait_min": int64(30), "retry_wait_max": int64(30)},
		},
		{
			name:  "min set alone is compared against the client default",
			attrs: configAttrs{"retry_wait_min": int64(900)},
		},
		{
			name:  "max set alone",
			attrs: configAttrs{"retry_wait_max": int64(5)},
		},
		{
			name:  "unknown min does not fire the cross-field rule",
			attrs: configAttrs{"retry_wait_min": unknownValue, "retry_wait_max": int64(1)},
		},
		{
			name:  "unknown max does not fire the cross-field rule",
			attrs: configAttrs{"retry_wait_min": int64(90), "retry_wait_max": unknownValue},
		},
		{
			name:      "min one second above max is rejected",
			attrs:     configAttrs{"retry_wait_min": int64(31), "retry_wait_max": int64(30)},
			wantError: true,
		},
		{
			name:      "min far above max is rejected",
			attrs:     configAttrs{"retry_wait_min": int64(90), "retry_wait_max": int64(1)},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := validateConfig(t, ctx, tc.attrs)

			if tc.wantError {
				assertDiagErrorContains(t, resp.Diagnostics, "Invalid Retry Backoff Window")
				return
			}

			assertNoDiagErrors(t, resp.Diagnostics, tc.name)
		})
	}
}

// TestProviderValidateConfig_malformedConfigIsRejected covers the decode guard:
// a config whose raw value does not match the schema must produce diagnostics
// rather than panic.
func TestProviderValidateConfig_malformedConfigIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := providerSchema(t, ctx).Schema
	objType := s.Type().TerraformType(ctx).(tftypes.Object)

	brokenAttrTypes := map[string]tftypes.Type{}
	brokenValues := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		if name == "default_tags" {
			brokenAttrTypes[name] = tftypes.String
			brokenValues[name] = tftypes.NewValue(tftypes.String, "not-a-list")
			continue
		}

		brokenAttrTypes[name] = attrType
		brokenValues[name] = tftypes.NewValue(attrType, nil)
	}

	p := New("test")().(*NodePingProvider)
	req := fwprovider.ValidateConfigRequest{
		Config: tfsdk.Config{
			Schema: s,
			Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: brokenAttrTypes}, brokenValues),
		},
	}
	resp := fwprovider.ValidateConfigResponse{}
	p.ValidateConfig(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics for a config that does not match the schema")
	}
}

// TestProviderConfigure_apiURLSchemeIsEnforced covers the Configure-side check,
// which is the only place the NODEPING_API_URL environment variable is seen.
func TestProviderConfigure_apiURLSchemeIsEnforced(t *testing.T) {
	tests := []struct {
		name      string
		configURL interface{}
		envURL    string
		wantError bool
	}{
		{
			name:      "http from the provider block is rejected",
			configURL: "http://api.nodeping.com/api/1",
			wantError: true,
		},
		{
			name:      "http from NODEPING_API_URL is rejected",
			envURL:    "http://api.nodeping.com/api/1",
			wantError: true,
		},
		{
			name:      "a malformed NODEPING_API_URL is rejected",
			envURL:    "://nodeping",
			wantError: true,
		},
		{
			name:      "https from the provider block is accepted",
			configURL: "https://api.nodeping.com/api/1",
		},
		{
			name:   "https from NODEPING_API_URL is accepted",
			envURL: "https://api.example.com/api/1",
		},
		{
			name:      "http on loopback is accepted",
			configURL: "http://127.0.0.1:8080/api/1",
		},
		{
			name:      "an empty api_url falls back to the client default",
			configURL: "",
			envURL:    "https://api.example.com/api/1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearNodePingEnv(t)

			if tc.envURL != "" {
				t.Setenv("NODEPING_API_URL", tc.envURL)
			}

			attrs := configAttrs{"api_token": "token"}
			if tc.configURL != nil {
				attrs["api_url"] = tc.configURL
			}

			resp := configure(t, context.Background(), attrs)

			if tc.wantError {
				if !hasErrorSummary(t, resp, "Invalid NodePing API URL") {
					t.Errorf("expected an Invalid NodePing API URL diagnostic, got %v", resp.Diagnostics)
				}

				if resp.ResourceData != nil || resp.DataSourceData != nil {
					t.Error("no client must be handed out for an insecure or malformed API URL")
				}

				return
			}

			configuredClient(t, resp)
		})
	}
}
