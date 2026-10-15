---
page_title: "nodeping_maintenance Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a scheduled NodePing maintenance window.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_maintenance (Resource)

Manages a scheduled NodePing maintenance window.
A maintenance disables the checks in `checklist` at the time that `cron` sets.
The checks stay disabled for `duration` minutes, then NodePing enables them again.
Use a maintenance to stop alerts during planned deployments, patches, or backups.

This resource manages scheduled maintenance only. It does not manage ad-hoc maintenance.
To start an ad-hoc maintenance from OpenTofu, see [Ad-hoc maintenance](#ad-hoc-maintenance).

## Example Usage

```hcl
resource "nodeping_maintenance" "weekly_patching" {
  name     = "Weekly patching window"
  cron     = "0 3 * * 0" # 03:00 every Sunday
  duration = 60          # minutes
  enabled  = true

  checklist = [
    nodeping_check.web.id,
    nodeping_check.api.id,
  ]
}
```

### Disable a schedule for a period

Set `enabled = false` to keep the definition and stop the schedule.
The provider always sends `enabled` to the API.
Thus, when you set it to `true` again, the next apply starts the schedule again.

```hcl
resource "nodeping_maintenance" "quarterly_dr_test" {
  name      = "Quarterly DR test"
  cron      = "0 2 1 */3 *"
  duration  = 240
  enabled   = false
  checklist = [nodeping_check.web.id]
}
```

### Maintenance in a subaccount

```hcl
resource "nodeping_maintenance" "client_patching" {
  customer_id = "201205050153W2Q4C"
  name        = "Client patching window"
  cron        = "15 1 * * 6"
  duration    = 45
  checklist   = [nodeping_check.client_web.id]
}
```

## Schema

### Required

- `name` (String) The name of the maintenance. Length 1 to 255 characters.
- `cron` (String) A standard 5-field cron expression that sets the start time, in the timezone of the account. See [Cron syntax](#cron-syntax). The provider validates it at plan time.
- `duration` (Number) The time that the checks stay disabled, in minutes. Range 1 to 527040 (about one year). A larger value is almost always a units mistake, and it disables monitoring for too long.
- `checklist` (Set of String) The IDs of the checks to disable. The order is not significant. 1 to 5000 entries. Each entry must not be empty.

### Optional

- `enabled` (Boolean) If `true`, the schedule is active. A disabled maintenance does not run. Default `true`.
- `customer_id` (String) The customer ID of the subaccount that owns the maintenance. Set it to manage a maintenance in a subaccount. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. Changing it forces replacement.

### Read-Only

- `id` (String) The ID that NodePing assigns, for example `9VKOB`.
- `processing` (Boolean) `true` while NodePing runs the maintenance.
- `nextrun` (Number) The time of the next scheduled run, in milliseconds since the Unix epoch.

This resource does not show the `history` field of the API (the ten most recent runs).
The value changes at each run.
On a resource, that change rewrites state and adds noise to each plan.
To read the history, use the [`nodeping_maintenance`](../data-sources/nodeping_maintenance.md) data source.

### Cron syntax

The expression must have exactly 5 fields, separated by whitespace:

| Position | Field | Values |
|---|---|---|
| 1 | minute | `0`-`59` |
| 2 | hour | `0`-`23` |
| 3 | day of month | `1`-`31`, or `?` |
| 4 | month | `1`-`12`, or `JAN`-`DEC` |
| 5 | day of week | `0`-`7` (`0` and `7` are Sunday), `SUN`-`SAT`, or `?` |

Each field accepts these forms:

- `*` for all values.
- A single value, for example `5` or `MON`. Names are not case-sensitive.
- A range, for example `8-17`. The start must not be greater than the end.
- A list, for example `0,30`.
- A step after a base value, for example `*/15` or `0-30/10`. The step must be from 1 to the maximum value of the field.

The provider rejects a 6-field expression (with seconds), non-standard macros such as `@daily`, and control characters.

## Import

Import a maintenance in the account of the API token by its ID:

```shell
tofu import nodeping_maintenance.weekly_patching 9VKOB
```

To import a maintenance in a subaccount, use `customer_id:maintenance_id`:

```shell
tofu import nodeping_maintenance.weekly_patching 201205050153W2Q4C:9VKOB
```

With OpenTofu 1.12 or later, you can also use an `import` block with an `identity`.
The identity has two attributes: `id` (required) and `customer_id` (optional).
Do not set `customer_id` for a maintenance in the account of the API token.

```hcl
import {
  to = nodeping_maintenance.weekly_patching
  identity = {
    id          = "9VKOB"
    customer_id = "201205050153W2Q4C"
  }
}
```

The import writes `customer_id` to state.
If you do not supply it, the provider uses the value that the API returns.
All later API calls for the resource use this value.

> **CAUTION:** Set `customer_id` in the configuration to the imported value. If you omit it or set a different value, a later plan can replace the maintenance.

The import fails for an ad-hoc maintenance.
It also fails for a maintenance that has no `cron` expression.
The error message tells you the cause.

## Ad-hoc maintenance

NodePing also accepts ad-hoc maintenance.
A request with `id = "ad-hoc"` starts a maintenance immediately.
**NodePing deletes the record when the maintenance ends.**

This behavior does not agree with declarative state.
When the record deletes itself, the next refresh removes it from state.
Then the next apply creates it again, and that starts a new maintenance.
NodePing deletes that record too, and the loop continues.
Each cycle disables the monitoring of the account, and nobody asked for it.

Thus this resource has these limits:

- It requires `cron`, so each maintenance that it manages is a scheduled one.
- It never sends `id = "ad-hoc"`. If the API returns an ad-hoc record on create, the create fails.
- It does not update a maintenance with the ID `ad-hoc`.
- It does not import an ad-hoc maintenance, or a maintenance without a `cron` expression.

An ad-hoc maintenance is an imperative operation: "disable these checks now".
Start it from the NodePing UI, an API call, or a script.
The [`nodeping_maintenance`](../data-sources/nodeping_maintenance.md) and [`nodeping_maintenances`](../data-sources/nodeping_maintenances.md) data sources show an ad-hoc maintenance while it exists.

### Start an ad-hoc maintenance from OpenTofu

You can start an ad-hoc maintenance as part of an apply, for example around a deploy.
Use a `terraform_data` resource with a `local-exec` provisioner.
The maintenance does not go into provider state, so the loop above cannot occur.

The script `scripts/nodeping-adhoc-maintenance.sh` in this repository sends the API request.
It validates its input before it calls the API:

| Input | Rule |
|---|---|
| `--duration` (required) | An integer from 1 to 527040 minutes. |
| `--checks` (required) | Comma-separated check IDs. Letters, digits and hyphens only. 1 to 5000 IDs. |
| `--name` (optional) | Letters, digits and `_ . : -` only. 255 characters or fewer. The default is `adhoc-<UTC timestamp>`. |
| `NODEPING_API_TOKEN` (required) | The API token. |
| `NODEPING_CUSTOMER_ID` (optional) | The customer ID of a subaccount. Letters, digits and hyphens only. |
| `NODEPING_API_URL` (optional) | The API base URL. It must use `https`, or `http` to `localhost` or `127.0.0.1` for tests. |

The script fails if the API returns an error in the response body or a non-2xx HTTP status.

To use the recipe, do these steps:

1. Copy the script into your module, for example to `scripts/` below the module root.
2. Add a `terraform_data` resource like the example below.
3. Set `triggers_replace` to a value that changes once for each deploy.
4. Supply the API token through the `environment` block.

> **SECURITY:** Put the API token in `environment`, not in `command`. Any value in the command string shows in the process list (`ps`) of the machine that runs the apply.

```hcl
resource "terraform_data" "deploy_window" {
  # Each new value starts exactly one new maintenance. Use a value that
  # changes once per deploy, such as an image tag or a release ID. Do not
  # use timestamp(), because it changes at each plan.
  triggers_replace = [var.release_version]

  provisioner "local-exec" {
    command = "${path.module}/scripts/nodeping-adhoc-maintenance.sh --duration 15 --checks ${join(",", local.deploy_check_ids)}"

    # The token goes in the environment, never in `command`.
    environment = {
      NODEPING_API_TOKEN   = var.nodeping_api_token
      NODEPING_CUSTOMER_ID = var.subaccount_id
    }
  }
}
```

> **SECURITY:** OpenTofu stores `triggers_replace` in state. Do not put a secret in it.

Read these limits before you use the recipe:

- **Use provisioners only as a last resort.** This case qualifies, because no declarative form exists for a "fire and forget" action. OpenTofu has no equivalent of the Terraform `action` block, and it can choose not to add one ([opentofu#3309](https://github.com/opentofu/opentofu/issues/3309)).
- **OpenTofu does not track the maintenance.** The plan does not show it, and OpenTofu does not end it early. Use the data sources to see it.
- **The script runs on the machine that runs the apply.** That machine needs `bash` and `curl`.
- **A script failure stops the apply.** If the maintenance must not block a deploy, add `on_failure = continue` to the provisioner.

## Notes

### Drift and deletion outside OpenTofu

If a person deletes a scheduled maintenance in the NodePing UI, the next refresh removes it from state.
The next apply then creates it again.
This is correct for a scheduled maintenance, because only a person deletes it.
An ad-hoc maintenance deletes itself, and that is why this resource does not manage ad-hoc maintenance.

If the maintenance is deleted between the refresh and an update, the update fails.
Run `tofu refresh`, or remove the resource with `tofu state rm`, then apply again.
