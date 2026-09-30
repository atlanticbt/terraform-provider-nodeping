---
page_title: "nodeping_subaccount Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing subaccount.
---

# nodeping_subaccount (Resource)

Manages a NodePing subaccount.

NodePing's `/accounts` endpoint creates subaccounts and nothing else. A top-level account
cannot be created, imported or destroyed through the API, and `id`/`customerid` are not
accepted on create. That is why this resource is named `nodeping_subaccount` rather than
`nodeping_account`: it can only ever manage a subaccount. To read the account your API
token belongs to, use the [`nodeping_account`](../data-sources/nodeping_account.md) data
source.

The resource *is* the account, so there is no `customer_id` attribute. Its `id` is the
subaccount's NodePing customer ID.

## Destroying a subaccount destroys everything inside it

`terraform destroy` on this resource removes the subaccount along with every check,
contact, schedule and stored result it owns. There is no undo, no export, and no
confirmation beyond Terraform's own plan. This is the most destructive operation the
provider offers.

Guard any subaccount that holds real monitoring:

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

`prevent_destroy` also blocks any change that would force replacement. The provider itself
never plans a replacement for the create-only attributes described below.

## Deletion is asynchronous

NodePing answers a delete with `{"success":"Records queued for deletion."}` and performs
the removal afterwards. The provider treats that acknowledgement as success — polling for
disappearance would stall the apply on a queue it cannot influence. Consequences:

- The subaccount may still appear in `nodeping_accounts` for a short time after a
  successful `terraform destroy`.
- Recreating a subaccount with the same name immediately after destroying one is fine;
  NodePing assigns a new ID.

## Create-only attributes

`contactname` and `email` are accepted only on create. NodePing's update endpoint does not
take them and no read returns them, so the provider cannot refresh them or change them in
place. Changing either on an existing subaccount is **an error at plan time**: change it in
the NodePing UI, then update the configuration to match. The provider never replaces a
subaccount to apply such a change, because replacing one deletes everything in it. Removing
either from the configuration is allowed and stops tracking it.

`emailme` and `autodiagnotifications` are also never returned by the API. They are written
on create and update and tracked from configuration; the provider deliberately does not map
them from responses, because doing so would overwrite the configured value with an empty
string and produce permanent drift. If either is changed outside Terraform, the provider
cannot detect it.

`location` is likewise write-only. NodePing reports the account's regions back as
`default_locations`, which is exposed separately rather than being folded into `location`.

## Example Usage

### Basic subaccount

```hcl
resource "nodeping_subaccount" "client" {
  name        = "Example Client"
  contactname = "Example Operations"
  email       = "ops@example.com"
  timezone    = "-7"
  location    = "nam"
}
```

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

### Contacts and checks inside the subaccount

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

- `name` (String) The subaccount name. Returned by the API as `customer_name`.
- `timezone` (String) GMT offset expressed as a signed number in a string, for example
  `"-7"`, `"0"` or `"5.5"`. NodePing may return it with a decimal, such as `"-5.0"`.

### Required to create

These are Optional in the schema so that an existing subaccount can be imported, because
NodePing never returns them. Planning a new subaccount without all three is an error.

- `contactname` (String) Name of the subaccount's primary contact. Sent only on create;
  the API neither returns nor updates it. Changing it once it is set is refused at plan time.
- `email` (String) Email address of the subaccount's primary contact. Sent only on create;
  the API neither returns nor updates it. Changing it once it is set is refused at plan time.
- `location` (String) Default probe region. One of `nam`, `lam`, `eur`, `eao`, `wlw`. On
  import it is taken from `default_locations` when that holds exactly one region.

  NodePing's create documentation lists only `nam`, `eur`, `eao` and `wlw` and omits `lam`.
  Its update documentation and the `runlocations` field on checks both accept `lam`, so the
  provider treats the omission as a documentation bug and accepts `lam` here.

### Optional

- `emailme` (String) Whether NodePing emails the parent account about this subaccount.
  `"yes"` or `"no"`. Defaults to `"no"`. NodePing returns it as a boolean, which is read
  back as `"yes"` or `"no"`.
- `autodiagnotifications` (String) Whether automatic diagnostics are attached to
  notifications. `"true"` or `"false"`. Defaults to `"false"`. Never returned by the API.
- `status` (String) `"Active"` or `"Suspend"`. A suspended subaccount stops running its
  checks. Modelled as a string rather than a boolean to match the API. Not supported on a
  primary account, which this resource never manages. Defaults to `"Active"` as assigned
  by NodePing at creation.

### Read-Only

- `id` (String) The subaccount's NodePing customer ID, for example `201203232048C76FH`.
- `type` (String) Account type reported by NodePing, for example `subcustomer`.
- `creation_date` (Number) Creation timestamp reported by NodePing.
- `next_billing_date` (String) Next billing date reported by NodePing.
- `default_locations` (List of String) Probe regions NodePing records as the subaccount's
  defaults. Derived from `location`; NodePing may normalise or expand the value.

## Not implemented: `accountsuppressall`

NodePing's update endpoint accepts `accountsuppressall` (`"true"`/`"false"`), which
disables or re-enables all notifications for an account. It is not exposed as an attribute,
for two reasons:

1. No read returns it, so its value could never be refreshed. Terraform would carry a
   configuration value it can neither confirm nor detect drift on.
2. It is not idempotent in the way a declarative attribute needs to be. Re-enabling
   notifications does **not** restore contacts that were suppressed individually, so
   `accountsuppressall = "false"` does not describe a state the provider can guarantee.

It is an operational switch, not configuration. Use the NodePing dashboard or a direct API
call during an incident.

## Import

```shell
terraform import nodeping_subaccount.client 201203232048C76FH
```

Import cannot recover `contactname`, `email` or `autodiagnotifications`, because NodePing
never returns them. `contactname` and `email` stay null, which the schema allows on an
existing subaccount; `autodiagnotifications` is seeded with its default of `"false"`.
`emailme` is read from the API, and `location` is taken from `default_locations` when that
holds a single region.

Deleting a subaccount deletes every check and contact in it, and a deleted subaccount cannot
be recovered. Give imported subaccounts `lifecycle { prevent_destroy = true }`.

Setting `contactname` or `email` on an imported subaccount, where state holds no value,
records the configured value with no API call. Once a value is recorded, changing it is an
error at plan time rather than a replacement.
