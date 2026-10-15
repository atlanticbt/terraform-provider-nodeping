# Contributing

Thanks for helping. This provider manages live monitoring, so the bar for a change is
that it is tested against the real NodePing API, not only against mocks.

## Project rules

These apply to every change, whoever or whatever writes it.

- **Security first.** When in doubt, choose the more secure option and record the decision
  in [docs/assumptions.md](docs/assumptions.md).
- **No secrets in code, logs, test output or state.** Tokens come from the environment.
  Credential attributes stay `Sensitive`, and credential-bearing fields are write-only
  (see [Secrets must not reach state](#secrets-must-not-reach-state)).
- **Validate and bound input at every boundary.** Schema attributes carry validators with
  explicit limits, and the API client checks identifiers and sizes before building a
  request. Each validator and parser has tests for valid, invalid, boundary and error
  cases.
- **Never swallow an error.** Wrap it with context and return it as a diagnostic. Tell
  configuration mistakes (an attribute error at plan time) apart from API failures (an
  error naming the endpoint and status). No `panic` in provider code.
- **Tests for every change.** A bug fix starts with a regression test that fails without
  the fix. Tests are deterministic: no reliance on map order, timing or shared state.
- **No dead code**: no commented-out blocks, unused functions or stubs.
- **Justify every new dependency.** Prefer the standard library; a new module needs a
  reason in the pull request.
- **Write plainly.** Docs, comments and commit messages are specific and concrete. If a
  sentence can be deleted without losing information, delete it. Avoid stock phrases such
  as "delve into", "seamlessly", "robust solution", "cutting-edge", "game-changer",
  "leverage" (as a verb) and "it's important to note", as well as "Not only... but
  also..." constructions, stacked three-item lists in every sentence, and closing
  summaries that restate what was just said.

## Fork identity and license

| Thing | Value |
|---|---|
| Provider address | `atlanticbt/nodeping` (`registry.opentofu.org/atlanticbt/nodeping`) |
| Go module path | `github.com/atlanticbt/terraform-provider-nodeping` |
| License | MPL-2.0, unchanged from upstream |

- Never use the upstream address `phizzl/nodeping` or upstream's module path; both would
  collide with upstream resolution.
- Keep `LICENSE` unmodified and keep upstream copyright notices in files we did not
  write. MPL-2.0 is file-level copyleft: modified MPL-covered files stay under MPL-2.0.
- Keep [NOTICE.md](NOTICE.md) accurate when anything is adopted from upstream.

## Prerequisites

- Go, at the version in `go.mod` (the `toolchain` directive pins the patch release)
- OpenTofu 1.12 or later (`tofu`), for manual and acceptance testing
- `golangci-lint` v2.14.0
- For acceptance tests, a NodePing account used only for testing

## Common tasks

```bash
make build       # build the binary
make test        # unit tests with the race detector
make cover       # coverage.out and coverage.html
make lint        # golangci-lint
make vet         # go vet
make vulncheck   # govulncheck
make fmt         # gofmt -w .
```

Run `make fmt`, `make vet`, `make lint` and `make test` before pushing. CI runs the same
checks and enforces a coverage floor.

## Testing a local build

The quickest loop is a `dev_overrides` block in `~/.tofurc`, which points OpenTofu at the
binary you just built and skips installation:

```hcl
provider_installation {
  dev_overrides {
    "registry.opentofu.org/atlanticbt/nodeping" = "/path/to/terraform-provider-nodeping"
  }
  direct {}
}
```

`dev_overrides` bypasses the dependency lock file and makes `tofu init` unnecessary for
that provider. Never use it in CI.

To test installation itself, `make install` copies the binary into a local filesystem
mirror. If you use one, add an `exclude` for the provider to the `direct` block: OpenTofu
tries every matching installation method and picks the newest version across all of
them, so without the exclude a local build can be passed over silently.

## Acceptance tests

Acceptance tests create and destroy real NodePing checks, contacts, contact groups,
schedules, maintenance windows and notification profiles. **Run them only against a
NodePing account that holds nothing you care about.** Never point them at an account
with production monitoring.

```bash
NODEPING_API_TOKEN=<token for the test account> make test-acc
```

Some tests need fixtures that already exist in that account and are named through these
variables; tests whose fixture is missing skip rather than fail:

| Variable | What it is |
|---|---|
| `NODEPING_TEST_CHECK_ID` | ID of an existing check, used by the check and maintenance tests |
| `NODEPING_TEST_CONTACT_ID` | ID of an existing contact *address*, used by notification profile tests |
| `NODEPING_TEST_CUSTOMER_ID` | The test account's own customer ID, for the `customer_id` tests |

`make test-acc` sets the four variables the harness needs to drive OpenTofu:

```
TF_ACC=1
TF_ACC_TERRAFORM_PATH=<path to tofu>
TF_ACC_PROVIDER_HOST=registry.opentofu.org
TF_ACC_PROVIDER_NAMESPACE=hashicorp
```

All four are required. `terraform-plugin-testing` hardcodes `registry.terraform.io` and
tries the legacy `-` namespace when building `TF_REATTACH_PROVIDERS`; OpenTofu rejects
`-` under any other host, and the resulting "Invalid provider namespace" error at init
points nowhere useful. If `TF_ACC_TERRAFORM_PATH` is unset, the harness silently
downloads HashiCorp Terraform and tests that instead.

The subaccount lifecycle test is excluded from `make test-acc` by the
`acceptance_subaccount` build tag and runs only through `make test-acc-subaccount`.
Creating subaccounts needs a token for a parent account, and the test deletes the
subaccount it creates, along with everything inside it. Use a parent account that exists
only for testing.

Rules for every acceptance test:

- Use only `example.com` or `example.net` addresses and `+1555555xxxx` phone numbers.
- Create contacts with `custrole = "notify"`. NodePing sends a welcome email with login
  details to any contact created as `edit` or `view`.
- Start labels and names with `acctest-`, and leave the account as you found it. Every
  object a test creates must be destroyed by the framework.
- Audit any `tfversion.SkipBelow` guard you add. It compares raw version numbers, and
  OpenTofu's numbering does not mean the same thing as Terraform's.

## OpenTofu first

This provider targets the latest production OpenTofu release. Terraform compatibility is
welcome, but no Terraform-only behaviour may become load-bearing.

- **Never read `req.TerraformVersion`.** OpenTofu reports a fixed impersonated Terraform
  version to every provider, so a version gate written against it is meaningless. If
  behaviour must diverge, gate it on a provider attribute or an environment variable.
- **Declare capabilities through the schema** and let core decide. Do not feature-detect
  the host.
- **Do not implement `ListResource`, `PlanAction` or `InvokeAction`.** They appear in the
  vendored protocol definition, but OpenTofu has said it may diverge from Terraform's
  design there.
- **Never branch on `DeferralAllowed`.** OpenTofu removed it in 1.11. Return real errors.
- The provider speaks plugin protocol 6. OpenTofu supports it, though its written
  compatibility promise names protocol 5 only; `terraform-plugin-mux` can serve both if
  that ever matters.

Version floor: OpenTofu 1.12, terraform-plugin-framework v1.19.0.

## Secrets must not reach state

NodePing returns some credentials, such as check passwords, in plaintext. What the
provider controls is the state file.

- Use write-only attributes for every credential-bearing field, paired with a
  `_wo_version` attribute. Write-only values are invisible to diffing, so bumping the
  version is what triggers an update.
- Read write-only values from `req.Config` in Create and Update. `req.Plan` and
  `req.State` are always null for them.
- `Read` must never copy a credential from an API response into a regular attribute.
- `Sensitive: true` only redacts CLI output. Use it as well, never instead.

## Working on the resource layer

**`mapForApply` must never overwrite a value the practitioner configured.** It starts
from the plan and fills only attributes that were unknown. `mapForRead` takes the API as
truth so that drift is visible. Merging the two paths brings back "Provider produced
inconsistent result after apply" errors; if you find yourself adding a per-field restore,
the split has been bypassed somewhere.

- Never range over a Go map to build an ordered list. Map order is randomised per
  process, so plans become nondeterministic. Sort first.
- Attributes the API normalises must be `Optional + Computed`.
- When NodePing returns a shape the provider did not expect, record what the API actually
  returned in `docs/assumptions.md` and add a regression test built from that response.

## Go conventions

- Wrap errors with context (`fmt.Errorf("creating check: %w", err)`). Never discard an
  error with `_` in provider code, and never `panic`; return a diagnostic.
- Every API call takes a `context.Context`, honours cancellation and has a timeout.
- Log through `tflog` with structured fields. Never log the API token, headers carrying
  it, or full request bodies. Log failures with the endpoint, status code and object ID.

## Commits and pull requests

- Branch from `main` and open a pull request; nothing is pushed to `main` directly.
- Use [Conventional Commits](https://www.conventionalcommits.org/):
  `fix(check): keep the configured target when NodePing normalises it`.
- Every change needs tests. Every bug fix needs a regression test that fails without the
  fix.
- Update the docs under `docs/` when behaviour changes. `docs_drift_test.go` fails when
  the schema and the docs disagree.
- Each pull request needs one approving review from a code owner, and CI must pass.

## Releases

Releases are cut by tagging and built only by CI, which signs the checksums and publishes
to the registries. The steps are in [docs/releasing.md](docs/releasing.md).

## Upstream

This repository is a fork of
[`phizzl/terraform-provider-nodeping`](https://github.com/phizzl/terraform-provider-nodeping).
Upstream changes are not merged blindly: review them as a diff, adopt them through a
normal pull request, and get a security review for anything touching dependencies, token
handling or the release pipeline. Record what was adopted in [NOTICE.md](NOTICE.md).

## Reporting security issues

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
