---
page_title: "nodeping_maintenance Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a scheduled NodePing maintenance window.
---

# nodeping_maintenance (Resource)

Manages a scheduled NodePing maintenance window.

A maintenance window disables the checks listed in `checklist` on the schedule
given by `cron`, leaves them disabled for `duration` minutes, then re-enables
them. Use it to stop alerting during planned deployments, patching, or backups.

## Ad-hoc maintenance is not supported, on purpose

NodePing's API also accepts *ad-hoc* maintenance: post `id = "ad-hoc"` and the
window starts immediately. **NodePing deletes the record as soon as the window
finishes running.**

That behaviour is incompatible with declarative state. A resource whose object
deletes itself looks like drift on the next refresh, so OpenTofu removes it from
state and the next apply recreates it — which opens another maintenance window,
which is deleted again. The result is a loop that repeatedly disables the
account's monitoring, on every plan/apply cycle, without anyone asking for it.

This resource therefore:

- requires `cron`, so every window it manages is a scheduled one;
- never sends `id = "ad-hoc"` on create, and errors out if the API returns an
  ad-hoc record;
- refuses to import an ad-hoc window, or any window with no `cron` expression.

Ad-hoc maintenance is an imperative operation — "silence these checks, now" —
and belongs in the NodePing UI, a `curl` call, or a runbook script, not in
state. The `nodeping_maintenance` and `nodeping_maintenances` data sources will
still report an ad-hoc window for as long as one exists.

### Running one from OpenTofu anyway

If you need to open an ad-hoc window as part of an apply — silencing checks
around a deploy, say — drive it imperatively with `terraform_data` and a
provisioner. This deliberately keeps the window out of provider state, so the
recreate loop described above cannot occur.

`scripts/nodeping-adhoc-maintenance.sh` in this repository does the API call
with input validation and error handling:

```hcl
resource "terraform_data" "deploy_window" {
  # Each new value opens exactly one new window. Use something that changes
  # once per deploy -- an image tag, a release id -- not `timestamp()`, which
  # would fire on every plan.
  triggers_replace = [var.release_version]

  provisioner "local-exec" {
    command = "${path.module}/scripts/nodeping-adhoc-maintenance.sh --duration 15 --checks ${join(",", local.deploy_check_ids)}"

    # The token goes in the environment, never in `command`. Anything
    # interpolated into the command string is visible in `ps` output on the
    # machine running the apply.
    environment = {
      NODEPING_API_TOKEN   = var.nodeping_api_token
      NODEPING_CUSTOMER_ID = var.subaccount_id
    }
  }
}
```

Caveats worth understanding before using this:

- **Provisioners are a last resort**, and this is one of the few cases that
  genuinely qualifies: there is no declarative way to express a fire-and-forget
  action. OpenTofu has no equivalent of Terraform's `action` blocks and has
  signalled it may diverge rather than adopt them
  ([opentofu#3309](https://github.com/opentofu/opentofu/issues/3309)), so this
  workaround is not obviously temporary.
- **Nothing is tracked.** OpenTofu does not know the window exists, cannot show
  it in a plan, and will not close it early. Use the data sources to observe it.
- **`triggers_replace` is stored in state.** Never interpolate a secret into it.
  The `environment` block is not persisted; the `command` string is not
  persisted either, but it *is* visible in the process table while running.
- **It runs on the machine doing the apply**, which needs `bash` and `curl`.
- **Failure fails the apply.** Add `on_failure = continue` if opening a
  maintenance window should not block a deploy.

Replace this with a first-class resource if and when OpenTofu ships an actions
equivalent.

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

### Temporarily disabling a schedule

Set `enabled = false` to keep the definition but stop it from firing. The value
is always sent to the API, so re-enabling it later takes effect on the next
apply.

```hcl
resource "nodeping_maintenance" "quarterly_dr_test" {
  name      = "Quarterly DR test"
  cron      = "0 2 1 */3 *"
  duration  = 240
  enabled   = false
  checklist = [nodeping_check.web.id]
}
```

### SubAccount window

```hcl
resource "nodeping_maintenance" "client_patching" {
  customer_id = "201205050153W2Q4C"
  name        = "Client patching window"
  cron        = "15 1 * * 6"
  duration    = 45
  checklist   = [nodeping_check.client_web.id]
}
```

## Argument Reference

- `name` - (Required) Friendly name for the maintenance window. 1-255 characters.
- `cron` - (Required) Standard 5-field cron expression, `minute hour day-of-month
  month day-of-week`, giving the start time of the window in the account's
  timezone. Ranges (`8-17`), lists (`0,30`), steps (`*/15`), `?` in the two day
  fields, and three-letter month/day names (`JAN`, `MON`) are accepted.
  Six-field (seconds) and non-standard expressions are rejected at plan time.
- `duration` - (Required) How long the checks stay disabled, in minutes. Minimum
  1, maximum 527040 (roughly one year). The upper bound exists because a value
  that large is nearly always a units mistake that would disable monitoring
  indefinitely.
- `checklist` - (Required) Set of check IDs to disable for the duration of the
  window. Order is not significant. At least one entry.
- `enabled` - (Optional) Whether the schedule is active. Defaults to `true`.
- `customer_id` - (Optional) Customer ID (SubAccount ID) that owns the window.
  Set this to manage a window belonging to a SubAccount. Changing it forces a
  new window to be created.

## Attribute Reference

- `id` - The unique identifier assigned by NodePing (a short string such as
  `9VKOB`).
- `processing` - `true` while NodePing is currently running this window.
- `nextrun` - Unix timestamp in milliseconds of the next scheduled run.

The API's `history` field — the ten most recent runs — is deliberately **not**
exposed on this resource. It changes every time the window runs, so a computed
attribute tracking it would rewrite state and show "known after apply" churn on
every plan while carrying no information the practitioner can act on. Read it
through the `nodeping_maintenance` data source instead, where a changing value
costs nothing.

## Import

Maintenance windows are imported by ID:

```shell
terraform import nodeping_maintenance.weekly_patching 9VKOB
```

For SubAccount windows, use `customer_id:maintenance_id`:

```shell
terraform import nodeping_maintenance.weekly_patching 201205050153W2Q4C:9VKOB
```

The imported `customer_id` is written to state and used for every subsequent API
call for this resource. Set `customer_id` in your configuration to match,
otherwise the next plan shows a replacement.

Importing `ad-hoc`, or any window that has no `cron` expression, fails with an
explanatory error rather than creating a resource that cannot be maintained.

## Drift and out-of-band deletion

If a scheduled window is deleted in the NodePing UI, the next refresh removes it
from state and the next apply recreates it. That is the correct behaviour for a
window that only disappears because a human deleted it — which is why the
resource refuses to manage ad-hoc windows, whose disappearance means nothing of
the sort.
