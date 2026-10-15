---
page_title: "nodeping_maintenances Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing maintenance windows, sorted by ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_maintenances (Data Source)

Reads all NodePing maintenance windows in one account.

> **CAUTION:** The list includes each ad-hoc window that exists at the time of the read. NodePing deletes an ad-hoc window when it ends. Thus the number of entries and their IDs can change between reads. Filter on `ad_hoc` to keep only the windows that stay.

## Example Usage

```hcl
data "nodeping_maintenances" "all" {}

output "scheduled_window_ids" {
  value = [for m in data.nodeping_maintenances.all.maintenances : m.id if !m.ad_hoc]
}

# The windows that disable checks now.
output "running_now" {
  value = [
    for m in data.nodeping_maintenances.all.maintenances : m.name
    if m.processing
  ]
}

# Each check that an enabled window includes.
locals {
  covered_check_ids = distinct(flatten([
    for m in data.nodeping_maintenances.all.maintenances : m.checklist
    if m.enabled
  ]))
}
```

### List the windows of a subaccount

```hcl
data "nodeping_maintenances" "client" {
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
| `maintenances` | List of Object | The maintenance windows, sorted by ID. |
| `maintenances[].id` | String | The ID of the window. |
| `maintenances[].customer_id` | String | The ID of the account that owns the window. |
| `maintenances[].name` | String | The name of the window. |
| `maintenances[].ad_hoc` | Bool | `true` for an ad-hoc window. The `nodeping_maintenance` resource cannot manage ad-hoc windows. |
| `maintenances[].enabled` | Bool | `true` when the schedule is active. |
| `maintenances[].cron` | String | The 5-field cron expression. It is empty for an ad-hoc window. |
| `maintenances[].duration` | Number | The time that the checks stay disabled, in minutes. |
| `maintenances[].checklist` | List of String | The IDs of the checks that the window disables, sorted by ID. |
| `maintenances[].processing` | Bool | `true` while NodePing runs the window. |
| `maintenances[].nextrun` | Number | The time of the next run, in milliseconds since the Unix epoch. |
| `maintenances[].last_edited_by` | String | The email address of the account that made the last change. |
| `maintenances[].last_edited_at` | Number | The time of the last change, in milliseconds since the Unix epoch. |

## Notes

- The list does not include the run history. To read it, use the [`nodeping_maintenance`](nodeping_maintenance.md) data source.
