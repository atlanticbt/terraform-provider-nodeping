---
page_title: "nodeping_checks Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing checks, with optional filters.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_checks (Data Source)

Reads all NodePing checks in one account. Use the filters to make the list smaller.
A check must match all filters that you set.
The list is sorted by check ID.

## Example Usage

### Read All Checks

```hcl
data "nodeping_checks" "all" {}

output "all_check_ids" {
  value = [for c in data.nodeping_checks.all.checks : c.id]
}
```

### Filter by Type, Label or Tags

```hcl
data "nodeping_checks" "http_only" {
  type = "HTTP"
}

data "nodeping_checks" "homepage" {
  label = "Example - www.example.com - Homepage"
}

data "nodeping_checks" "production" {
  tags = ["production", "web"]
}
```

### Find Failing or Disabled Checks

```hcl
data "nodeping_checks" "all" {}

output "failing_checks" {
  value = [for c in data.nodeping_checks.all.checks : c.label if c.state == 0]
}

output "disabled_checks" {
  value = [for c in data.nodeping_checks.all.checks : c.label if !c.enabled]
}
```

### List the Checks of a Subaccount

```hcl
data "nodeping_checks" "client" {
  customer_id = "201205050153W2Q4C"
}
```

## Schema

### Optional

- `type` (String) Return only checks of this type. Must be one of the check types in [Supported Check Types](../resources/nodeping_check.md#supported-check-types), in upper case, for example `HTTP`.
- `label` (String) Return only checks with this exact label. The match is case-sensitive.
- `tags` (List of String) Return only checks that have all of these tags. The filter compares against all tags on the check, including the provider `default_tags`. An empty list matches all checks.
- `customer_id` (String) The customer ID of the subaccount to list. If you do not set it, the provider uses its own `customer_id`, or the account of the API token.

### Read-Only

- `checks` (Attributes List) The checks that match the filters, sorted by check ID. Refer to [Nested Schema for `checks`](#nested-schema-for-checks).

### Nested Schema for `checks`

- `id` (String) The check ID.
- `customer_id` (String) The customer ID of the account that owns the check.
- `type` (String) The check type.
- `target` (String) The URL, host name, or IP address that the check tests, in the form that NodePing stores.
- `label` (String) The label of the check.
- `enabled` (Boolean) `true` when the check is enabled.
- `interval` (Number) The time between two runs, in minutes.
- `state` (Number) The current state: `0` means failing, `1` means passing.
- `tags` (List of String) All tags that NodePing keeps on the check, including the provider `default_tags`.
- `dep` (String) The ID of the parent check. Refer to [notification dependency](../notification_dependency.md).

## Notes

- The list has summary attributes only. For `description`, `threshold`, `sens`, `public` and the timestamps, use the [`nodeping_check`](nodeping_check.md) data source.
- The data source reads the full check list from NodePing in one request, then applies the filters in the provider.
