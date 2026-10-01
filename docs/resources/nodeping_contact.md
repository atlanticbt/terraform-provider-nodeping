---
page_title: "nodeping_contact Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing contact for receiving notifications.
---

# nodeping_contact (Resource)

Manages a NodePing contact for receiving notifications.

## Example Usage

### Basic Contact with Email

```hcl
resource "nodeping_contact" "basic" {
  name     = "Operations Team"
  custrole = "notify"

  address {
    type    = "email"
    address = "ops@example.com"
  }
}
```

### Contact with Multiple Addresses

```hcl
resource "nodeping_contact" "multi" {
  name     = "On-Call Engineer"
  custrole = "notify"

  address {
    type    = "email"
    address = "oncall@example.com"
  }

  address {
    type    = "sms"
    address = "+1-555-123-4567"
  }

  address {
    type     = "pushover"
    address  = "user-key"
    priority = 1
  }
}
```

### Contact with Webhook

```hcl
resource "nodeping_contact" "webhook" {
  name     = "Slack Notifications"
  custrole = "notify"

  address {
    type    = "webhook"
    address = "https://hooks.slack.com/services/XXX/YYY/ZZZ"
    action  = "post"
    headers = {
      "Content-Type" = "application/json"
    }
    data = jsonencode({
      text = "NodePing Alert: {{label}} is {{event}}"
    })
  }
}
```

### Contact with Notification Suppression

```hcl
resource "nodeping_contact" "critical_only" {
  name     = "Critical Alerts Only"
  custrole = "notify"

  address {
    type           = "email"
    address        = "critical@example.com"
    suppress_up    = true
    suppress_first = true
  }
}
```

## Argument Reference

### Top-Level Arguments

- `name` - (Optional) The name of the contact. Used as a display label. NodePing has no way
  to store an empty name, so `name = ""` behaves as if the attribute were omitted and will
  show a permanent diff; leave it out instead.
- `custrole` - (Optional) The permission role for this contact. Valid values: `owner`, `edit`, `view`, `notify`. Defaults to `notify`.
  - `owner` - The account's own login contact. Accepted so owner contacts can be imported.
  - `edit` - Can edit account settings and checks
  - `view` - Can view account settings and checks
  - `notify` - Can only receive notifications

### Address Block

The `address` block supports the following arguments:

- `type` - (Optional) The type of notification address. Required on every address NodePing
  has not created yet; the provider refuses a new address without one. Owner and login
  addresses are stored by NodePing with no type, and are imported with `type` unset. Values:
  - `email` - Email address
  - `sms` - SMS phone number
  - `voice` - Voice call phone number
  - `webhook` - HTTP webhook
  - `slack` - Slack webhook
  - `pushover` - Pushover notification
  - `pagerduty` - PagerDuty integration
  - `hipchat` - HipChat (legacy)

- `address` - (Required) The address value. Format depends on type:
  - Email: `user@example.com`
  - SMS/Voice: `+1-555-123-4567`
  - Webhook: `https://example.com/webhook`

  Not marked sensitive, so it is shown in plan output. Marking it sensitive would make the
  contact impossible to import: `tofu plan -generate-config-out` writes `null` for a sensitive
  attribute, and a required attribute set to `null` is a configuration error. The value is an
  email address or phone number rather than a credential, and NodePing displays it in its own
  UI. `headers` and `querystrings` are treated the same way, for the reason given under
  [Webhook-Specific Arguments](#webhook-specific-arguments).

- `suppress_up` - (Optional) Suppress "up" notifications. Defaults to `false`.
- `suppress_down` - (Optional) Suppress "down" notifications. Defaults to `false`.
- `suppress_first` - (Optional) Suppress "first result" notifications. Defaults to `false`.
- `suppress_diag` - (Optional) Must be `false`, which is the default. NodePing accepts
  `suppressdiag` on a contact address, discards it, and never returns it, so a `true` here
  would leave state claiming a suppression that is not in effect. The provider rejects it at
  plan time. Use `suppress_all` to stop notifications to an address.
- `suppress_all` - (Optional) Suppress all notifications. Defaults to `false`.
- `mute` - (Optional) Mute all notifications to this address. Defaults to `false`.

#### Webhook-Specific Arguments

- `action` - (Optional) HTTP method for webhook requests. Valid values: `get`, `put`, `post`, `head`, `delete`. Defaults to `get`.
- `headers` - (Optional) Map of HTTP headers to send with webhook requests.
- `querystrings` - (Optional) Map of query string parameters to append to webhook URL.

  Neither is marked sensitive, so both appear in plan output and in generated configuration. That
  is deliberate: a sensitive attribute is written as `null` by `tofu plan -generate-config-out`,
  so an imported webhook contact would plan `(sensitive value) -> null` and the apply would clear
  the real values — silently, because the diff shows `(sensitive value)` on both sides.

  A value NodePing stores as a JSON object (an OpsGenie integration keeps a custom field this
  way) is read as its JSON text and sent back as a JSON object. Only text that parses as a
  JSON object or array is sent that way; everything else is sent as a string.

  These frequently carry an authentication token. Configuration generated from an account that
  uses webhook auth therefore contains live credentials: scrub the file before committing it, and
  prefer a variable reference over the literal value. See A-015 in `docs/assumptions.md`.
- `data` - (Optional) Request body for POST/PUT webhook requests.

#### Pushover-Specific Arguments

- `priority` - (Optional) Pushover notification priority. Valid values: `-2` to `2`. Defaults to `2`.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

- `id` - The unique identifier of the contact.
- `customer_id` - The customer ID (account ID) that owns this contact.

Each `address` block also exports:

- `id` - The unique identifier of the address within the contact.

## Import

Contacts can be imported using the contact ID:

```shell
terraform import nodeping_contact.example 201205050153W2Q4C-BKPGH
```

For SubAccount contacts, use the format `customer_id:contact_id`:

```shell
terraform import nodeping_contact.example 201205050153W2Q4C:201205050153W2Q4C-BKPGH
```

NodePing returns a contact's addresses as a map keyed by address ID, so an imported contact
lists its `address` blocks in ID order rather than in the order the configuration writes
them. For a contact with more than one address the first plan after import may therefore show
the blocks being reordered. The apply is a no-op at the API — addresses are matched by
content, so no address ID changes — and the plan after it is empty.

## Notes

- Contacts with `edit` or `view` roles will receive a welcome email with login credentials.
- To avoid welcome emails, create the contact with `notify` role first, then update to `edit` or `view`.
- Address IDs are generated by NodePing and cannot be set manually.
- A contact cannot be reduced to zero addresses. NodePing refuses the update, so the
  provider rejects it at plan time: keep at least one address, or destroy the contact.
  A contact created with no addresses at all is allowed.
