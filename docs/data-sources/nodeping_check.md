---
page_title: "nodeping_check Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Fetches a NodePing check by ID.
---

# nodeping_check (Data Source)

Fetches a NodePing check by ID.

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

output "check_enabled" {
  value = data.nodeping_check.example.enabled
}
```

## Argument Reference

- `id` - (Required) The unique identifier of the check.

## Attribute Reference

- `customer_id` - The customer ID (account ID) that owns this check.
- `type` - The type of check.
- `target` - The target URL, hostname, or IP address.
- `label` - The display label for the check.
- `enabled` - Whether the check is enabled.
- `public` - Whether public reports are enabled.
- `interval` - Check interval in minutes.
- `threshold` - Timeout in seconds.
- `sens` - Sensitivity (rechecks before status change).
- `state` - Current state: `0` (failing) or `1` (passing).
- `created` - Creation timestamp (milliseconds).
- `modified` - Last modification timestamp (milliseconds).
- `description` - Description of the check.
- `tags` - List of tags.
