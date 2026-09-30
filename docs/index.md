---
page_title: "NodePing Provider"
subcategory: ""
description: |-
  The NodePing provider allows you to manage NodePing monitoring resources including contacts and checks.
---

# NodePing Provider

The NodePing provider allows you to manage [NodePing](https://nodeping.com/) monitoring resources including contacts and checks.

> **These reference pages are hand-written and lag the schema.** They do not yet cover
> per-resource `customer_id` or the datasource filter arguments. The [README](../README.md) is
> authoritative until `tfplugindocs` generation lands — see [roadmap](roadmap.md).
>
> Installation is via the ABT private mirror, not a public registry. The `source` address
> below only resolves with a `provider_installation` block configured; see the README.

## Example Usage

```terraform
terraform {
  required_providers {
    nodeping = {
      source  = "atlanticbt.com/abt/nodeping"
      version = "~> 0.2"
    }
  }
}

provider "nodeping" {
  api_token = var.nodeping_api_token
}

resource "nodeping_contact" "email" {
  name = "DevOps Team"
  address {
    type    = "email"
    address = "devops@example.com"
  }
}

resource "nodeping_check" "website" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Website Monitor"
  enabled = true

  notifications {
    contact_id = nodeping_contact.email.address[0].id
    delay      = 0
    schedule   = "All"
  }
}
```

## Authentication

The provider requires an API token for authentication. You can configure it in the provider block or via environment variables.

### Provider Configuration

```terraform
provider "nodeping" {
  api_token = "your-api-token"
}
```

### Environment Variables

- `NODEPING_API_TOKEN` - API token for authentication
- `NODEPING_CUSTOMER_ID` - Default SubAccount customer ID
- `NODEPING_API_URL` - API base URL (for testing)

## Multi-Account Usage

Use provider aliases to manage multiple accounts or subaccounts:

```terraform
provider "nodeping" {
  alias     = "primary"
  api_token = var.primary_token
}

provider "nodeping" {
  alias       = "subaccount"
  api_token   = var.primary_token
  customer_id = "SUBACCOUNT_ID"
}

resource "nodeping_check" "primary_check" {
  provider = nodeping.primary
  type     = "HTTP"
  target   = "https://primary.example.com"
  label    = "Primary Account Check"
}

resource "nodeping_check" "subaccount_check" {
  provider = nodeping.subaccount
  type     = "HTTP"
  target   = "https://subaccount.example.com"
  label    = "SubAccount Check"
}
```

## Default Tags

You can define default tags at the provider level that will be automatically applied to all resources that support tags (e.g., checks):

```terraform
provider "nodeping" {
  api_token    = var.nodeping_token
  default_tags = ["managed-by-terraform", "team-devops"]
}

resource "nodeping_check" "example" {
  type   = "HTTP"
  target = "https://example.com"
  label  = "Example Check"
  tags   = ["production"]
}
```

The check is created with `["managed-by-terraform", "team-devops", "production"]`. Duplicates are removed.

The merged set is exposed as `tags_all`, not `tags`. On the resource, `tags` keeps exactly what the
configuration declares — Terraform requires an optional argument's planned value to match the
configuration, so writing the merge back into `tags` produced an invalid plan for any check that
declared tags of its own. Reference `nodeping_check.example.tags_all` when you need what NodePing
actually stores.

Changing `default_tags` re-plans every check the provider manages. A tag that is both a default and
declared on a check stays in that check's `tags` when the default is later removed from the
provider, because it was the configuration that asked for it.

## Schema

### Optional

- `api_token` (String, Sensitive) - NodePing API token. Can also be set via `NODEPING_API_TOKEN` environment variable.
- `customer_id` (String) - SubAccount customer ID for managing SubAccount resources. Can also be set via `NODEPING_CUSTOMER_ID` environment variable.
- `api_url` (String) - NodePing API base URL. Defaults to `https://api.nodeping.com/api/1`. Can also be set via the `NODEPING_API_URL` environment variable. **Must use `https`** unless the host is a loopback address (`localhost`, `127.0.0.0/8`, `::1`), which is permitted for local mock servers. A plaintext `http` endpoint would send the API token as HTTP basic auth in cleartext on every request. The same check is applied to the environment variable, which no attribute validator can see.
- `default_tags` (List of String) - Tags merged into every check this provider instance manages. They appear in each check's computed `tags_all`, never in its `tags`. Empty strings are rejected — a zero-length tag cannot be searched or filtered in NodePing and is almost always an unset variable.
- `rate_limit` (Number) - Maximum requests per second to the NodePing API. Must be greater than `0`. Defaults to `10`.
- `max_retries` (Number) - Maximum number of retries for failed requests. Must be `0` or greater; `0` disables retries. Defaults to `3`.
- `retry_wait_min` (Number) - Minimum wait time in seconds between retries. Must be `0` or greater, and must not exceed `retry_wait_max`. Defaults to `1`.
- `retry_wait_max` (Number) - Maximum wait time in seconds between retries. Must be `0` or greater. Defaults to `30`.
