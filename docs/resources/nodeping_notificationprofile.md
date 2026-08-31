---
page_title: "nodeping_notificationprofile Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing notification profile.
---

# nodeping_notificationprofile (Resource)

Manages a NodePing notification profile.

A notification profile is a named, reusable list of notification targets. Each
entry points at a contact address or a contact group and carries its own delay
and schedule. Profile IDs have the form `<accountid>-P-3QJWG`.

## Example Usage

### Contact and Contact Group

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

  # A contact address: paged immediately.
  notifications {
    contact_id = nodeping_contact.oncall.address[0].id
    delay      = 0
    schedule   = "All"
  }

  # A contact group: paged 15 minutes later if the check is still down.
  notifications {
    contact_id = "201205050153W2Q4C-G-3QJWG"
    delay      = 15
    schedule   = "Days"
  }
}
```

### Repeated Targets

The same contact may appear more than once, for example to page during business
hours and again out of hours. Each block is a distinct entry; the provider never
merges them.

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

### Subaccount Profile

```hcl
resource "nodeping_notificationprofile" "client" {
  customer_id = "201205050153W2Q4C"
  name        = "Client Escalation"

  notifications {
    contact_id = "201205050153W2Q4C-BKPGH"
  }
}
```

## Update Semantics

NodePing replaces the entire notifications array on every update; it does not
merge. The provider always sends the complete list, so removing a
`notifications` block removes that target from the profile, and removing every
block leaves the profile with no targets.

## Argument Reference

- `name` - (Required) The name of the notification profile. 1-255 characters.
- `customer_id` - (Optional) The subaccount (customer) ID that owns this
  profile. Defaults to the account the provider is configured for. Changing it
  forces a new profile to be created.
- `notifications` - (Optional) Repeatable block describing one notification
  target:
  - `contact_id` - (Required) A contact address ID or a contact group ID.
  - `delay` - (Optional) Delay in minutes before this target is notified.
    Defaults to `0`.
  - `schedule` - (Optional) Notification schedule name. Defaults to the
    account's `All` schedule.

## Attribute Reference

- `id` - The unique identifier of the notification profile, in the form
  `<accountid>-P-3QJWG`.

## Import

Import an existing profile by ID:

```shell
tofu import nodeping_notificationprofile.escalation 201205050153W2Q4C-P-3QJWG
```

For a profile that lives on a subaccount, prefix the customer ID so subsequent
requests stay scoped to that account:

```shell
tofu import nodeping_notificationprofile.client 201205050153W2Q4C:201205050153W2Q4C-P-3QJWG
```
