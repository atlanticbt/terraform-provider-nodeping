<!--
doc: env-vars
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Environment Variables

> **SECURITY:** Never log, share or commit values from `.env`. Rotate a token immediately
> if it is exposed.

1. Copy `.env.example` to `.env`.
2. Set `NODEPING_API_TOKEN`. It is the only value that you must set.
3. Load the file into your shell before you run OpenTofu or the tests.

`.gitignore` excludes `.env`. Do not change that.

## Variable Reference

### Provider Variables

The provider reads these variables when the matching argument is not set in the provider
block. A value in the provider block has priority.

| Variable | Required | Default | Description | Used in |
|---|---|---|---|---|
| `NODEPING_API_TOKEN` | Yes, if `api_token` is not set | None | The NodePing API token. | `internal/provider/provider.go:327` |
| `NODEPING_CUSTOMER_ID` | No | None | The default subaccount ID for resources that do not set `customer_id`. | `internal/provider/provider.go:328` |
| `NODEPING_API_URL` | No | `https://api.nodeping.com/api/1` | A different API base URL, for tests only. It must use HTTPS, except for a loopback host. | `internal/provider/provider.go:329` |

The provider does not read `NODEPING_API_KEY`. Earlier versions used it as an undocumented
fallback for the token. This is a breaking change in the Unreleased version. Set
`NODEPING_API_TOKEN` instead.

### Acceptance Test Variables

> **SECURITY:** These variables give the tests write access to a live NodePing account.
> Use a test-only account. Never use a token for an account with production monitoring.

| Variable | Required | Default | Description | Used in |
|---|---|---|---|---|
| `NODEPING_API_TOKEN` | Yes | None | The token for the test account. The tests also use it for direct API checks. | `internal/resources/contact/resource_acc_test.go:42` |
| `NODEPING_API_URL` | No | `https://api.nodeping.com/api/1` | The API base URL for direct API checks in tests. | `internal/resources/contact/resource_acc_shapes_test.go:20` |
| `NODEPING_TEST_CHECK_ID` | No | None | The ID of an existing check. The maintenance tests skip without it. | `internal/resources/maintenance/resource_acc_test.go:46` |
| `NODEPING_TEST_CONTACT_ID` | No | None | The ID of an existing contact address. The notification profile tests skip without it. | `internal/resources/notificationprofile/resource_acc_test.go:47` |
| `NODEPING_TEST_CUSTOMER_ID` | No | None | The customer ID of the test account. The `customer_id` tests skip without it. | `internal/resources/contact/resource_acc_test.go:51` |

### Test Harness Variables

`make test-acc` and `make test-acc-subaccount` set these variables. You do not need to
set them yourself.

| Variable | Value | Description | Used in |
|---|---|---|---|
| `TF_ACC` | `1` | Turns on the acceptance tests in `terraform-plugin-testing`. | `Makefile:49` |
| `TF_ACC_TERRAFORM_PATH` | Path to `tofu` | The binary that the harness runs. Without it, the harness downloads Terraform. | `Makefile:50` |
| `TF_ACC_PROVIDER_HOST` | `registry.opentofu.org` | The registry host for the provider under test. | `Makefile:51` |
| `TF_ACC_PROVIDER_NAMESPACE` | `hashicorp` | The namespace for the provider under test. OpenTofu rejects the default `-`. | `Makefile:52` |

### Make Variables

| Variable | Default | Description | Used in |
|---|---|---|---|
| `TOFU_BIN` | The `tofu` on your `PATH` | The OpenTofu binary for the acceptance tests. | `Makefile:11` |
| `VERSION` | `dev` | The version string in the binary and in the `make install` path. | `Makefile:4` |

### OpenTofu Logging

| Variable | Example | Description |
|---|---|---|
| `TF_LOG` | `DEBUG` | The log level for OpenTofu core and all providers. |
| `TF_LOG_PROVIDER` | `DEBUG` | The log level for providers only. |

> **SECURITY:** Debug logs can contain request and response bodies with check credentials
> and webhook tokens. Do not share debug logs before you remove the secrets.

## Getting Secret Values

1. Sign in to the NodePing web interface.
2. Open the account settings.
3. Make an API token.

> **SECURITY:** Give each environment its own token. Store pipeline tokens in the secrets
> store of the pipeline. For the scheduled acceptance tests, store the token and the
> fixture IDs as secrets of the protected `acceptance` environment. See
> [docs/releasing.md](releasing.md).
