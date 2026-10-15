---
page_title: "nodeping_maintenance Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing maintenance window by ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_maintenance (Data Source)

Reads one NodePing maintenance window by ID. A maintenance window disables a list of checks for a period.

The [`nodeping_maintenance`](../resources/nodeping_maintenance.md) resource manages only scheduled windows. This data source also reads an ad-hoc window while the window exists.

> **CAUTION:** NodePing deletes an ad-hoc maintenance window when the window ends. If your configuration reads an ad-hoc window by ID, the next plan fails with a "not found" error. Read scheduled windows with this data source. Do not keep ad-hoc IDs in configuration.

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

### Read a window in a subaccount

```hcl
data "nodeping_maintenance" "client_patching" {
  id          = "9VKOB"
  customer_id = "201205050153W2Q4C"
}
```

## Argument Reference

| Argument | Required | Description |
|----------|----------|-------------|
| `id` | Yes | The ID of the maintenance window. |
| `customer_id` | No | The ID of the account that owns the window. Set `customer_id` to read an object that belongs to a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. |

## Attribute Reference

| Attribute | Type | Description |
|-----------|------|-------------|
| `name` | String | The name of the window. |
| `enabled` | Bool | `true` when the schedule is active. |
| `cron` | String | The 5-field cron expression that sets the start time. It is empty for an ad-hoc window. |
| `duration` | Number | The time that the checks stay disabled, in minutes. |
| `checklist` | List of String | The IDs of the checks that the window disables, sorted by ID. |
| `processing` | Bool | `true` while NodePing runs the window. |
| `nextrun` | Number | The time of the next run, in milliseconds since the Unix epoch. |
| `last_edited_by` | String | The email address of the account that made the last change. |
| `last_edited_at` | Number | The time of the last change, in milliseconds since the Unix epoch. |
| `history` | List of Object | The ten most recent runs, oldest first. |
| `history[].timestamp` | Number | The time of the run, in milliseconds since the Unix epoch. |
| `history[].disabled_checks` | List of String | The IDs of the checks that the run disabled. |
| `history[].skipped_checks` | List of String | The IDs of the checks that the run did not disable, usually because they were disabled already. |

## Notes

- The API returns `history` as a map keyed by time. The provider changes it to a list, oldest first, so each read gives the same result.
