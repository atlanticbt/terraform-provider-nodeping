---
page_title: "nodeping_account Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing account or subaccount.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_account (Data Source)

Reads one NodePing account or subaccount.

If you do not set `id`, the data source reads the account that owns the API token. Set `id` to read a subaccount.

## Example Usage

```hcl
# The account that owns the API token.
data "nodeping_account" "primary" {}

# A subaccount.
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

### Read a subaccount that you manage

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

## Argument Reference

| Argument | Required | Description |
|----------|----------|-------------|
| `id` | No | The NodePing account ID. If you do not set it, the data source finds the account that owns the API token and writes its ID here. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `name` | String | The account name. The API calls this field `customer_name`. |
| `type` | String | The account type, for example `customer` or `subcustomer`. |
| `status` | String | The account status: `Active` or `Suspend`. |
| `timezone` | String | The offset from GMT (Greenwich Mean Time), for example `"-7"`. The provider always returns a string, also when the API sends a number. |
| `creation_date` | Number | The time when NodePing created the account. |
| `next_billing_date` | String | The next billing date that NodePing reports. |
| `default_locations` | List of String | The default probe regions of the account. |

## Notes

- NodePing has no request that returns "my account". When `id` is not set, the provider reads the account list. The primary account is the only entry with a subaccount count. The provider then reads that account by ID.
- If the list does not identify one primary account, the read fails. Set `id` in that case.
- NodePing does not return `contactname`, `email`, `emailme` or `autodiagnotifications`. This data source cannot show them. The [`nodeping_subaccount`](../resources/nodeping_subaccount.md) resource also cannot read them after an import.
