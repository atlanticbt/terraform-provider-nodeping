---
page_title: "NodePing Provider"
subcategory: ""
description: |-
  The NodePing provider manages NodePing monitoring: checks, contacts, contact groups, schedules, notification profiles, maintenance, and subaccounts.
---
<!--
doc: registry-index
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# NodePing Provider

The NodePing provider manages [NodePing](https://nodeping.com/) monitoring. It manages
checks, contacts, contact groups, schedules, notification profiles, maintenance, and
subaccounts. The provider targets OpenTofu 1.12 and later.

## Example Usage

```terraform
terraform {
  required_providers {
    nodeping = {
      source  = "atlanticbt/nodeping"
      version = "~> 1.1"
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

A check sends notifications to a contact address. Thus `notifications.contact_id` takes an
address ID (`address[0].id`) or a contact group ID. It does not take a contact ID.

## Authentication

The provider needs a NodePing API token. Set it in the provider block or in the
`NODEPING_API_TOKEN` environment variable. A value in the provider block overrides the
environment variable.

> **SECURITY:** Do not write the token into a `.tf` file. Use the environment variable, or a
> sensitive variable that gets its value from a secret store.

```terraform
variable "nodeping_api_token" {
  type      = string
  sensitive = true
}

provider "nodeping" {
  api_token = var.nodeping_api_token
}
```

The provider reads these environment variables:

| Variable | Argument |
|---|---|
| `NODEPING_API_TOKEN` | `api_token` |
| `NODEPING_CUSTOMER_ID` | `customer_id` |
| `NODEPING_API_URL` | `api_url` (for tests only) |

The provider reads no other variable for the token. Releases before the first public release
also read `NODEPING_API_KEY`. That fallback is removed.

## Subaccounts

A token for a parent account can manage each of its subaccounts. Select the subaccount with
`customer_id`. You can set `customer_id` on the provider, as a default, or on each resource.
A token that you create for a subaccount can reach only that subaccount.

For a small number of accounts, use provider aliases:

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

For many subaccounts, use the OpenTofu provider `for_each`. The
[README](https://github.com/atlanticbt/terraform-provider-nodeping#subaccounts) has an example.

## Default Tags

The provider can add tags to each resource that supports tags. At this time, only checks
support tags.

```terraform
provider "nodeping" {
  api_token    = var.nodeping_api_token
  default_tags = ["managed-by-tofu", "team-devops"]
}

resource "nodeping_check" "example" {
  type   = "HTTP"
  target = "https://example.com"
  label  = "Example Check"
  tags   = ["production"]
}
```

NodePing stores the check with `["managed-by-tofu", "team-devops", "production"]`. The
provider removes duplicate tags.

The merged set is in the computed attribute `tags_all`. The `tags` attribute keeps only the
tags that the configuration declares. OpenTofu requires that the planned value of an optional
argument agrees with the configuration. A merged value in `tags` thus caused an invalid plan.
Use `nodeping_check.example.tags_all` to get the tags that NodePing stores.

> **CAUTION:** A change to `default_tags` changes the plan for each check that the provider
> manages. Make the change when a large plan is acceptable.

Sometimes a tag is in `default_tags` and also in the `tags` of a check. If you remove that
tag from `default_tags`, it stays on the check. The configuration of the check still
declares it.

## Schema

### Optional

- `api_token` (String, Sensitive) The NodePing API token. You can also set it with the
  `NODEPING_API_TOKEN` environment variable.
- `customer_id` (String) The customer ID of the default subaccount. Resources that do not set
  `customer_id` use this subaccount. You can also set it with the `NODEPING_CUSTOMER_ID`
  environment variable.
- `api_url` (String) The NodePing API base URL. Default `https://api.nodeping.com/api/1`. You
  can also set it with the `NODEPING_API_URL` environment variable. The URL must use `https`.
  The only exception is a loopback host (`localhost`, `127.0.0.0/8`, or `::1`), for a local
  mock server. The provider sends the token with HTTP basic authentication, so plain `http`
  sends the token in cleartext. The provider applies the same rule to the environment
  variable.
- `default_tags` (List of String) Tags that the provider adds to each check that this
  provider instance manages. The tags appear in the computed `tags_all` of each check, never
  in its `tags`. Empty strings are not permitted. An empty tag cannot be found with a search
  in NodePing, and it is usually a variable with no value.
- `rate_limit` (Number) The maximum number of requests per second to the NodePing API. The
  value must be more than `0`. Default `10`.
- `max_retries` (Number) The maximum number of retries after a failed request. The value
  must be `0` or more. Set `0` to make one attempt only. Default `3`.
- `retry_wait_min` (Number) The minimum time in seconds between retries. The value must be
  `0` or more, and it must not be more than `retry_wait_max`. Set `0` to retry with no
  wait. Default `1`.
- `retry_wait_max` (Number) The maximum time in seconds between retries. The value must be
  `0` or more. Set `0` to retry with no wait. Default `30`.

## Request behavior

These limits apply to each API request. You cannot change them.

| Limit | Value |
|---|---|
| Timeout for one request | 30 seconds |
| Maximum size of a response | 16 MiB |

The provider retries a failed request only when a retry is safe:

- It retries `GET`, `PUT`, and `DELETE` after a network error or a `5xx` response.
- It retries `POST` only when the request did not reach NodePing, or when NodePing returned
  `429`. A `POST` that reached NodePing can create the object before it fails, so a retry can
  create a duplicate.
- After a `429`, it waits for the time in the `Retry-After` header. The wait is never more
  than `retry_wait_max`.
