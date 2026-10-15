---
page_title: "nodeping_accounts Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads the account of the API token and all of its subaccounts.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_accounts (Data Source)

Reads the account that owns the API token and all of its subaccounts.

## Example Usage

```hcl
data "nodeping_accounts" "all" {}

output "account_names" {
  value = { for a in data.nodeping_accounts.all.accounts : a.id => a.name }
}

# The primary account is the only entry with a subaccount count.
output "primary_account_id" {
  value = one([
    for a in data.nodeping_accounts.all.accounts : a.id
    if a.subaccount_count != null
  ])
}

output "suspended_subaccounts" {
  value = [
    for a in data.nodeping_accounts.all.accounts : a.name
    if a.status == "Suspend" && a.subaccount_count == null
  ]
}
```

## Argument Reference

This data source has no arguments.

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `accounts` | List of Object | The accounts that the API token can see, sorted by account ID. |
| `accounts[].id` | String | The NodePing account ID. |
| `accounts[].name` | String | The account name. |
| `accounts[].status` | String | The account status: `Active` or `Suspend`. |
| `accounts[].active_checks` | Number | The number of active checks in the account. |
| `accounts[].subaccount_count` | Number | The number of subaccounts. NodePing gives this value only for the primary account. It is `null` for each subaccount. Use it to identify the primary account. |

## Notes

- NodePing returns the accounts as a JSON (JavaScript Object Notation) object keyed by account ID. A JSON object has no order. The provider sorts the list by account ID, so the plan does not show a change at each read.
- If you set `customer_id` on the provider, NodePing limits the response to that one account. The list then has one entry. In that entry, `active_checks` is `0` and `subaccount_count` is `null`, because the limited response does not include them. To see all accounts, configure the provider without `customer_id`.
- The list has only summary fields. For the timezone, default locations, creation date or billing date, use the [`nodeping_account`](nodeping_account.md) data source.
- NodePing deletes accounts in a queue. A subaccount that you destroy with [`nodeping_subaccount`](../resources/nodeping_subaccount.md) can stay in this list for a short time.
