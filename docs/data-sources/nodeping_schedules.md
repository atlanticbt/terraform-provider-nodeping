---
page_title: "nodeping_schedules Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing notification schedules, sorted by schedule ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_schedules (Data Source)

Reads all NodePing notification schedules in one account. The list is sorted by schedule ID, so each read gives the same order.

## Example Usage

```hcl
data "nodeping_schedules" "all" {}

output "schedule_ids" {
  value = [for s in data.nodeping_schedules.all.schedules : s.id]
}

# The schedules that notify all day on Sunday.
locals {
  sunday_always_on = [
    for s in data.nodeping_schedules.all.schedules : s.id
    if s.sunday != null && s.sunday.allday
  ]
}
```

### List the schedules of a subaccount

```hcl
data "nodeping_schedules" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

| Argument | Required | Description |
|----------|----------|-------------|
| `customer_id` | No | The ID of the account to list. Set `customer_id` to list the objects of a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `schedules` | List of Object | All schedules, sorted by ID. |
| `schedules[].id` | String | The schedule name. |
| `schedules[].monday`, `schedules[].tuesday`, `schedules[].wednesday`, `schedules[].thursday`, `schedules[].friday`, `schedules[].saturday`, `schedules[].sunday` | Object | The notification period for that day. It is `null` when the day has no schedule. Each day has the fields below. |
| `schedules[].<day>.time1` | String | The start of the notification period, in 24-hour `H:MM` or `HH:MM` format. It is empty on a day off. |
| `schedules[].<day>.time2` | String | The end of the notification period. It is empty on a day off. |
| `schedules[].<day>.exclude` | Bool | `true` when the period is inverted: NodePing notifies all day, except between `time1` and `time2`. |
| `schedules[].<day>.disabled` | Bool | `true` when notifications are off for the day. |
| `schedules[].<day>.allday` | Bool | `true` when notifications are on for the whole day. |
