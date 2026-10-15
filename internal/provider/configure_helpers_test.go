package provider

import (
	"testing"
	"time"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStringFromConfigOrEnv(t *testing.T) {
	t.Setenv("NODEPING_TEST_VALUE", "from-env")

	if got := stringFromConfigOrEnv(types.StringValue("from-config"), "NODEPING_TEST_VALUE"); got != "from-config" {
		t.Errorf("a set config value should win, got %q", got)
	}
	if got := stringFromConfigOrEnv(types.StringValue(""), "NODEPING_TEST_VALUE"); got != "" {
		t.Errorf("an explicitly empty config value should not fall back to the environment, got %q", got)
	}
	if got := stringFromConfigOrEnv(types.StringNull(), "NODEPING_TEST_VALUE"); got != "from-env" {
		t.Errorf("a null config value should fall back to the environment, got %q", got)
	}
}

func TestRetryPolicyFrom(t *testing.T) {
	unset := NodePingProviderModel{
		MaxRetries:   types.Int64Null(),
		RetryWaitMin: types.Int64Null(),
		RetryWaitMax: types.Int64Unknown(),
	}
	got := retryPolicyFrom(unset)
	want := client.RetryPolicy{MaxRetries: client.DefaultMaxRetries, MinWait: client.DefaultRetryMinWait, MaxWait: client.DefaultRetryMaxWait}
	if got != want {
		t.Errorf("unset and unknown should keep the defaults: got %+v, want %+v", got, want)
	}

	zero := NodePingProviderModel{
		MaxRetries:   types.Int64Value(0),
		RetryWaitMin: types.Int64Value(0),
		RetryWaitMax: types.Int64Value(0),
	}
	if got := retryPolicyFrom(zero); got != (client.RetryPolicy{}) {
		t.Errorf("configured zeros should reach the client as zeros, got %+v", got)
	}

	set := NodePingProviderModel{
		MaxRetries:   types.Int64Value(5),
		RetryWaitMin: types.Int64Value(2),
		RetryWaitMax: types.Int64Value(9),
	}
	want = client.RetryPolicy{MaxRetries: 5, MinWait: 2 * time.Second, MaxWait: 9 * time.Second}
	if got := retryPolicyFrom(set); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRateLimitFrom(t *testing.T) {
	if got := rateLimitFrom(NodePingProviderModel{RateLimit: types.Float64Null()}); got != client.DefaultRateLimit {
		t.Errorf("unset should use the default, got %v", got)
	}
	if got := rateLimitFrom(NodePingProviderModel{RateLimit: types.Float64Value(2.5)}); got != 2.5 {
		t.Errorf("got %v, want 2.5", got)
	}
}
