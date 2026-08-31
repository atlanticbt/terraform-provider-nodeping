---
page_title: "nodeping_account Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches a NodePing account or subaccount.
---

# nodeping_account (Data Source)

Fetches a NodePing account or subaccount.

Omit `id` to read the account the configured API token belongs to. Supply an account ID to
read a specific subaccount.

## Example Usage

```hcl
# The account the API token belongs to.
data "nodeping_account" "primary" {}

# A specific subaccount.
data "nodeping_account" "client" {
  id = "201203232048C76FH"
}

output "primary_timezone" {
  value = data.nodeping_account.primary.timezone
}

output "client_is_suspended" {
  value = data.nodeping_account.client.status == "Suspend"
}
```

### Reading back a managed subaccount

```hcl
resource "nodeping_subaccount" "client" {
  name        = "Example Client"
  contactname = "Example Operations"
  email       = "ops@example.com"
  timezone    = "-7"
  location    = "nam"
}

data "nodeping_account" "client" {
  id = nodeping_subaccount.client.id
}
```

## Resolving the primary account

NodePing has no endpoint that returns "my account" directly. When `id` is omitted, the
provider reads the account listing and identifies the primary account as the only entry
that carries a subaccount count, then fetches that account by ID. If the listing is
ambiguous — several entries and no subaccount count on any of them — the read fails with a
diagnostic asking for an explicit `id` rather than guessing.

## Schema

### Optional

- `id` (String) The NodePing account ID. Leave unset to read the account the API token
  belongs to; the resolved ID is written back to this attribute.

### Read-Only

- `name` (String) The account name, returned by the API as `customer_name`.
- `type` (String) The account type, for example `customer` or `subcustomer`.
- `status` (String) The account status, `Active` or `Suspend`.
- `timezone` (String) The account's GMT offset as a string, for example `"-7"`. NodePing
  sometimes sends this as a JSON number; the provider normalises it to a string either way.
- `creation_date` (Number) The account creation timestamp reported by NodePing.
- `next_billing_date` (String) The next billing date reported by NodePing.
- `default_locations` (List of String) The account's default probe regions.

## Fields this data source cannot expose

NodePing does not return `contactname`, `email`, `emailme` or `autodiagnotifications` from
any read, so they are unavailable here and in
[`nodeping_subaccount`](../resources/nodeping_subaccount.md) after an import.
