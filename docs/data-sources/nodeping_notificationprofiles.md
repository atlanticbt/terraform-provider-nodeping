---
page_title: "nodeping_notificationprofiles Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches all NodePing notification profiles.
---

# nodeping_notificationprofiles (Data Source)

Fetches all NodePing notification profiles, ordered by profile ID.

## Example Usage

```hcl
data "nodeping_notificationprofiles" "all" {}

output "profile_names" {
  value = { for p in data.nodeping_notificationprofiles.all.profiles : p.id => p.name }
}
```

Look profiles up on a subaccount:

```hcl
data "nodeping_notificationprofiles" "client" {
  customer_id = "201205050153W2Q4C"
}

locals {
  escalation_id = one([
    for p in data.nodeping_notificationprofiles.client.profiles :
    p.id if p.name == "Client Escalation"
  ])
}
```

## Argument Reference

- `customer_id` - (Optional) The subaccount (customer) ID to list profiles for.
  Defaults to the account the provider is configured for.

## Attribute Reference

- `profiles` - Notification profiles, ordered by ID. Each entry contains:
  - `id` - The unique identifier of the notification profile.
  - `customer_id` - The customer ID (account ID) that owns the profile.
  - `name` - The name of the notification profile.
  - `notifications` - Notification targets belonging to the profile. Each entry
    contains `contact_id`, `delay` and `schedule`.
