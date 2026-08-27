# Development

## Prerequisites

- Go (version per `go.mod` — the `toolchain` directive pins the patch release)
- OpenTofu >= 1.12 (`tofu`), for manual and acceptance testing
- A NodePing API token for a **dedicated test subaccount**

## Common tasks

```bash
make build       # build the binary
make test        # unit tests, race detector
make cover       # coverage.out + coverage.html
make lint        # golangci-lint
make vet         # go vet
make vulncheck   # govulncheck
make fmt         # gofmt -w .
```

CI enforces an 80% statement coverage floor and fails the build below it.

## Testing a local build

`make install` builds and copies the binary into the local OpenTofu filesystem mirror at
`~/.local/share/opentofu/plugins/atlanticbt.com/abt/nodeping/<version>/<os>_<arch>/`.

Point `~/.tofurc` at it:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/Users/you/.local/share/opentofu/plugins"
    include = ["atlanticbt.com/*/*"]
  }
  direct {
    exclude = ["atlanticbt.com/*/*"]
  }
}
```

The `exclude` on `direct` is not optional. OpenTofu tries every matching installation
method and selects the newest version across all of them, so without it a local build can
be silently passed over.

For rapid iteration, `dev_overrides` skips the install step entirely:

```hcl
provider_installation {
  dev_overrides {
    "atlanticbt.com/abt/nodeping" = "/Users/you/git/terraform-provider-nodeping"
  }
  direct {}
}
```

`dev_overrides` bypasses the dependency lock file and makes `tofu init` unnecessary for
that provider. Never use it in CI.

## Acceptance tests

```bash
NODEPING_API_TOKEN=<test-subaccount-token> make test-acc
```

These create and destroy live NodePing checks and contacts. Run them against a dedicated
test subaccount only — never a client subaccount, never the main account.

`make test-acc` sets four environment variables:

```
TF_ACC=1
TF_ACC_TERRAFORM_PATH=<path to tofu>
TF_ACC_PROVIDER_HOST=registry.opentofu.org
TF_ACC_PROVIDER_NAMESPACE=hashicorp
```

All four are required. `terraform-plugin-testing` hardcodes the host
`registry.terraform.io` and tries the legacy `-` namespace when building
`TF_REATTACH_PROVIDERS`; OpenTofu rejects `-` under any other host, and the resulting
"Invalid provider namespace" error at init points nowhere useful.

If `TF_ACC_TERRAFORM_PATH` is unset, the harness downloads HashiCorp Terraform from
`releases.hashicorp.com` and tests that instead — silently. Always set it.

Audit any `tfversion.SkipBelow` guard you add. It compares raw version numbers, and
OpenTofu's numbering does not carry the same meaning as Terraform's, so a guard written
for a Terraform feature version can run against an OpenTofu release that lacks the
feature.

## Branching and commits

Per ABT standards:

- Branch from `main`: `feature/{TICKET}-description`, `bugfix/{TICKET}-description`,
  `hotfix/{TICKET}-description`, `ops/{TICKET}-description`
- Conventional Commits with the Jira ID: `fix(check): preserve password on apply [IT-1234]`
- Never push directly to `main`

See [CLAUDE.md](../CLAUDE.md) for the full PR checklist and Definition of Done.

## Working on the resource layer

The single most important convention: **`mapForApply` must never overwrite a value the
user configured.** It starts from the plan and fills only attributes that were unknown.
`mapForRead` takes the API as truth so that drift is visible.

Collapsing those two paths back into one mapper reintroduces the entire class of
"Provider produced inconsistent result after apply" failures that the split exists to
prevent. If you find yourself adding a per-field restore hack, the split has been
bypassed somewhere.

Related invariants:

- Never range a Go map into an ordered list. Map iteration order is randomized per
  process, and the result is nondeterministic plans or apply-consistency errors. Sort
  first.
- Attributes the API normalizes must be `Optional + Computed`.
- `Read` must never copy a credential from an API response into a regular attribute.

## Upstream changes

Upstream (`github.com/phizzl/terraform-provider-nodeping`) is a single-maintainer public
repository. Do not merge from it blindly. Review commits as a diff, adopt through a normal
PR, and require a security review for anything touching dependencies, token handling, or
the release pipeline. Record what was adopted in [NOTICE.md](../NOTICE.md).
