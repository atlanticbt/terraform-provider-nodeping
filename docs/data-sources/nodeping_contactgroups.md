---
page_title: "nodeping_contactgroups Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches all NodePing contact groups.
---

# nodeping_contactgroups (Data Source)

Fetches every NodePing contact group on the account, ordered by group ID.

Each group's `members` set holds contact **address** IDs, not contact IDs. See
the [`nodeping_contactgroup` resource](../resources/nodeping_contactgroup.md)
for why that distinction matters.

## Example Usage

```hcl
data "nodeping_contactgroups" "all" {}

output "group_ids" {
  value = [for g in data.nodeping_contactgroups.all.contactgroups : g.id]
}

output "group_names" {
  value = [for g in data.nodeping_contactgroups.all.contactgroups : g.name]
}
```

### Looking a group up by name

The API has no name lookup, so filter the full list:

```hcl
locals {
  prod_group = one([
    for g in data.nodeping_contactgroups.all.contactgroups : g
    if g.name == "Production Alerts"
  ])
}

output "prod_group_id" {
  value = local.prod_group.id
}
```

### Finding which groups an address belongs to

```hcl
locals {
  groups_with_ops_email = [
    for g in data.nodeping_contactgroups.all.contactgroups : g.name
    if contains(g.members, nodeping_contact.ops.address[0].id)
  ]
}
```

### Listing a SubAccount's groups

```hcl
data "nodeping_contactgroups" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Optional

- `customer_id` (String) The customer ID (SubAccount ID) to list contact groups
  for. Omit to list the groups of the account the provider is configured with.

### Read-Only

- `contactgroups` (List of Object) Contact groups, ordered by group ID. Each
  entry contains:
  - `id` (String) The unique identifier of the contact group.
  - `customer_id` (String) The customer ID that owns the group.
  - `name` (String) The name of the contact group.
  - `members` (Set of String) Contact address IDs in the group. These are not
    contact IDs.
