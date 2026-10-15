---
page_title: "nodeping_check Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing check by ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_check (Data Source)

Reads one NodePing check by ID. The data source gives the general settings and the current state of the check.
To manage a check, use the [`nodeping_check`](../resources/nodeping_check.md) resource.

If NodePing cannot find the check, the read fails with an error.

## Example Usage

```hcl
data "nodeping_check" "example" {
  id = "201205050153W2Q4C-0J2HSIRF"
}

output "check_label" {
  value = data.nodeping_check.example.label
}

output "check_state" {
  description = "0 = failing, 1 = passing"
  value       = data.nodeping_check.example.state
}
```

### Read a Check in a Subaccount

```hcl
data "nodeping_check" "client" {
  id          = "201205050153W2Q4C-0J2HSIRF"
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Required

- `id` (String) The check ID.

### Optional

- `customer_id` (String) The customer ID of the subaccount that owns the check. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. After the read, it holds the customer ID that NodePing returns.

### Read-Only

- `type` (String) The check type, for example `HTTP` or `SSL`.
- `target` (String) The URL, host name, or IP address that the check tests. The value is the form that NodePing stores. For URL targets, this can be a canonical form of the configured value.
- `label` (String) The label of the check.
- `description` (String) The description of the check.
- `enabled` (Boolean) `true` when the check is enabled.
- `public` (Boolean) `true` when public reports are enabled.
- `interval` (Number) The time between two runs, in minutes.
- `threshold` (Number) The timeout, in seconds.
- `sens` (Number) The number of rechecks before the status changes.
- `state` (Number) The current state: `0` means failing, `1` means passing.
- `tags` (List of String) All tags that NodePing keeps on the check. This includes the provider `default_tags`, so it matches `tags_all` on the resource.
- `dep` (String) The ID of the parent check. Refer to [notification dependency](../notification_dependency.md).
- `created` (Number) The creation time, in milliseconds since the Unix epoch.
- `modified` (Number) The time of the last change, in milliseconds since the Unix epoch.

## Notes

- The data source does not return credentials, notifications, or type-specific settings.
