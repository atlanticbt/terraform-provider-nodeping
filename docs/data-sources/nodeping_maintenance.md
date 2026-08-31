---
page_title: "nodeping_maintenance Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches a NodePing maintenance window by ID.
---

# nodeping_maintenance (Data Source)

Fetches a NodePing maintenance window by ID.

## Ad-hoc windows disappear

Unlike the `nodeping_maintenance` resource, this data source will return an
ad-hoc window while one exists. **NodePing deletes an ad-hoc window as soon as
it finishes running**, so a configuration that reads one by ID starts failing at
plan time with a "not found" error the moment the window completes. Read
scheduled windows here; treat ad-hoc IDs as short-lived.

## Example Usage

```hcl
data "nodeping_maintenance" "patching" {
  id = "9VKOB"
}

output "next_patching_run" {
  value = data.nodeping_maintenance.patching.nextrun
}

output "checks_in_window" {
  value = data.nodeping_maintenance.patching.checklist
}
```

### SubAccount window

```hcl
data "nodeping_maintenance" "client_patching" {
  id          = "9VKOB"
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

- `id` - (Required) The unique identifier of the maintenance window.
- `customer_id` - (Optional) Customer ID (SubAccount ID) to look the window up
  in. Leave unset to use the provider-level account.

## Attribute Reference

- `name` - Friendly name for the maintenance window.
- `enabled` - Whether the schedule is active.
- `cron` - The 5-field cron expression. Empty for ad-hoc windows.
- `duration` - How long the checks stay disabled, in minutes.
- `checklist` - Check IDs disabled by this window, sorted by ID.
- `processing` - `true` while NodePing is currently running this window.
- `nextrun` - Unix timestamp in milliseconds of the next scheduled run.
- `last_edited_by` - Email address of the account that last edited the window.
- `last_edited_at` - Unix timestamp in milliseconds of the last edit.
- `history` - The ten most recent runs, oldest first. Each entry contains:
  - `timestamp` - Unix timestamp in milliseconds of the run.
  - `disabled_checks` - Check IDs disabled by that run, sorted by ID.
  - `skipped_checks` - Check IDs skipped by that run, typically because they
    were already disabled.

`history` is returned by the API as a map keyed by timestamp; it is flattened
into a list sorted oldest first so that repeated reads produce an identical
result.
