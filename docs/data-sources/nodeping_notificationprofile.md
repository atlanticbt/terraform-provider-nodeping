---
page_title: "nodeping_notificationprofile Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches a NodePing notification profile by ID.
---

# nodeping_notificationprofile (Data Source)

Fetches a NodePing notification profile by ID.

## Example Usage

```hcl
data "nodeping_notificationprofile" "escalation" {
  id = "201205050153W2Q4C-P-3QJWG"
}

output "escalation_targets" {
  value = [for n in data.nodeping_notificationprofile.escalation.notifications : n.contact_id]
}
```

Look a profile up on a subaccount:

```hcl
data "nodeping_notificationprofile" "client" {
  customer_id = "201205050153W2Q4C"
  id          = "201205050153W2Q4C-P-3QJWG"
}
```

## Argument Reference

- `id` - (Required) The unique identifier of the notification profile.
- `customer_id` - (Optional) The subaccount (customer) ID to look the profile up
  in. Defaults to the account the provider is configured for.

## Attribute Reference

- `name` - The name of the notification profile.
- `notifications` - Notification targets belonging to the profile, in the order
  NodePing reports them. Each entry contains:
  - `contact_id` - The contact address ID or contact group ID that is notified.
  - `delay` - Delay in minutes before this target is notified.
  - `schedule` - Notification schedule name.
