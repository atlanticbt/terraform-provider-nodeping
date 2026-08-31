---
page_title: "nodeping_schedules Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches all NodePing notification schedules.
---

# nodeping_schedules (Data Source)

Fetches all NodePing notification schedules. The list is ordered by schedule ID
so repeated reads produce the same result.

## Example Usage

```hcl
data "nodeping_schedules" "all" {}

output "schedule_ids" {
  value = [for s in data.nodeping_schedules.all.schedules : s.id]
}

# Which schedules leave Sunday completely open?
locals {
  sunday_always_on = [
    for s in data.nodeping_schedules.all.schedules : s.id
    if s.sunday != null && s.sunday.allday
  ]
}
```

### Listing a SubAccount's Schedules

```hcl
data "nodeping_schedules" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

- `customer_id` - (Optional) The customer ID (SubAccount ID) to list schedules
  for. Omit to use the account the provider is configured for.

## Attribute Reference

- `schedules` - All schedules, ordered by ID. Each contains:
  - `id` - The schedule name.
  - `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`, `sunday` -
    The notification window for that day, or `null` when the day is unscheduled.
    Each contains `time1`, `time2`, `exclude`, `disabled` and `allday`.

See [`nodeping_schedule`](nodeping_schedule.md) for the meaning of each day
field.
