# Fetch a single contact by ID
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
