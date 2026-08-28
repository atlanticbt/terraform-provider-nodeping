# OpenTofu Provider for NodePing

Manages [NodePing](https://nodeping.com) monitoring as code — checks, contacts, and the
subaccounts they live in.

Atlantic BT maintains this provider for internal use. It is a fork of
[phizzl/terraform-provider-nodeping](https://github.com/phizzl/terraform-provider-nodeping)
(MPL-2.0); see [NOTICE.md](NOTICE.md) for provenance and attribution.

> **Targets OpenTofu, not Terraform.** Where the two diverge, OpenTofu wins. Some features
> this provider relies on — write-only attributes, provider `for_each` — behave differently
> or do not exist in Terraform. See [OpenTofu compatibility](#opentofu-compatibility).

## Status

| | |
|---|---|
| Provider address | `atlanticbt.com/abt/nodeping` |
| Requires | OpenTofu >= 1.12 |
| Distribution | ABT private provider mirror (not a public registry) |
| Test coverage | 94%+ statements |
| API coverage | 7 of 7 NodePing CRUD resources |

**Resources:** `nodeping_check` · `nodeping_contact` · `nodeping_contactgroup` ·
`nodeping_schedule` · `nodeping_notificationprofile` · `nodeping_maintenance` ·
`nodeping_subaccount`

**Data sources:** singular and plural forms of each of the above, plus `nodeping_account`
/ `nodeping_accounts`.

> **Not yet verified against the live NodePing API.** Every resource is unit-tested, but
> acceptance tests have never been executed — that needs a token for a dedicated test
> subaccount. See [A-010](docs/assumptions.md) for exactly what remains unverified before
> this should carry production monitoring.

## Installation

The provider is not published to a public registry. Install it from the ABT mirror by
adding a `provider_installation` block to `~/.tofurc`:

```hcl
provider_installation {
  network_mirror {
    url     = "https://tofu-mirror.example.abt/"
    include = ["atlanticbt.com/*/*"]
  }
  direct {
    exclude = ["atlanticbt.com/*/*"]
  }
}
```

The `exclude` on `direct` is required. Without it OpenTofu tries every matching
installation method and picks the newest across all of them, which means it will still
reach out to the public registry.

Then declare it as usual:

```hcl
terraform {
  required_version = ">= 1.12.0"
  required_providers {
    nodeping = {
      source  = "atlanticbt.com/abt/nodeping"
      version = "~> 0.1"
    }
  }
}
```

## Configuration

```hcl
provider "nodeping" {
  # Prefer NODEPING_API_TOKEN over an inline value.
  api_token = var.nodeping_api_token

  # Applied to every check this provider instance manages.
  default_tags = ["managed-by-tofu"]
}
```

| Argument | Env var | Required | Description |
|---|---|---|---|
| `api_token` | `NODEPING_API_TOKEN` | yes | Parent-account API token. Subaccounts have no token of their own. |
| `customer_id` | `NODEPING_CUSTOMER_ID` | no | Default subaccount for resources that do not set `customer_id`. |
| `api_url` | `NODEPING_API_URL` | no | Override the API base URL. Testing only. |
| `default_tags` | — | no | Tags merged into every managed check. |
| `rate_limit` | — | no | Requests per second. Default 10. |
| `max_retries` | — | no | Default 3. |

## Usage

```hcl
resource "nodeping_contact" "ops" {
  name = "Ops Team"

  address {
    type    = "email"
    address = "ops@example.com"
  }
}

resource "nodeping_check" "www" {
  label    = "example.com"
  type     = "HTTP"
  target   = "https://www.example.com"
  interval = 5
  enabled  = true
  tags     = ["tier-critical"]

  notifications {
    contact_id = nodeping_contact.ops.id
    delay      = 0
    schedule   = "All"
  }
}
```

### Subaccounts

Subaccounts have no API token. Everything is the parent token plus a `customer_id`, which
is settable per resource:

```hcl
resource "nodeping_check" "client_site" {
  customer_id = "201203232048C76FH"
  label       = "clientsite prod"
  type        = "HTTP"
  target      = "https://www.clientsite.com"
}
```

`customer_id` forces replacement — a check cannot move between accounts in place.

For fleets, use OpenTofu's provider `for_each` rather than hand-written aliases:

```hcl
provider "nodeping" {
  alias    = "sub"
  for_each = var.subaccounts

  api_token   = var.nodeping_api_token
  customer_id = each.value
}

resource "nodeping_check" "site" {
  for_each = var.monitored_sites   # must NOT be the same expression as the provider's

  provider = nodeping.sub[each.value.subaccount]
  label    = each.key
  type     = "HTTP"
  target   = each.value.url
}
```

The resource's `for_each` expression must differ from the provider's. This is deliberate
in OpenTofu: provider instances have to outlive the resources bound to them so that
destroy can still reach them.

### Credentials on checks

Check types that authenticate (`FTP`, `IMAP4`, `MYSQL`, `POP3`, `SMTP`, `SSH`) take a
password. Use `password_wo`, which is write-only — the value reaches NodePing but is
**never written to state or to a saved plan file**.

```hcl
ephemeral "aws_secretsmanager_secret_version" "db" {
  secret_id = "nodeping/db-monitor"
}

resource "nodeping_check" "db" {
  label    = "prod mysql"
  type     = "MYSQL"
  target   = "db.example.com"
  username = "monitoring"

  password_wo         = ephemeral.aws_secretsmanager_secret_version.db.secret_string
  password_wo_version = 1 # bump to rotate
}
```

Write-only values are invisible to diffing, so OpenTofu cannot detect that the secret
changed. Incrementing `password_wo_version` is what triggers an update.

Pairing it with an `ephemeral` block matters: a regular `data` source stores its result in
state, so reading the secret that way would put the plaintext in state even though the
check attribute is write-only. Ephemeral reads are not persisted, and OpenTofu rejects an
ephemeral value assigned to a non-write-only attribute — so the mistake is a plan-time
error rather than a silent leak.

The deprecated `password` attribute still works but **does** land in state in plaintext.
The two are mutually exclusive.

Note that NodePing stores and returns check passwords in plaintext through its own API
regardless. Write-only protects your state; it cannot protect their storage. Use a
least-privilege, monitoring-only credential for every authenticated check.

### Importing

```bash
tofu import nodeping_check.www CHECK-ID
tofu import nodeping_check.client 201203232048C76FH:CHECK-ID   # subaccount
```

The subaccount form persists `customer_id` into state, so subsequent plans keep addressing
the same account. Mirror the value in your configuration to avoid a replacement diff.

## Development

```bash
make build       # build the binary
make test        # unit tests with race detector
make cover       # coverage report
make lint        # golangci-lint
make vulncheck   # govulncheck
make install     # install into the local OpenTofu filesystem mirror
```

`make install` writes to `~/.local/share/opentofu/plugins/`. Point `~/.tofurc` at it with a
`filesystem_mirror` block to test local builds. See
[docs/development.md](docs/development.md).

### Acceptance tests

```bash
NODEPING_API_TOKEN=<test-subaccount-token> make test-acc
```

These create and destroy live NodePing resources. **Run them against a dedicated test
subaccount — never a client subaccount and never the main account.**

`make test-acc` sets four environment variables, all of which are required.
`TF_ACC_TERRAFORM_PATH` alone fails with a misleading "Invalid provider namespace" error,
because `terraform-plugin-testing` hardcodes `registry.terraform.io` and the legacy `-`
namespace, which OpenTofu rejects under any other host. Worse, if that variable is unset
the test harness silently downloads HashiCorp Terraform and tests the wrong binary.

## OpenTofu compatibility

Features this provider depends on that Terraform handles differently or not at all:

| Feature | Notes |
|---|---|
| Write-only attributes | OpenTofu 1.11+. Verified to keep values out of both state and saved plan files. |
| Provider `for_each` | OpenTofu 1.9+. No Terraform equivalent. |
| Resource identity | OpenTofu 1.12+. |
| State encryption | OpenTofu 1.7+. Recommended for consumers; not required by the provider. |

Do not add code that branches on `req.TerraformVersion`. OpenTofu 1.12 reports the literal
string `"1.13.0"` to every provider regardless of its actual version, and does not identify
itself as OpenTofu. Version gates written against it are silently meaningless. See
[CLAUDE.md](CLAUDE.md) for the full set of provider conventions.

## Security

Report anything sensitive through the internal ABT security channel, not a repository
issue. See [NOTICE.md](NOTICE.md) for the security history of this fork.

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
