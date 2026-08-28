---
page_title: "nodeping_maintenances Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches all NodePing maintenance windows.
---

# nodeping_maintenances (Data Source)

Fetches all NodePing maintenance windows, sorted by ID.

## Ad-hoc windows come and go

The list includes any ad-hoc window that happens to exist at the moment of the
read. **NodePing deletes an ad-hoc window as soon as it finishes running**, so
the number of entries returned is not stable over time and neither are their
IDs. Filter on `ad_hoc` when you need only the windows that persist.

## Example Usage

```hcl
data "nodeping_maintenances" "all" {}

output "scheduled_window_ids" {
  value = [for m in data.nodeping_maintenances.all.maintenances : m.id if !m.ad_hoc]
}

# Windows currently disabling checks
output "running_now" {
  value = [
    for m in data.nodeping_maintenances.all.maintenances : m.name
    if m.processing
  ]
}

# Every check covered by at least one enabled window
locals {
  covered_check_ids = distinct(flatten([
    for m in data.nodeping_maintenances.all.maintenances : m.checklist
    if m.enabled
  ]))
}
```

### SubAccount windows

```hcl
data "nodeping_maintenances" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

- `customer_id` - (Optional) Customer ID (SubAccount ID) to list windows for.
  Leave unset to use the provider-level account.

## Attribute Reference

- `maintenances` - Maintenance windows, sorted by ID. Each entry contains:
  - `id` - The unique identifier of the window.
  - `customer_id` - The customer ID that owns the window.
  - `name` - Friendly name for the window.
  - `enabled` - Whether the schedule is active.
  - `cron` - The 5-field cron expression. Empty for ad-hoc windows.
  - `duration` - How long the checks stay disabled, in minutes.
  - `checklist` - Check IDs disabled by the window, sorted by ID.
  - `processing` - `true` while NodePing is currently running the window.
  - `nextrun` - Unix timestamp in milliseconds of the next scheduled run.
  - `ad_hoc` - `true` when the entry is an ad-hoc window. Ad-hoc windows are
    deleted by NodePing once they finish and cannot be managed by the
    `nodeping_maintenance` resource.
  - `last_edited_by` - Email address of the account that last edited the window.
  - `last_edited_at` - Unix timestamp in milliseconds of the last edit.

Per-run `history` is not included in the list; read it through the singular
`nodeping_maintenance` data source when you need it.
