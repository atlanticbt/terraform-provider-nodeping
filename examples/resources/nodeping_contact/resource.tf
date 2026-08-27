# Basic contact with email
resource "nodeping_contact" "basic" {
  name     = "Operations Team"
  custrole = "notify"

  address {
    type    = "email"
    address = "ops@example.com"
  }
}

# Contact with multiple addresses
resource "nodeping_contact" "multi_address" {
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
}

# Contact with webhook
resource "nodeping_contact" "webhook" {
  name     = "Slack Webhook"
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

# Contact with notification suppression
resource "nodeping_contact" "suppressed" {
  name     = "Critical Only"
  custrole = "notify"

  address {
    type           = "email"
    address        = "critical@example.com"
    suppress_up    = true
    suppress_first = true
  }
}
