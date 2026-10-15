---
page_title: "nodeping_notificationprofiles Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing notification profiles, sorted by profile ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_notificationprofiles (Data Source)

Reads all NodePing notification profiles in one account.

## Example Usage

```hcl
data "nodeping_notificationprofiles" "all" {}

output "profile_names" {
  value = { for p in data.nodeping_notificationprofiles.all.profiles : p.id => p.name }
}
```

### Find a profile by name in a subaccount

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

| Argument | Required | Description |
|----------|----------|-------------|
| `customer_id` | No | The ID of the account to list. Set `customer_id` to list the objects of a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `profiles` | List of Object | The notification profiles, sorted by profile ID. |
| `profiles[].id` | String | The ID of the notification profile. |
| `profiles[].customer_id` | String | The ID of the account that owns the profile. |
| `profiles[].name` | String | The name of the notification profile. |
| `profiles[].notifications` | List of Object | The notification targets. Each target has `contact_id`, `delay` and `schedule`, as in [`nodeping_notificationprofile`](nodeping_notificationprofile.md). |
