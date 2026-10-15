---
page_title: "nodeping_contactgroup Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing contact group, a reusable set of notification addresses.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contactgroup (Resource)

Manages a NodePing contact group. A contact group is a set of addresses. A check can notify all of them through one contact group ID. The `members` attribute holds address IDs, not contact IDs. See [Members are address IDs](#members-are-address-ids).

> **CAUTION:** NodePing accepts a contact ID in `members` and returns no error. The contact group then notifies nobody. Use `nodeping_contact.<name>.address[<index>].id`, not `nodeping_contact.<name>.id`.

## Example Usage

### Two contacts and one contact group

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
    # The EMAIL address of the ops contact. Its SMS address (address[1])
    # is not in this group.
    nodeping_contact.ops.address[0].id,

    # The only address of the on-call contact.
    nodeping_contact.oncall.address[0].id,
  ]
}
```

### All addresses of a contact

```hcl
resource "nodeping_contactgroup" "all_ops_channels" {
  name    = "All Ops Channels"
  members = [for a in nodeping_contact.ops.address : a.id]
}
```

### Only addresses of one type

```hcl
resource "nodeping_contactgroup" "sms_only" {
  name = "SMS Escalation"
  members = [
    for a in nodeping_contact.ops.address : a.id if a.type == "sms"
  ]
}
```

### Contact group in a subaccount

```hcl
resource "nodeping_contactgroup" "client_alerts" {
  customer_id = "201205050153W2Q4C"
  name        = "Client Alerts"
  members     = [nodeping_contact.client_ops.address[0].id]
}
```

## Schema

### Optional

- `customer_id` (String) The customer ID of the subaccount that owns the contact group. Set it to manage a contact group in a subaccount. If you do not set it, the provider records the customer ID that NodePing returns. Changing it forces replacement.
- `members` (Set of String) The address IDs in the contact group. These are not contact IDs. The order is not significant. If NodePing returns the IDs in a different order, the plan shows no diff. If you omit `members` on create, the contact group has no members. If you remove `members` from an existing configuration, the provider keeps the current members. To remove all members, set `members = []`.
- `name` (String) The name of the contact group, used as a label. If you remove `name` from an existing configuration, the provider keeps the current name.

### Read-Only

- `id` (String) The contact group ID, in the format `<accountid>-G-XXXXX`, for example `201205050153W2Q4C-G-ISYB7`.

## Import

Import a contact group in the account of the API token by its contact group ID:

```shell
tofu import nodeping_contactgroup.production_alerts 201205050153W2Q4C-G-ISYB7
```

Import a contact group in a subaccount with the format `customer_id:contactgroup_id`:

```shell
tofu import nodeping_contactgroup.client_alerts 201205050153W2Q4C:201205050153W2Q4C-G-ISYB7
```

On OpenTofu 1.12 and later, you can also import with a resource identity. The identity has two attributes:

| Attribute | Required | Description |
|---|---|---|
| `id` | Yes | The contact group ID. |
| `customer_id` | No | The customer ID of the subaccount. Omit it for the account of the API token. |

```hcl
import {
  to = nodeping_contactgroup.client_alerts
  identity = {
    id          = "201205050153W2Q4C-G-ISYB7"
    customer_id = "201205050153W2Q4C"
  }
}
```

> **CAUTION:** Import writes `customer_id` to state, and the provider uses it for every later API call. `customer_id` forces replacement. If your configuration has a different value, the next plan replaces the contact group. Set `customer_id` in the configuration to the imported value.

## Members are address IDs

A NodePing contact is a person or an integration. A contact owns one or more addresses, for example an email address, a phone number, or a webhook URL. Each address has its own 8-character address ID, for example `K5SP9CQP`. A contact group holds addresses, not contacts. Thus you can put the email address of a contact in a daytime contact group and its SMS address in an after-hours contact group.

| Value | Example | Use in `members` |
|---|---|---|
| Contact ID | `201205050153W2Q4C-BKPGH` | No |
| Address ID | `K5SP9CQP` | Yes |

The [`nodeping_contact`](nodeping_contact.md) resource exports each address ID as `address[*].id`. Reference an address ID as `nodeping_contact.ops.address[0].id`.
