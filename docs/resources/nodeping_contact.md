---
page_title: "nodeping_contact Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing contact and its notification addresses.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contact (Resource)

Manages a NodePing contact. A contact owns one or more addresses, for example an email address, a phone number, or a webhook URL. NodePing gives each address its own address ID. Checks and contact groups notify an address ID, not a contact ID. See [Address ID and contact ID](#address-id-and-contact-id).

> **SECURITY:** The provider does not mark `address`, `headers`, `querystrings`, or `data` as sensitive. Webhook headers and query strings often contain an authentication token. These values appear in plan output, in the state file, and in configuration that `tofu plan -generate-config-out` writes. Protect the state file as a secret. Use a variable for each token, not a literal value.

## Example Usage

### Contact with one email address

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

### Contact with more than one address

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
    address = "+15555550123"
  }

  address {
    type     = "pushover"
    address  = "user-key"
    priority = 1
  }
}
```

### Contact with a webhook

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

### Webhook with a JSON body in a heredoc

You can write a JSON body with heredoc syntax. If the value is valid JSON, the provider compares it as JSON, not as text. See [Webhook `data` comparison](#webhook-data-comparison).

```hcl
variable "teams_webhook_key" {
  type      = string
  sensitive = true
}

resource "nodeping_contact" "msteams" {
  name     = "MS Teams Notifications"
  custrole = "notify"

  address {
    type    = "webhook"
    address = "https://outlook.office.com/webhook/..."
    action  = "post"
    headers = {
      "content-type" = "application/json"
    }
    querystrings = {
      "key" = var.teams_webhook_key
    }
    data = <<-EOT
      {
        "@type": "MessageCard",
        "title": "Host {label} : {type} is {event}",
        "text": "{message}"
      }
    EOT
  }
}
```

### Contact with notification suppression

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

### Contact in a subaccount

```hcl
resource "nodeping_contact" "client_ops" {
  customer_id = "201205050153W2Q4C"
  name        = "Client Operations"

  address {
    type    = "email"
    address = "ops@client.example.com"
  }
}
```

## Schema

### Optional

- `customer_id` (String) The customer ID of the subaccount that owns the contact. Set it to manage a contact in a subaccount. If you do not set it, the provider records the customer ID that NodePing returns. Changing it forces replacement.
- `custrole` (String) The permission role of the contact. Allowed values: `owner`, `edit`, `view`, `notify`. Default `notify`.
  - `owner`: the login contact of the account. The provider accepts this value so that you can import owner contacts.
  - `edit`: the contact can edit account settings and checks.
  - `view`: the contact can view account settings and checks.
  - `notify`: the contact only receives notifications.
- `name` (String) The name of the contact, used as a label. At most 1024 characters. NodePing cannot store an empty name. If you set `name = ""`, every plan shows a diff. Omit the attribute instead.

### Read-Only

- `id` (String) The contact ID, for example `201205050153W2Q4C-BKPGH`. This is not an address ID.

### Nested Schema for `address`

The `address` block is a list. Each block is one notification address.

Required:

- `address` (String) The destination: an email address, a phone number, a webhook URL, or a service key. 1 to 2048 characters. Examples: `user@example.com`, `+15555550123`, `https://example.com/webhook`.

Optional:

- `type` (String) The address type. Common values: `email`, `sms`, `voice`, `webhook`, `slack`, `hipchat`, `pushover`, `pushbullet`, `rocketchat`, `pagerduty`. The provider does not limit the values; NodePing validates them. Every new address must have a type. If a new address has no type, the apply fails with `Missing Address Type`. NodePing stores owner and login addresses without a type, so an import leaves `type` unset on them.
- `suppress_up` (Boolean) If `true`, NodePing does not send "up" notifications to this address. Default `false`.
- `suppress_down` (Boolean) If `true`, NodePing does not send "down" notifications to this address. Default `false`.
- `suppress_first` (Boolean) If `true`, NodePing does not send "first result" notifications to this address. Default `false`.
- `suppress_diag` (Boolean) Must be `false`. Default `false`. NodePing accepts this field on an address, discards it, and never returns it. A `true` value fails validation at plan time. To stop all notifications to an address, use `suppress_all`.
- `suppress_all` (Boolean) If `true`, NodePing sends no notifications to this address. Default `false`.
- `mute` (Boolean) If `true`, the address is muted. Default `false`. The provider sends an explicit `false`, so an unmute reaches NodePing.
- `action` (String) The HTTP method for a webhook address. Allowed values: `get`, `put`, `post`, `head`, `delete`. The provider sets no default.
- `headers` (Map of String) HTTP headers for a webhook address. Not sensitive. See the SECURITY notice at the top of this page.
- `querystrings` (Map of String) Query string parameters for a webhook address. Not sensitive. See the SECURITY notice at the top of this page.
- `data` (String) The request body for a webhook address, for `post` and `put`. At most 65536 characters. If the value is valid JSON, the provider compares it as JSON. See [Webhook `data` comparison](#webhook-data-comparison).
- `priority` (Number) The priority for a Pushover address. Allowed range: `-2` to `2`. The provider sets no default.

Read-Only:

- `id` (String) The address ID that NodePing assigns, for example `K5SP9CQP`. You cannot set it. Use this value in contact group `members` and in check notifications.

## Import

Import a contact in the account of the API token by its contact ID:

```shell
tofu import nodeping_contact.example 201205050153W2Q4C-BKPGH
```

Import a contact in a subaccount with the format `customer_id:contact_id`:

```shell
tofu import nodeping_contact.example 201205050153W2Q4C:201205050153W2Q4C-BKPGH
```

On OpenTofu 1.12 and later, you can also import with a resource identity. The identity has two attributes:

| Attribute | Required | Description |
|---|---|---|
| `id` | Yes | The contact ID. |
| `customer_id` | No | The customer ID of the subaccount. Omit it for the account of the API token. |

```hcl
import {
  to = nodeping_contact.example
  identity = {
    id          = "201205050153W2Q4C-BKPGH"
    customer_id = "201205050153W2Q4C"
  }
}
```

> **CAUTION:** Import writes `customer_id` to state, and the provider uses it for every later API call. `customer_id` forces replacement. If your configuration has a different value, the next plan replaces the contact. Set `customer_id` in the configuration to the imported value.

NodePing returns the addresses of a contact as a map keyed by address ID. After import, the `address` blocks in state are in address ID order, not in configuration order. Thus the first plan after import can show the blocks in a new order. The apply sends no address changes, because the provider matches addresses by content. After that apply, the plan is empty.

> **SECURITY:** If the account uses webhook authentication, `tofu plan -generate-config-out` writes live tokens into the generated file. Remove the tokens from the file before you commit it. Replace each token with a variable reference.

## Address ID and contact ID

A check notifies an address ID or a contact group ID. A check never notifies a contact ID. The `notifications.contact_id` attribute of `nodeping_check` has a misleading name: give it an address ID.

| Value | Example | Where to use it |
|---|---|---|
| Contact ID | `201205050153W2Q4C-BKPGH` | Import of `nodeping_contact`. The `nodeping_contact` data source. |
| Address ID | `K5SP9CQP` | `notifications.contact_id` of `nodeping_check`. `members` of `nodeping_contactgroup`. |

Use `nodeping_contact.x.address[0].id` to get an address ID:

```hcl
resource "nodeping_check" "web" {
  # ...

  notifications {
    contact_id = nodeping_contact.x.address[0].id
  }
}
```

Do not use `nodeping_contact.x.id` where NodePing expects an address ID. NodePing accepts a contact ID there, but it sends no notifications to it.

The provider matches each `address` block to a NodePing address by content (`type` and `address`), not by list position. This has these results:

- If you add, remove, or reorder `address` blocks, each address keeps its address ID.
- If you change the `address` value and keep the same `type`, the address keeps its address ID.
- If you change the `type`, NodePing creates a new address with a new address ID.

## Webhook `data` comparison

If both the configured `data` value and the value from NodePing are valid JSON, the provider compares them as JSON. Whitespace, indentation, and key order do not cause a diff. If one value is not JSON, for example a form-encoded body, the provider compares the text exactly.

State keeps the exact string from your configuration. NodePing can return `data` as a string or as a decoded JSON object. If the returned value is equivalent JSON, the provider keeps your string.

Two parts of the provider do this comparison:

- A custom value type with semantic equality. It applies during refresh, create, and update. It prevents the error `Provider produced inconsistent result after apply`.
- A plan modifier. It applies during plan. After an import, state holds the encoding that NodePing returned. The plan modifier keeps that state value if your configuration is equivalent JSON, so the plan shows no whitespace diff.

## Notes

- NodePing sends a welcome email with login credentials to a contact with the `edit` or `view` role.
- To prevent the welcome email, create the contact with the `notify` role first. Then change the role to `edit` or `view`.
- NodePing does not let a contact go from one or more addresses to zero addresses. If an update removes every `address` block, the apply fails with `Cannot Remove Every Contact Address`. Keep at least one address, or destroy the contact. You can create a contact with no addresses.
- In a webhook `headers` or `querystrings` value, NodePing can store a JSON object, for example an OpsGenie custom field. The provider reads that value as JSON text. When the text is a JSON object or array, the provider sends it back as JSON. The provider sends all other values as strings.
