---
page_title: "nodeping_notificationprofile Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing notification profile by ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_notificationprofile (Data Source)

Reads one NodePing notification profile by ID. A notification profile is a named list of notification targets that checks can use.

## Example Usage

```hcl
data "nodeping_notificationprofile" "escalation" {
  id = "201205050153W2Q4C-P-3QJWG"
}

output "escalation_targets" {
  value = [for n in data.nodeping_notificationprofile.escalation.notifications : n.contact_id]
}
```

### Read a profile in a subaccount

```hcl
data "nodeping_notificationprofile" "client" {
  id          = "201205050153W2Q4C-P-3QJWG"
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

| Argument | Required | Description |
|----------|----------|-------------|
| `id` | Yes | The ID of the notification profile. |
| `customer_id` | No | The ID of the account that owns the profile. Set `customer_id` to read an object that belongs to a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `customer_id` | String | The ID of the account that owns the profile. |
| `name` | String | The name of the notification profile. |
| `notifications` | List of Object | The notification targets, in the order that NodePing gives them. |
| `notifications[].contact_id` | String | The ID of the contact address or contact group that gets the notification. |
| `notifications[].delay` | Number | The time before NodePing notifies this target, in minutes. |
| `notifications[].schedule` | String | The name of the notification schedule. |
