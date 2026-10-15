---
page_title: "nodeping_schedule Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing notification schedule by ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_schedule (Data Source)

Reads one NodePing notification schedule by ID. A notification schedule sets the hours when NodePing sends notifications.

The person who creates a schedule chooses its ID. NodePing does not make the ID. Thus the ID is the schedule name, for example `BusinessHours`.

## Example Usage

```hcl
data "nodeping_schedule" "business_hours" {
  id = "BusinessHours"
}

output "monday_window" {
  value = "${data.nodeping_schedule.business_hours.monday.time1}-${data.nodeping_schedule.business_hours.monday.time2}"
}

# A day without a schedule is null.
output "saturday_is_unscheduled" {
  value = data.nodeping_schedule.business_hours.saturday == null
}
```

### Read a schedule in a subaccount

```hcl
data "nodeping_schedule" "client_hours" {
  id          = "ClientHours"
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

| Argument | Required | Description |
|----------|----------|-------------|
| `id` | Yes | The schedule name. |
| `customer_id` | No | The ID of the account that owns the schedule. Set `customer_id` to read an object that belongs to a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `customer_id` | String | The ID of the account that owns the schedule, when the API gives it. |
| `monday` to `sunday` | Object | The notification period for that day. It is `null` when the day has no schedule. Each day has the fields below. |
| `<day>.time1` | String | The start of the notification period, in 24-hour `H:MM` or `HH:MM` format. It is empty on a day off. |
| `<day>.time2` | String | The end of the notification period. It is empty on a day off. |
| `<day>.exclude` | Bool | `true` when the period is inverted: NodePing notifies all day, except between `time1` and `time2`. |
| `<day>.disabled` | Bool | `true` when notifications are off for the day. |
| `<day>.allday` | Bool | `true` when notifications are on for the whole day. |

## Notes

- If the schedule does not exist, the read fails. It does not return an empty result.
