# OpenTofu Provider for NodePing

Manages [NodePing](https://nodeping.com) monitoring as code — checks, contacts, and the
subaccounts they live in.

Developed at [Atlantic BT](https://www.atlanticbt.com) and maintained by Doug Eubanks
([@RealDougEubanks](https://github.com/RealDougEubanks)). It began as a fork of
[phizzl/terraform-provider-nodeping](https://github.com/phizzl/terraform-provider-nodeping)
by Phillip Schleicher (MPL-2.0); see [Attribution](#attribution) and [NOTICE.md](NOTICE.md).

> **Targets OpenTofu, not Terraform.** Where the two diverge, OpenTofu wins. Some features
> this provider relies on — write-only attributes, provider `for_each` — behave differently
> or do not exist in Terraform. See [OpenTofu compatibility](#opentofu-compatibility).

## Status

| | |
|---|---|
| Provider address | `atlanticbt/nodeping` |
| Requires | OpenTofu >= 1.12 |
| Registries | [OpenTofu Registry](https://search.opentofu.org/provider/atlanticbt/nodeping), Terraform Registry |
| API coverage | 7 of 7 NodePing CRUD resources |

**Resources:** `nodeping_check` · `nodeping_contact` · `nodeping_contactgroup` ·
`nodeping_schedule` · `nodeping_notificationprofile` · `nodeping_maintenance` ·
`nodeping_subaccount`

**Data sources:** singular and plural forms of each of the above, plus `nodeping_account`
/ `nodeping_accounts`.

Every resource is covered by acceptance tests that run against the live NodePing API, and
the provider has imported and managed a large production NodePing estate without drift.
Where NodePing's API behaves differently from its documentation, the observed behaviour
and the provider's handling of it are recorded in [docs/assumptions.md](docs/assumptions.md).

## Installation

```hcl
terraform {
  required_version = ">= 1.12.0"
  required_providers {
    nodeping = {
      source  = "atlanticbt/nodeping"
      version = "~> 1.1"
    }
  }
}
```

`tofu init` downloads it from the OpenTofu Registry. Releases are built by CI, and their
checksums are signed.

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
| `api_token` | `NODEPING_API_TOKEN` | yes | NodePing API token. A parent-account token reaches its subaccounts through `customer_id`; a token created for a subaccount reaches only that account. |
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

A parent-account token manages any of its subaccounts through `customer_id`, which is
settable per resource. (A token created in NodePing for a single subaccount needs no
`customer_id`; it can only reach that account.)

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

All resources also support **resource identity**, so an `import` block can address them
structurally instead of by parsing a colon-delimited string:

```hcl
import {
  to = nodeping_check.client
  identity = {
    id          = "CHECK-ID"
    customer_id = "201203232048C76FH"
  }
}
```

Identity is `id` + `customer_id` on every resource except `nodeping_subaccount`, which uses
`id` alone. Both import styles work; identity is additive.

## Development

```bash
make build       # build the binary
make test        # unit tests with race detector
make lint        # golangci-lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for local builds, acceptance tests, conventions and
the pull request process.

### Acceptance tests

```bash
NODEPING_API_TOKEN=<token for a test-only account> make test-acc
```

These create and destroy live NodePing resources. **Run them only against an account that
holds nothing you care about.** CONTRIBUTING.md lists the fixture variables and the
environment the harness needs.

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
[CONTRIBUTING.md](CONTRIBUTING.md) for the full set of provider conventions.

## Security

Please report vulnerabilities privately, as described in [SECURITY.md](SECURITY.md), not
in a public issue.

## Attribution

Developed at [Atlantic BT](https://www.atlanticbt.com) and maintained by Doug Eubanks
([@RealDougEubanks](https://github.com/RealDougEubanks)).

The original provider, including its plugin-framework layout, NodePing API client and
check and contact resources, was written by Phillip Schleicher as
[phizzl/terraform-provider-nodeping](https://github.com/phizzl/terraform-provider-nodeping).
This repository continues from that work under the same license.

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
