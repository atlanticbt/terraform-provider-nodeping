---
page_title: "nodeping_contactgroup Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches a NodePing contact group by ID.
---

# nodeping_contactgroup (Data Source)

Fetches a single NodePing contact group by ID.

`members` holds contact **address** IDs, not contact IDs. A NodePing contact
owns one or more addresses, each with its own 8-character ID such as
`K5SP9CQP`, and it is the address that joins a group. See the
[`nodeping_contactgroup` resource](../resources/nodeping_contactgroup.md) for
the full explanation.

## Example Usage

```hcl
data "nodeping_contactgroup" "alerts" {
  id = "201205050153W2Q4C-G-ISYB7"
}

output "alert_group_name" {
  value = data.nodeping_contactgroup.alerts.name
}

# Address IDs, ready to reuse.
output "alert_group_address_ids" {
  value = data.nodeping_contactgroup.alerts.members
}
```

### Reusing an existing group's membership

Because both the data source and the resource expose `members` as a set of
address IDs, one can be composed from the other:

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

### A group in a SubAccount

```hcl
data "nodeping_contactgroup" "client" {
  id          = "201205050153W2Q4C-G-ISYB7"
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Required

- `id` (String) The unique identifier of the contact group.

### Optional

- `customer_id` (String) The customer ID (SubAccount ID) that owns this contact
  group. Set this to look up a group that belongs to a SubAccount. When omitted,
  it is populated from the API response.

### Read-Only

- `name` (String) The name of the contact group.
- `members` (Set of String) Contact address IDs that belong to this group. These
  are not contact IDs.
