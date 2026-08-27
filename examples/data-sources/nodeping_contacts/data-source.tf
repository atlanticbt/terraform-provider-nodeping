# Fetch all contacts
data "nodeping_contacts" "all" {}

output "all_contact_ids" {
  value = [for c in data.nodeping_contacts.all.contacts : c.id]
}

output "all_contact_names" {
  value = [for c in data.nodeping_contacts.all.contacts : c.name]
}
