---
page_title: "nodeping_contactgroups Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing contact groups in one account, sorted by contact group ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contactgroups (Data Source)

Reads all NodePing contact groups in one account.

In each contact group, `members` holds address IDs, not contact IDs. A NodePing contact owns one or more addresses. Each address has its own 8-character address ID, for example `K5SP9CQP`. A contact group holds addresses, not contacts. For more detail, see the [`nodeping_contactgroup`](../resources/nodeping_contactgroup.md) resource.

## Example Usage

```hcl
data "nodeping_contactgroups" "all" {}

output "group_names" {
  value = [for g in data.nodeping_contactgroups.all.contactgroups : g.name]
}
```

### Find a contact group by name

The API cannot search by name. Filter the full list:

```hcl
locals {
  prod_group = one([
    for g in data.nodeping_contactgroups.all.contactgroups : g
    if g.name == "Production Alerts"
  ])
}
```

### Find the contact groups of one address

```hcl
locals {
  groups_with_ops_email = [
    for g in data.nodeping_contactgroups.all.contactgroups : g.name
    if contains(g.members, nodeping_contact.ops.address[0].id)
  ]
}
```

### List the contact groups of a subaccount

```hcl
data "nodeping_contactgroups" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Optional

- `customer_id` (String) The customer ID of the subaccount to list. If you do not set it, the provider uses its own `customer_id`, or the account of the API token.

### Read-Only

- `contactgroups` (List of Object) The contact groups, sorted by contact group ID. See [Nested Schema for `contactgroups`](#nested-schema-for-contactgroups).

### Nested Schema for `contactgroups`

Read-Only:

- `id` (String) The contact group ID.
- `customer_id` (String) The customer ID of the account that owns the contact group.
- `name` (String) The name of the contact group.
- `members` (Set of String) The address IDs in the contact group. These are not contact IDs.
