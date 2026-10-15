---
page_title: "nodeping_contacts Data Source - terraform-provider-nodeping"
subcategory: ""
description: |-
  Reads all NodePing contacts in one account.
---
<!--
doc: registry-data-source
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_contacts (Data Source)

Reads all NodePing contacts in one account. The result includes the address ID of each address. A check or a contact group uses an address ID, not a contact ID. See [Address ID and contact ID](../resources/nodeping_contact.md#address-id-and-contact-id).

> **SECURITY:** An `address` can be a webhook URL that contains a token. The provider marks `address` as sensitive, so plan output hides it. The state file still contains the value. Protect the state file as a secret.

## Example Usage

```hcl
data "nodeping_contacts" "all" {}

output "all_contact_ids" {
  value = [for c in data.nodeping_contacts.all.contacts : c.id]
}

output "all_contact_names" {
  value = [for c in data.nodeping_contacts.all.contacts : c.name]
}
```

### Find contacts by name

```hcl
locals {
  ops_contacts = [
    for c in data.nodeping_contacts.all.contacts : c
    if can(regex("ops", lower(c.name)))
  ]
}
```

### Collect every email address ID

```hcl
locals {
  email_address_ids = flatten([
    for c in data.nodeping_contacts.all.contacts : [
      for a in c.addresses : a.id if a.type == "email"
    ]
  ])
}
```

## Schema

This data source has no arguments. To list the contacts of a subaccount, set `customer_id` on the provider.

### Read-Only

- `contacts` (List of Object) All contacts, sorted by contact ID. See [Nested Schema for `contacts`](#nested-schema-for-contacts).

### Nested Schema for `contacts`

Read-Only:

- `id` (String) The contact ID, for example `201205050153W2Q4C-BKPGH`.
- `customer_id` (String) The customer ID of the account that owns the contact.
- `name` (String) The name of the contact. A contact without a name has an empty string.
- `custrole` (String) The permission role: `owner`, `edit`, `view`, or `notify`.
- `addresses` (List of Object) The addresses of the contact, sorted by address ID. See [Nested Schema for `contacts.addresses`](#nested-schema-for-contactsaddresses).

### Nested Schema for `contacts.addresses`

Read-Only:

- `id` (String) The address ID, for example `K5SP9CQP`.
- `type` (String) The address type. Owner and login addresses can have an empty type.
- `address` (String, Sensitive) The email address, phone number, or URL.

## Notes

- This list does not include suppression flags. To read them, use the [`nodeping_contact`](nodeping_contact.md) data source.
