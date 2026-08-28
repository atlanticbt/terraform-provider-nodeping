---
page_title: "nodeping_schedule Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing notification schedule.
---

# nodeping_schedule (Resource)

Manages a NodePing notification schedule. A schedule names a weekly notification
window; contact addresses reference it by name to limit when they page someone.

## The ID is yours, not NodePing's

The NodePing schedules endpoint exposes only `GET`, `PUT` and `DELETE` — there is
no `POST`. `PUT` performs both create and update, and the identifier travels in
the URL path, so **you choose the ID**. That makes `id` a required argument
rather than a computed one, and it is why the same API call backs both a first
apply and every later change.

Because the ID is the API's primary key, changing it forces replacement:
OpenTofu creates the schedule under the new name and destroys the old one.

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

Days you leave out are not sent to the API at all, so omitting `sunday` and
setting `sunday = { disabled = true }` are different things: the first leaves the
day unscheduled, the second explicitly silences it.

### Inverting a Span with `exclude`

`exclude` flips the meaning of `time1`/`time2`: notifications go out all day
*except* during the span. This is the natural way to express an out-of-hours
rota — the window you name is the quiet period.

```hcl
resource "nodeping_schedule" "after_hours" {
  id = "AfterHours"

  # Page at any hour except 09:00-17:00, when the day team is watching.
  monday    = { time1 = "9:00", time2 = "17:00", exclude = true }
  tuesday   = { time1 = "9:00", time2 = "17:00", exclude = true }
  wednesday = { time1 = "9:00", time2 = "17:00", exclude = true }
  thursday  = { time1 = "9:00", time2 = "17:00", exclude = true }
  friday    = { time1 = "9:00", time2 = "17:00", exclude = true }

  # Weekends belong to the on-call engineer end to end.
  saturday = { allday = true }
  sunday   = { allday = true }
}
```

### SubAccount Schedule

```hcl
resource "nodeping_schedule" "client_hours" {
  id          = "ClientHours"
  customer_id = "201205050153W2Q4C"

  monday = { time1 = "6:00", time2 = "22:00" }
}
```

## Argument Reference

- `id` - (Required) The schedule name. You choose this value; NodePing does not
  generate one. 1-100 characters, and it must not contain `/`, `?` or `#`.
  Changing it forces a new schedule.
- `customer_id` - (Optional) The customer ID (SubAccount ID) that owns the
  schedule. Omit to use the account the provider is configured for. Changing it
  forces a new schedule.
- `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`, `sunday` -
  (Optional) The notification window for that day. Omit a day to leave it
  unscheduled. Each takes:
  - `time1` - (Optional) Start of the span, 24-hour `H:MM` or `HH:MM`, for
    example `"6:00"` or `"18:30"`.
  - `time2` - (Optional) End of the span, same format.
  - `exclude` - (Optional, default `false`) Invert the span: notify all day
    except between `time1` and `time2`.
  - `disabled` - (Optional, default `false`) Disable notifications for the day.
  - `allday` - (Optional, default `false`) Enable notifications for the whole
    day.

The three booleans are `0`/`1` on the wire. The provider converts them, so
configuration and state use real booleans.

## Attribute Reference

No attributes beyond the arguments are exported. `customer_id` is filled in from
the API when it is not set in configuration.

## Import

Schedules are imported by ID:

```shell
terraform import nodeping_schedule.business_hours BusinessHours
```

For SubAccount schedules, use `customer_id:schedule_id`:

```shell
terraform import nodeping_schedule.client_hours 201205050153W2Q4C:ClientHours
```

The imported `customer_id` is written to state and used for every subsequent API
call for this resource. Set `customer_id` in your configuration to match,
otherwise the next plan shows a replacement.
