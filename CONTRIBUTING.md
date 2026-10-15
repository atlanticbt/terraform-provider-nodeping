<!--
doc: contributing
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Contributing to the OpenTofu Provider for NodePing

This provider manages live monitoring. A change must pass tests against the real NodePing
API, not only against mocks.

## Before You Start

1. Read [README.md](README.md) to learn what the provider does.
2. Search the open issues. Do not do the same work twice.
3. For a large change, open an issue first. Agree on the approach before you write code.

> **SECURITY:** Never commit secrets, API keys, tokens or credentials. A pushed secret is
> hard to remove. See [SECURITY.md](SECURITY.md).

## Project Rules

These rules apply to each change. They apply to people and to automated tools.

| Rule | What it means |
|---|---|
| Security first | If you are not sure, choose the more secure option. Record the decision in [docs/assumptions.md](docs/assumptions.md). |
| No secrets in code, logs, test output or state | Tokens come from the environment. Credential attributes stay `Sensitive`. Credential fields are write-only. See [Secrets Must Not Reach State](#secrets-must-not-reach-state). |
| Validate and bound all input | Each schema attribute has validators with explicit limits. The API client checks IDs and sizes before it sends a request. |
| Test each validator and parser | Test valid, invalid, boundary and error cases. |
| Never ignore an error | Wrap the error with context. Return it as a diagnostic. |
| Separate the error types | A configuration mistake is an attribute error at plan time. An API failure names the endpoint and the status. |
| No `panic` | Provider code never calls `panic`. |
| Tests for each change | A bug fix starts with a regression test. The test must fail without the fix. |
| Deterministic tests | Tests do not depend on map order, time or shared state. |
| No dead code | No commented-out code, unused functions or stubs. |
| Justify each new dependency | Prefer the Go standard library. Give the reason for a new module in the pull request. |
| Write plainly | See [Writing Style](#writing-style). |

### Writing Style

Docs, comments and commit messages are specific and concrete.

- If you can delete a sentence and lose no information, delete it.
- Do not use stock phrases. Examples: "delve into", "seamlessly", "robust solution",
  "cutting-edge", "game-changer", "leverage" (as a verb), "it's important to note".
- Do not use "Not only... but also..." constructions.
- Do not put a three-item list in every sentence.
- Do not end with a summary that repeats the text above it.
- The project documentation follows ASD-STE100 Simplified Technical English where
  practical: one instruction in each sentence, active voice, short sentences.

## Fork Identity and License

| Item | Value |
|---|---|
| Provider address | `atlanticbt/nodeping` (`registry.opentofu.org/atlanticbt/nodeping`) |
| Go module path | `github.com/atlanticbt/terraform-provider-nodeping` |
| License | MPL-2.0 (Mozilla Public License 2.0), the same as upstream |

1. Do not use the upstream address `phizzl/nodeping` or the upstream module path. Both
   conflict with the upstream provider.
2. Do not change `LICENSE`.
3. Keep the upstream copyright notices in files that we did not write.
4. Keep modified MPL-covered files under MPL-2.0. MPL-2.0 is copyleft at the file level.
5. Update [NOTICE.md](NOTICE.md) when you take a change from upstream.

## Prerequisites

| Tool | Version |
|---|---|
| Go | The version in `go.mod`. The `toolchain` line sets the patch release. |
| OpenTofu (`tofu`) | 1.12 or later. Needed for manual tests and acceptance tests. |
| golangci-lint | v2.14.0. The configuration is `.golangci.yml`. |
| NodePing account | A test-only account. Needed for acceptance tests. |

## Workflow

1. Branch from `main`:

   ```bash
   git checkout main && git pull
   git checkout -b fix/short-description
   ```

2. Make your changes.
3. Format the code: `make fmt`.
4. Run `go vet`: `make vet`.
5. Run the linter: `make lint`.
6. Run the unit tests: `make test`.
7. Open a pull request with a clear title and description.

CI (continuous integration) runs the same checks: `fmt-vet`, `lint`, `test`, `vulncheck`
and `secret-scan`. CI also runs `trivy` and `workflow-audit` (zizmor), and reports the total
test coverage. See [docs/releasing.md](docs/releasing.md#ci-status-checks).

## Common Tasks

| Command | What it does |
|---|---|
| `make build` | Build the binary. |
| `make test` | Run the unit tests with the race detector. |
| `make cover` | Write `coverage.out` and `coverage.html`. |
| `make lint` | Run golangci-lint. |
| `make vet` | Run `go vet`. |
| `make vulncheck` | Run govulncheck. |
| `make fmt` | Run `gofmt -w .`. |

## Testing a Local Build

The fastest method is a `dev_overrides` block in `~/.tofurc`. It points OpenTofu to the
binary that you built. OpenTofu then does not install the provider.

1. Build the binary: `make build`.
2. Add this block to `~/.tofurc`. Use the absolute path to the binary:

   ```hcl
   provider_installation {
     dev_overrides {
       "registry.opentofu.org/atlanticbt/nodeping" = "/path/to/terraform-provider-nodeping"
     }
     direct {}
   }
   ```

3. Run `tofu plan` in a test configuration. You do not need `tofu init` for this provider.

`dev_overrides` ignores the dependency lock file. Do not use it in CI.

To test the installation itself, use `make install`. It copies the binary into a local
filesystem mirror. If you use a mirror, add an `exclude` for the provider to the `direct`
block. OpenTofu tries each matching installation method and selects the newest version.
Without the `exclude`, OpenTofu can ignore your local build and give no warning.

## Acceptance Tests

> **SECURITY:** Acceptance tests create and delete real NodePing checks, contacts,
> contact groups, schedules, maintenance windows and notification profiles. Use a
> NodePing account that holds nothing important. Never use an account with production
> monitoring.

1. Get an API token for the test account.
2. Run the tests:

   ```bash
   NODEPING_API_TOKEN='<token for the test account>' make test-acc
   ```

Some tests need objects that already exist in the test account. Name them with these
variables. A test skips if its fixture variable is not set.

| Variable | What it is |
|---|---|
| `NODEPING_TEST_CHECK_ID` | The ID of an existing check. Used by the check and maintenance tests. |
| `NODEPING_TEST_CONTACT_ID` | The ID of an existing contact *address*. Used by the notification profile tests. |
| `NODEPING_TEST_CUSTOMER_ID` | The customer ID of the test account. Used by the `customer_id` tests. |

`make test-acc` sets the four variables that the test harness needs to run OpenTofu:

```
TF_ACC=1
TF_ACC_TERRAFORM_PATH=<path to tofu>
TF_ACC_PROVIDER_HOST=registry.opentofu.org
TF_ACC_PROVIDER_NAMESPACE=hashicorp
```

All four are necessary:

- `terraform-plugin-testing` uses `registry.terraform.io` and the namespace `-` when it
  makes `TF_REATTACH_PROVIDERS`. OpenTofu rejects `-` for other hosts. The result is an
  "Invalid provider namespace" error at init that does not show the real cause.
- If `TF_ACC_TERRAFORM_PATH` is not set, the harness downloads HashiCorp Terraform and
  tests Terraform, not OpenTofu. It gives no warning.

### The Subaccount Test

The `acceptance_subaccount` build tag removes the subaccount lifecycle test from
`make test-acc`. Only `make test-acc-subaccount` runs it.

> **SECURITY:** This test needs a token for a parent account. It deletes the subaccount
> that it creates, and all the data in that subaccount. Use a parent account that exists
> only for tests.

### Rules for Each Acceptance Test

1. Use only `example.com` or `example.net` addresses and `+1555555xxxx` phone numbers.
2. Create contacts with `custrole = "notify"`. NodePing sends a welcome email with login
   details to a contact with `edit` or `view`.
3. Start each label and name with `acctest-`.
4. Make sure that the framework deletes each object that the test creates.
5. Check each `tfversion.SkipBelow` guard that you add. It compares version numbers, and
   OpenTofu version numbers do not mean the same as Terraform version numbers.

## OpenTofu First

This provider targets the latest production release of OpenTofu. Terraform support is
welcome, but no behavior can depend on Terraform only.

- **Do not read `req.TerraformVersion`.** OpenTofu sends a fixed Terraform version to each
  provider. A version gate that uses it does not work. If behavior must change, use a
  provider attribute or an environment variable.
- **Declare capabilities in the schema.** Let OpenTofu decide. Do not detect the host.
- **Do not implement `ListResource`, `PlanAction` or `InvokeAction`.** They are in the
  protocol definition, but OpenTofu can change its design for them.
- **Do not use `DeferralAllowed`.** OpenTofu 1.11 removed it. Return real errors.
- The provider uses plugin protocol 6. OpenTofu supports it. The written OpenTofu
  compatibility promise names protocol 5 only. `terraform-plugin-mux` can serve both.

Minimum versions: OpenTofu 1.12 and terraform-plugin-framework v1.19.0.

## Secrets Must Not Reach State

NodePing returns some credentials, for example check passwords, in plain text. The
provider controls only the state file.

1. Use a write-only attribute for each field that holds a credential. Add a matching
   `_wo_version` attribute.
2. Tell users to increase the `_wo_version` value to send a new credential. OpenTofu cannot
   see a change to a write-only value.
3. Read write-only values from `req.Config` in Create and Update. `req.Plan` and
   `req.State` are always null for these values.
4. In `Read`, never copy a credential from an API response into a normal attribute.
5. Also set `Sensitive: true`. It only hides the value in CLI output, so it does not replace
   write-only.

## Working on the Resource Layer

**`mapForApply` must not change a value that the user configured.** It starts from the plan
and fills only unknown attributes. `mapForRead` uses the API as the source of truth, so
that drift shows in the plan. If you merge the two paths, OpenTofu shows "Provider
produced inconsistent result after apply". If you add a restore for one field, look for
the place where the split was bypassed.

- Do not range over a Go map to make an ordered list. Go randomizes map order in each
  process, so plans change from run to run. Sort the keys first.
- Make an attribute `Optional + Computed` if the API normalizes its value.
- If NodePing returns a shape that the provider does not expect, record the real response
  in `docs/assumptions.md`. Add a regression test that uses that response.

## Go Conventions

- Wrap errors with context: `fmt.Errorf("creating check: %w", err)`.
- Do not discard an error with `_` in provider code. Do not call `panic`. Return a
  diagnostic.
- Give each API call a `context.Context`. The call must stop when the context is
  cancelled, and it must have a timeout.
- Log through `tflog` with structured fields.

> **SECURITY:** Never log the API token, a header that holds it, or a full request body.
> Log a failure with the endpoint, the status code and the object ID.

## Commits and Pull Requests

1. Branch from `main`. Open a pull request. Nobody pushes to `main` directly.
2. Use [Conventional Commits](https://www.conventionalcommits.org/). Example:
   `fix(check): keep the configured target when NodePing normalizes it`.
3. Add tests for each change. Add a regression test for each bug fix.
4. Update the docs in `docs/` when behavior changes. `internal/provider/docs_drift_test.go`
   fails when the schema and the docs do not agree.
5. Get one approval from a code owner (`@atlanticbt/it-admin`). CI must pass.

## PR Checklist

- [ ] Tests pass (`make test`).
- [ ] Lint passes (`make lint`).
- [ ] No new secrets or hard-coded credentials.
- [ ] Docs updated if behavior changed.
- [ ] One code owner approved the pull request.

## Code Style

Run `make fmt` and `make lint`. The linter configuration is `.golangci.yml`. It adds
`gosec`, `errorlint`, `bodyclose`, `noctx`, `unparam`, `misspell`, `gocyclo` and `nestif`
to the standard linters.

## Releases

A maintainer makes a release with a tag. Only CI builds releases. CI signs the checksums
and publishes to the registries. The steps are in [docs/releasing.md](docs/releasing.md).

## Upstream

This repository is a fork of
[`phizzl/terraform-provider-nodeping`](https://github.com/phizzl/terraform-provider-nodeping).
Do not merge upstream changes automatically.

1. Review the upstream change as a diff.
2. Take it through a normal pull request.
3. Get a security review if the change touches dependencies, token handling or the
   release pipeline.
4. Record the change in [NOTICE.md](NOTICE.md).

## Reporting Security Issues

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
