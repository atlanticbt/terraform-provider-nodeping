# Fetch all checks
data "nodeping_checks" "all" {}

# Fetch only HTTP checks
data "nodeping_checks" "http_only" {
  type = "HTTP"
}

output "all_check_ids" {
  value = [for c in data.nodeping_checks.all.checks : c.id]
}

output "http_check_labels" {
  value = [for c in data.nodeping_checks.http_only.checks : c.label]
}

output "failing_checks" {
  value = [for c in data.nodeping_checks.all.checks : c.label if c.state == 0]
}
