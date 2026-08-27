# Fetch a single check by ID
data "nodeping_check" "example" {
  id = "201205050153W2Q4C-0J2HSIRF"
}

output "check_label" {
  value = data.nodeping_check.example.label
}

output "check_state" {
  description = "0 = failing, 1 = passing"
  value       = data.nodeping_check.example.state
}

output "check_enabled" {
  value = data.nodeping_check.example.enabled
}
