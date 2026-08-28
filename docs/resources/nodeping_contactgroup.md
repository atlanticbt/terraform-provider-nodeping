---
page_title: "nodeping_contactgroup Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing contact group, a reusable bundle of notification addresses.
---

# nodeping_contactgroup (Resource)

Manages a NodePing contact group. A group bundles notification targets so a
check can notify several people at once by referencing one group ID instead of
listing every recipient.

## Members are address IDs, not contact IDs

This is the one thing to get right. `members` holds contact **address** IDs.

A NodePing contact is a person or integration that owns one or more addresses —
an email, a phone number, a webhook URL. Each of those addresses has its own
8-character ID such as `K5SP9CQP`, and group membership is per address. That is
what lets you put a contact's email in the daytime group and its SMS number in
the after-hours group.

| Thing | Example | Belongs in `members`? |
|---|---|---|
| Contact ID | `201205050153W2Q4C-BKPGH` | No |
| Address ID | `K5SP9CQP` | Yes |

The API accepts a contact ID without complaint and produces a group that never
notifies anyone. The `nodeping_contact` resource exposes each address ID as
`address[*].id`, so the idiomatic reference is
`nodeping_contact.ops.address[0].id`.

## Example Usage

### Worked example: two contacts, one group

```hcl
resource "nodeping_contact" "ops" {
  name     = "Operations"
  custrole = "notify"

  address {
    type    = "email"
    address = "ops@example.com"
  }

  address {
    type    = "sms"
    address = "+15551230000"
  }
}

resource "nodeping_contact" "oncall" {
  name     = "On Call"
  custrole = "notify"

  address {
    type    = "email"
    address = "oncall@example.com"
  }
}

resource "nodeping_contactgroup" "production_alerts" {
  name = "Production Alerts"

  members = [
    # The ops contact's EMAIL address. Its SMS address (address[1]) is
    # deliberately left out of this group.
    nodeping_contact.ops.address[0].id,

    # The on-call contact's only address.
    nodeping_contact.oncall.address[0].id,
  ]
}
```

Writing `nodeping_contact.ops.id` in that list instead of
`nodeping_contact.ops.address[0].id` applies cleanly and silently produces a
group that notifies nobody.

### Every address a contact owns

```hcl
resource "nodeping_contactgroup" "all_ops_channels" {
  name    = "All Ops Channels"
  members = [for a in nodeping_contact.ops.address : a.id]
}
```

### Only addresses of a given type

```hcl
resource "nodeping_contactgroup" "sms_only" {
  name = "SMS Escalation"
  members = [
    for a in nodeping_contact.ops.address : a.id if a.type == "sms"
  ]
}
```

### A group in a SubAccount

```hcl
resource "nodeping_contactgroup" "client_alerts" {
  customer_id = "201205050153W2Q4C"
  name        = "Client Alerts"
  members     = [nodeping_contact.client_ops.address[0].id]
}
```

## Schema

### Optional

- `customer_id` (String) The customer ID (SubAccount ID) that owns this contact
  group. Set this to manage a group that belongs to a SubAccount. Changing it
  forces a new contact group to be created.
- `members` (Set of String) Contact **address** IDs that belong to this group.
  These are not contact IDs — see above. The order you write them in is not
  significant, and NodePing reordering them server-side does not produce a diff.
- `name` (String) The name of the contact group. Used as a label.

### Read-Only

- `id` (String) The unique identifier of the contact group, in the form
  `<accountid>-G-XXXXX` (for example `201205050153W2Q4C-G-ISYB7`).

## Import

Contact groups are imported by group ID:

```shell
terraform import nodeping_contactgroup.production_alerts 201205050153W2Q4C-G-ISYB7
```

For a group that belongs to a SubAccount, use `customer_id:group_id`:

```shell
terraform import nodeping_contactgroup.client_alerts 201205050153W2Q4C:201205050153W2Q4C-G-ISYB7
```

The `customer_id` is written to state on import and is used for every subsequent
API call for this resource. Set `customer_id` in your configuration to match
what was imported, otherwise the next plan shows a replacement.
