# HTTP Check
resource "nodeping_check" "http" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Example Website"
  enabled = true

  interval  = 5
  threshold = 10
  sens      = 2

  runlocations = ["nam"]
  tags         = ["production", "website"]
}

# HTTPS Check with content matching
resource "nodeping_check" "http_content" {
  type          = "HTTPCONTENT"
  target        = "https://api.example.com/health"
  label         = "API Health Check"
  enabled       = true
  contentstring = "\"status\":\"ok\""

  interval  = 1
  threshold = 5
}

# DNS Check
resource "nodeping_check" "dns" {
  type          = "DNS"
  target        = "8.8.8.8"
  label         = "DNS Resolution Check"
  enabled       = true
  dnstype       = "A"
  dnstoresolve  = "example.com"
  contentstring = "93.184.216.34"
}

# SSL Certificate Check
resource "nodeping_check" "ssl" {
  type        = "SSL"
  target      = "example.com"
  label       = "SSL Certificate Expiry"
  enabled     = true
  warningdays = 30
  servername  = "example.com"
}

# PING Check
resource "nodeping_check" "ping" {
  type    = "PING"
  target  = "8.8.8.8"
  label   = "Google DNS Ping"
  enabled = true

  interval  = 5
  threshold = 5
}

# PORT Check
resource "nodeping_check" "port" {
  type    = "PORT"
  target  = "example.com"
  label   = "SSH Port Check"
  enabled = true
  port    = 22
}

# SMTP Check
resource "nodeping_check" "smtp" {
  type        = "SMTP"
  target      = "mail.example.com"
  label       = "Mail Server"
  enabled     = true
  port        = 587
  secure      = "starttls"
  warningdays = 14
}

# Check with notifications
resource "nodeping_check" "with_notifications" {
  type    = "HTTP"
  target  = "https://critical.example.com"
  label   = "Critical Service"
  enabled = true

  interval  = 1
  threshold = 5
  sens      = 1

  notifications {
    contact_id = nodeping_contact.basic.id
    delay      = 0
    schedule   = "All"
  }

  notifications {
    contact_id = nodeping_contact.webhook.id
    delay      = 5
    schedule   = "All"
  }

  tags = ["critical", "production"]
}

# Check with dependency
resource "nodeping_check" "dependent" {
  type    = "HTTP"
  target  = "https://app.example.com"
  label   = "Application"
  enabled = true
  dep     = nodeping_check.http.id

  description = "This check depends on the main website being up"
}

# Reference to contacts defined in contact example
data "nodeping_contact" "basic" {
  id = nodeping_contact.basic.id
}
