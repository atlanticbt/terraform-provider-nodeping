---
page_title: "nodeping_contact Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads one NodePing contact by contact ID.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contact (Data Source)

Reads one NodePing contact by its contact ID. The result includes the address ID of each address. A check or a contact group uses an address ID, not the contact ID. See [Address ID and contact ID](../resources/nodeping_contact.md#address-id-and-contact-id).

> **SECURITY:** An `address` can be a webhook URL that contains a token. The provider marks `address` as sensitive, so plan output hides it. The state file still contains the value. Protect the state file as a secret.

## Example Usage

```hcl
data "nodeping_contact" "example" {
  id = "201205050153W2Q4C-BKPGH"
}

output "contact_name" {
  value = data.nodeping_contact.example.name
}

output "contact_addresses" {
  value     = data.nodeping_contact.example.addresses
  sensitive = true
}
```

### Use an address ID in a contact group

```hcl
data "nodeping_contact" "ops" {
  id = "201205050153W2Q4C-BKPGH"
}

resource "nodeping_contactgroup" "ops_email" {
  name = "Ops Email"
  members = [
    for a in data.nodeping_contact.ops.addresses : a.id if a.type == "email"
  ]
}
```

## Schema

### Required

- `id` (String) The contact ID, for example `201205050153W2Q4C-BKPGH`.

This data source has no `customer_id` argument. To read a contact in a subaccount, set `customer_id` on the provider.

### Read-Only

- `customer_id` (String) The customer ID of the account that owns the contact.
- `name` (String) The name of the contact. A contact without a name has an empty string.
- `custrole` (String) The permission role: `owner`, `edit`, `view`, or `notify`.
- `addresses` (List of Object) The addresses of the contact, sorted by address ID. See [Nested Schema for `addresses`](#nested-schema-for-addresses).

### Nested Schema for `addresses`

Read-Only:

- `id` (String) The address ID, for example `K5SP9CQP`.
- `type` (String) The address type, for example `email`, `sms`, or `webhook`. Owner and login addresses can have an empty type.
- `address` (String, Sensitive) The email address, phone number, or URL.
- `suppress_up` (Boolean) `true` if NodePing does not send "up" notifications to this address.
- `suppress_down` (Boolean) `true` if NodePing does not send "down" notifications to this address.
- `suppress_first` (Boolean) `true` if NodePing does not send "first result" notifications to this address.
- `suppress_diag` (Boolean) Always `false`. NodePing does not store this field on a contact address.
- `suppress_all` (Boolean) `true` if NodePing sends no notifications to this address.

## Notes

- This data source does not return `mute`, `action`, `headers`, `querystrings`, `data`, or `priority`. To manage these values, use the [`nodeping_contact`](../resources/nodeping_contact.md) resource.
- If the contact does not exist, the read fails with `Error Reading Contact`.
