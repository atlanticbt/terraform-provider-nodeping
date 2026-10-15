---
page_title: "nodeping_contactgroup Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing contact group by contact group ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contactgroup (Data Source)

Reads one NodePing contact group by its contact group ID.

`members` holds address IDs, not contact IDs. A NodePing contact owns one or more addresses. Each address has its own 8-character address ID, for example `K5SP9CQP`. A contact group holds addresses, not contacts. For more detail, see the [`nodeping_contactgroup`](../resources/nodeping_contactgroup.md) resource.

## Example Usage

```hcl
data "nodeping_contactgroup" "alerts" {
  id = "201205050153W2Q4C-G-ISYB7"
}

output "alert_group_name" {
  value = data.nodeping_contactgroup.alerts.name
}
```

### Copy the members of a contact group into a new contact group

The data source and the resource both use a set of address IDs for `members`. Thus you can combine them:

```hcl
data "nodeping_contactgroup" "baseline" {
  id = "201205050153W2Q4C-G-ISYB7"
}

resource "nodeping_contactgroup" "escalation" {
  name = "Escalation"
  members = setunion(
    data.nodeping_contactgroup.baseline.members,
    [nodeping_contact.manager.address[0].id],
  )
}
```

### Read a contact group in a subaccount

```hcl
data "nodeping_contactgroup" "client" {
  id          = "201205050153W2Q4C-G-ISYB7"
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Required

- `id` (String) The contact group ID, for example `201205050153W2Q4C-G-ISYB7`.

### Optional

- `customer_id` (String) The customer ID of the subaccount that owns the contact group. Set it to read a contact group in a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. After the read, this attribute holds the customer ID that NodePing returns.

### Read-Only

- `name` (String) The name of the contact group.
- `members` (Set of String) The address IDs in the contact group. These are not contact IDs.
