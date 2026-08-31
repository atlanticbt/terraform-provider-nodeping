---
page_title: "nodeping_accounts Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches the NodePing account and all of its subaccounts.
---

# nodeping_accounts (Data Source)

Fetches the account the configured API token belongs to along with all of its subaccounts.

NodePing returns the listing as a JSON object keyed by account ID, which carries no
inherent order. The provider sorts entries by account ID so the list is stable between
reads and does not produce a spurious diff on every plan.

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

## Schema

### Read-Only

- `accounts` (List of Object) Accounts visible to the configured API token, sorted by
  account ID.

<a id="nestedatt--accounts"></a>
### Nested Schema for `accounts`

Read-Only:

- `id` (String) The NodePing account ID.
- `name` (String) The account name.
- `status` (String) The account status, `Active` or `Suspend`.
- `active_checks` (Number) The number of active checks in the account.
- `subaccount_count` (Number) The number of subaccounts. NodePing reports this only for the
  primary account; it is `null` for every subaccount, which makes it the way to tell the
  two apart.

## Notes

The listing carries only these summary fields. For an account's timezone, default probe
locations, creation date or billing date, use the
[`nodeping_account`](nodeping_account.md) data source.

A subaccount destroyed with [`nodeping_subaccount`](../resources/nodeping_subaccount.md)
may still appear here briefly. NodePing queues account deletions rather than performing
them inline, so the account outlives the successful destroy by a short interval.
