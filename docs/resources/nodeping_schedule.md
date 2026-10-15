---
page_title: "nodeping_schedule Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing notification schedule.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_schedule (Resource)

Manages a NodePing notification schedule.
A schedule gives a name to a weekly notification window.
Contact addresses and notifications use the name to limit when NodePing sends alerts.

## You Choose the ID

The NodePing schedules endpoint has `GET`, `PUT` and `DELETE`, but no `POST`.
`PUT` creates and updates a schedule.
The identifier is in the URL path, so **you choose the ID**.
Thus `id` is a required argument, not a computed attribute.

The ID is the primary key in the API.
If you change it, OpenTofu replaces the schedule.
It creates the schedule with the new name, then deletes the schedule with the old name.

## Example Usage

### Business Hours

```hcl
resource "nodeping_schedule" "business_hours" {
  id = "BusinessHours"

  monday    = { time1 = "8:00", time2 = "18:00" }
  tuesday   = { time1 = "8:00", time2 = "18:00" }
  wednesday = { time1 = "8:00", time2 = "18:00" }
  thursday  = { time1 = "8:00", time2 = "18:00" }
  friday    = { time1 = "8:00", time2 = "17:00" }

  saturday = { disabled = true }
  sunday   = { disabled = true }
}
```

The provider does not send a day that you do not set.
Thus these two settings are different:

| Configuration | Result |
|---|---|
| No `sunday` attribute | Sunday is not scheduled. |
| `sunday = { disabled = true }` | Sunday is scheduled, and notifications are off for that day. |

### Notify Outside a Time Span With `exclude`

`exclude` reverses the span from `time1` to `time2`.
NodePing then sends notifications all day, but not during the span.
Use it for an out-of-hours rota: the span is the quiet period.

```hcl
resource "nodeping_schedule" "after_hours" {
  id = "AfterHours"

  # Notify at all hours except 09:00 to 17:00, when the day team monitors.
  monday    = { time1 = "9:00", time2 = "17:00", exclude = true }
  tuesday   = { time1 = "9:00", time2 = "17:00", exclude = true }
  wednesday = { time1 = "9:00", time2 = "17:00", exclude = true }
  thursday  = { time1 = "9:00", time2 = "17:00", exclude = true }
  friday    = { time1 = "9:00", time2 = "17:00", exclude = true }

  # The on-call engineer has all of Saturday and Sunday.
  saturday = { allday = true }
  sunday   = { allday = true }
}
```

### A Schedule in a Subaccount

```hcl
resource "nodeping_schedule" "client_hours" {
  id          = "ClientHours"
  customer_id = "201205050153W2Q4C"

  monday = { time1 = "6:00", time2 = "22:00" }
}
```

## Argument Reference

- `id` - (Required) The schedule name. You choose this value. NodePing does not make one. 1 to 100 characters. It must not contain `/`, `?` or `#`, and it must not start with a space. If you change it, OpenTofu replaces the schedule.
- `customer_id` - (Optional) The subaccount ID that owns the schedule. If you do not set it, the provider uses the account of the provider. If you change it, OpenTofu replaces the schedule.
- `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`, `sunday` - (Optional) The notification window for that day. If you do not set a day, it is not scheduled. Each day has these attributes:
  - `time1` - (Optional) The start of the span, in 24-hour `H:MM` or `HH:MM` format. Examples: `"6:00"`, `"18:30"`.
  - `time2` - (Optional) The end of the span, in the same format.
  - `exclude` - (Optional) Reverse the span: notify all day except from `time1` to `time2`. The default is `false`.
  - `disabled` - (Optional) Stop all notifications on this day. The default is `false`.
  - `allday` - (Optional) Send notifications for all of the day. The default is `false`.

NodePing stores the three boolean values as `0` and `1`.
The provider changes them, so configuration and state use true booleans.
A day that NodePing stores with no hours has no `time1` and no `time2` in state.

## Attribute Reference

There are no attributes other than the arguments.
When the configuration does not set `customer_id`, the provider writes the value from the API.

## Import

Import a schedule with its ID:

```shell
tofu import nodeping_schedule.business_hours BusinessHours
```

For a schedule in a subaccount, use the format `customer_id:schedule_id`:

```shell
tofu import nodeping_schedule.client_hours 201205050153W2Q4C:ClientHours
```

The import writes `customer_id` to state.
The provider uses it for all later API calls for this resource.
Set the same `customer_id` in your configuration.
If you do not, the next plan replaces the schedule.

You can also use an `import` block with an `identity`: set `id`, and set `customer_id` for a subaccount.
