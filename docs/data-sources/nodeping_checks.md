---
page_title: "nodeping_checks Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches all NodePing checks with optional filtering.
---

# nodeping_checks (Data Source)

Fetches all NodePing checks with optional filtering.

## Example Usage

### Fetch All Checks

```hcl
data "nodeping_checks" "all" {}

output "all_check_ids" {
  value = [for c in data.nodeping_checks.all.checks : c.id]
}
```

### Filter by Type

```hcl
data "nodeping_checks" "http_only" {
  type = "HTTP"
}

output "http_check_labels" {
  value = [for c in data.nodeping_checks.http_only.checks : c.label]
}
```

### Find Failing Checks

```hcl
data "nodeping_checks" "all" {}

output "failing_checks" {
  value = [for c in data.nodeping_checks.all.checks : c.label if c.state == 0]
}
```

### Find Disabled Checks

```hcl
data "nodeping_checks" "all" {}

output "disabled_checks" {
  value = [for c in data.nodeping_checks.all.checks : c.label if !c.enabled]
}
```

## Argument Reference

- `type` - (Optional) Filter checks by type (e.g., `HTTP`, `DNS`, `SSL`).

## Attribute Reference

- `checks` - List of checks matching the filter. Each check contains:
  - `id` - The unique identifier of the check.
  - `customer_id` - The customer ID (account ID) that owns this check.
  - `type` - The type of check.
  - `target` - The target URL, hostname, or IP address.
  - `label` - The display label for the check.
  - `enabled` - Whether the check is enabled.
  - `interval` - Check interval in minutes.
  - `state` - Current state: `0` (failing) or `1` (passing).
  - `tags` - List of tags.
