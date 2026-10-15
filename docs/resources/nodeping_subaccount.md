---
page_title: "nodeping_subaccount Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing subaccount.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_subaccount (Resource)

Manages a NodePing subaccount.
The NodePing `/accounts` endpoint creates subaccounts only.
The API cannot create, import or destroy a top-level account, and it does not accept `id` or `customerid` on create.
Thus the name of this resource is `nodeping_subaccount`, not `nodeping_account`.
To read the account of your API token, use the [`nodeping_account`](../data-sources/nodeping_account.md) data source.

The resource is the account, so it has no `customer_id` attribute.
Its `id` is the customer ID of the subaccount.

> **WARNING:** When you destroy this resource, NodePing deletes the subaccount and every check, contact and stored result in it. You cannot undo this. Add `lifecycle { prevent_destroy = true }` to each subaccount that holds real monitoring.

> **SECURITY:** This resource requires the API token of the parent account. That token can change and delete every subaccount. Keep it in a sensitive variable or in the `NODEPING_API_TOKEN` environment variable. Do not commit it to the repository.

## Example Usage

### Basic subaccount

```hcl
resource "nodeping_subaccount" "client" {
  name        = "Example Client"
  contactname = "Example Operations"
  email       = "ops@example.com"
  timezone    = "-7"
  location    = "nam"

  lifecycle {
    prevent_destroy = true
  }
}
```

`prevent_destroy` also blocks a change that forces replacement.
The provider never plans a replacement for the create-only attributes.

### Suspended subaccount with parent notifications

```hcl
resource "nodeping_subaccount" "staging" {
  name                  = "Example Staging"
  contactname           = "Example Operations"
  email                 = "ops@example.com"
  timezone              = "0"
  location              = "eur"
  emailme               = "yes"
  autodiagnotifications = "true"
  status                = "Suspend"

  lifecycle {
    prevent_destroy = true
  }
}
```

### Contacts and checks in the subaccount

Set `customer_id` on objects in the subaccount to the `id` of this resource:

```hcl
resource "nodeping_subaccount" "client" {
  name        = "Example Client"
  contactname = "Example Operations"
  email       = "ops@example.com"
  timezone    = "-7"
  location    = "nam"
}

resource "nodeping_contact" "client_ops" {
  customer_id = nodeping_subaccount.client.id
  name        = "Client Operations"
  custrole    = "notify"

  address {
    type    = "email"
    address = "ops@example.com"
  }
}
```

## Schema

### Required

- `name` (String) The name of the subaccount. The API returns it as `customer_name`. Length 1 to 255 characters.
- `timezone` (String) The offset from GMT, as a signed number in a string, for example `"-7"`, `"0"` or `"5.5"`. It must match `^[+-]?\d{1,2}(\.\d{1,2})?$`. NodePing can return the value with a decimal, for example `"-5.0"`.

### Required to create

The schema marks these attributes as optional, because NodePing never returns them.
This lets you import an existing subaccount.
If you plan a new subaccount without all three, the plan fails.

- `contactname` (String) The name of the primary contact of the subaccount. Length 1 to 255 characters. The provider sends it on create only. The API does not return or update it. A change after it is set fails at plan time. See [Create-only attributes](#create-only-attributes).
- `email` (String) The email address of the primary contact of the subaccount. Length 3 to 320 characters, and it must have the form of an email address. The provider sends it on create only. The API does not return or update it. A change after it is set fails at plan time.
- `location` (String) The default probe region. One of `nam`, `lam`, `eur`, `eao`, `wlw`. NodePing does not return it. On import, the provider takes it from `default_locations` when that list holds exactly one region. A change is sent as an in-place update.

  The NodePing create documentation lists only `nam`, `eur`, `eao` and `wlw`.
  The update documentation and the check `runlocations` field both accept `lam`.
  Thus the provider treats the omission as a documentation error and accepts `lam`.

### Optional

- `emailme` (String) If `"yes"`, NodePing sends email about this subaccount to the parent account. `"yes"` or `"no"`. Default `"no"`. NodePing returns it as a boolean, and the provider reads it back as `"yes"` or `"no"`.
- `autodiagnotifications` (String) If `"true"`, NodePing attaches automatic diagnostics to notifications. `"true"` or `"false"`. Default `"false"`. The API never returns it, so the provider tracks it from the configuration only. The provider cannot find a change that is made outside OpenTofu.
- `status` (String) `"Active"` or `"Suspend"`. A suspended subaccount does not run its checks. The type is a string, not a boolean, to match the API. If you do not set it, the value comes from NodePing, which creates each subaccount as `"Active"`.

### Read-Only

- `id` (String) The customer ID of the subaccount, for example `201203232048C76FH`.
- `type` (String) The account type that NodePing reports, for example `subcustomer`.
- `creation_date` (Number) The creation time that NodePing reports.
- `next_billing_date` (String) The next billing date that NodePing reports.
- `default_locations` (List of String) The probe regions that NodePing records as the defaults of the subaccount. NodePing derives them from `location`, and it can normalize or expand the value.

## Import

Import a subaccount by its customer ID:

```shell
tofu import nodeping_subaccount.client 201203232048C76FH
```

With OpenTofu 1.12 or later, you can also use an `import` block with an `identity`.
The identity has one attribute, `id`. It has no `customer_id`, because the subaccount is the account.

```hcl
import {
  to = nodeping_subaccount.client
  identity = {
    id = "201203232048C76FH"
  }
}
```

The import cannot recover `contactname`, `email` or `autodiagnotifications`, because NodePing never returns them.
After the import:

- `contactname` and `email` stay null. The schema permits this on an existing subaccount.
- `autodiagnotifications` gets its default, `"false"`.
- `emailme` comes from the API. If the API does not return it, it gets its default, `"no"`.
- `location` comes from `default_locations` when that list holds exactly one region.

If you then set `contactname` or `email`, the provider records the value in state with no API call.
After the provider records a value, a change to it fails at plan time.

> **WARNING:** A deleted subaccount cannot be recovered. Add `lifecycle { prevent_destroy = true }` to each imported subaccount before you run `tofu apply`.

## Create-only attributes

NodePing accepts `contactname` and `email` only when it creates the subaccount.
The update endpoint does not accept them, and no read returns them.
Thus the provider cannot refresh them or change them in place.

If you change either attribute on an existing subaccount, the plan fails.
To change the value, do these steps:

1. Change the value in the NodePing UI.
2. Set the configuration to the new value.

The provider never replaces a subaccount to apply such a change, because a replacement deletes everything in it.
You can remove either attribute from the configuration. The provider then stops tracking it.

## Destroy a subaccount

> **WARNING:** This step deletes the subaccount and every check, contact and stored result in it. NodePing has no undo and no export. OpenTofu shows only its usual plan before the delete.

1. Remove `prevent_destroy` from the resource, if it is set.
2. Run `tofu destroy -target=nodeping_subaccount.client`, or remove the resource and run `tofu apply`.

The provider does not delete the account that is set as its own `customer_id`.
That request fails before it reaches the API.

### Deletion is asynchronous

NodePing answers a delete with `{"success":"Records queued for deletion."}`.
It removes the records later.
The provider treats that answer as success.
It does not poll, because a poll can stop the apply on a queue that the provider cannot control.
As a result:

- The subaccount can stay in [`nodeping_accounts`](../data-sources/nodeping_accounts.md) for a short time after a successful destroy.
- You can create a subaccount with the same name immediately after you destroy one. NodePing assigns a new ID.

## Notes

### Not implemented: `accountsuppressall`

The NodePing update endpoint accepts `accountsuppressall` (`"true"` or `"false"`).
It disables or enables all notifications for an account.
The provider does not expose it as an attribute, for two reasons:

1. No read returns it. OpenTofu cannot refresh the value or find drift on it.
2. It is not idempotent. When you enable notifications again, contacts that were suppressed one by one stay suppressed. Thus `accountsuppressall = "false"` does not describe a state that the provider can guarantee.

It is an operational switch, not configuration.
During an incident, use the NodePing dashboard or a direct API call.
