---
page_title: "nodeping_notificationprofile Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing notification profile.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_notificationprofile (Resource)

Manages a NodePing notification profile.

A notification profile is a named list of notification targets that you can use again.
Each entry points to a contact address or a contact group.
Each entry has its own delay and schedule.
Profile IDs have the form `<accountid>-P-3QJWG`.

## Example Usage

### A Contact and a Contact Group

```hcl
resource "nodeping_contact" "oncall" {
  name     = "On-Call Engineer"
  custrole = "notify"

  address {
    type    = "email"
    address = "oncall@example.com"
  }
}

resource "nodeping_notificationprofile" "escalation" {
  name = "Production Escalation"

  # A contact address: NodePing notifies it immediately.
  notifications {
    contact_id = nodeping_contact.oncall.address[0].id
    delay      = 0
    schedule   = "All"
  }

  # A contact group: NodePing notifies it 15 minutes later if the check is still down.
  notifications {
    contact_id = "201205050153W2Q4C-G-3QJWG"
    delay      = 15
    schedule   = "Days"
  }
}
```

### The Same Target More Than One Time

A contact can be in the list more than one time.
For example, you can notify it in business hours and again out of hours.
Each block is a separate entry.
The provider does not merge them.

```hcl
resource "nodeping_notificationprofile" "tiered" {
  name = "Tiered Paging"

  notifications {
    contact_id = nodeping_contact.oncall.address[0].id
    delay      = 0
    schedule   = "Days"
  }

  notifications {
    contact_id = nodeping_contact.oncall.address[0].id
    delay      = 30
    schedule   = "Nights"
  }
}
```

### A Profile in a Subaccount

```hcl
resource "nodeping_notificationprofile" "client" {
  customer_id = "201205050153W2Q4C"
  name        = "Client Escalation"

  notifications {
    contact_id = "201205050153W2Q4C-BKPGH"
  }
}
```

## How Updates Work

NodePing replaces the full list of targets on each update.
It does not merge the new list with the old list.
The provider always sends the full list.
Thus, when you remove a `notifications` block, NodePing removes that target.
When you remove all blocks, the profile has no targets.

## Argument Reference

- `name` - (Required) The name of the profile. 1 to 255 characters.
- `customer_id` - (Optional) The subaccount ID that owns this profile. At most 128 characters. The default is the account of the provider. If you change it, OpenTofu replaces the profile.
- `notifications` - (Optional) A block for one notification target. Repeat the block for each target.
  - `contact_id` - (Required) A contact address ID or a contact group ID. 1 to 128 characters.
  - `delay` - (Optional) The delay in minutes before NodePing notifies this target. It must be `0` or more. The default is `0`.
  - `schedule` - (Optional) The name of the notification schedule. At most 255 characters. The default is the `All` schedule of the account.

## Attribute Reference

- `id` - The unique identifier of the profile, in the form `<accountid>-P-3QJWG`.

## Import

Import a profile with its ID:

```shell
tofu import nodeping_notificationprofile.escalation 201205050153W2Q4C-P-3QJWG
```

For a profile in a subaccount, put the subaccount ID first.
The provider then sends all later requests to that subaccount.

```shell
tofu import nodeping_notificationprofile.client 201205050153W2Q4C:201205050153W2Q4C-P-3QJWG
```

You can also use an `import` block with an `identity`: set `id`, and set `customer_id` for a subaccount.
