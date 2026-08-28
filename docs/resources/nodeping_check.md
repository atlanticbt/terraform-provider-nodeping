---
page_title: "nodeping_check Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing monitoring check.
---

# nodeping_check (Resource)

Manages a NodePing monitoring check.

## Example Usage

### HTTP Check

```hcl
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
```

### HTTPS Content Check

```hcl
resource "nodeping_check" "content" {
  type          = "HTTPCONTENT"
  target        = "https://api.example.com/health"
  label         = "API Health Check"
  enabled       = true
  contentstring = "\"status\":\"ok\""
  regex         = false
}
```

### DNS Check

```hcl
resource "nodeping_check" "dns" {
  type          = "DNS"
  target        = "8.8.8.8"
  label         = "DNS Resolution"
  enabled       = true
  dnstype       = "A"
  dnstoresolve  = "example.com"
  contentstring = "93.184.216.34"
}
```

### SSL Certificate Check

```hcl
resource "nodeping_check" "ssl" {
  type        = "SSL"
  target      = "example.com"
  label       = "SSL Certificate Expiry"
  enabled     = true
  warningdays = 30
  servername  = "example.com"
}
```

### PING Check

```hcl
resource "nodeping_check" "ping" {
  type    = "PING"
  target  = "8.8.8.8"
  label   = "Google DNS Ping"
  enabled = true
}
```

### PORT Check

```hcl
resource "nodeping_check" "port" {
  type    = "PORT"
  target  = "example.com"
  label   = "SSH Port"
  enabled = true
  port    = 22
}
```

### SMTP Check with TLS

```hcl
resource "nodeping_check" "smtp" {
  type        = "SMTP"
  target      = "mail.example.com"
  label       = "Mail Server"
  enabled     = true
  port        = 587
  secure      = "starttls"
  warningdays = 14
}
```

### Check with Notifications

```hcl
resource "nodeping_check" "with_notifications" {
  type    = "HTTP"
  target  = "https://critical.example.com"
  label   = "Critical Service"
  enabled = true

  interval  = 1
  threshold = 5
  sens      = 1

  notifications {
    contact_id = nodeping_contact.ops.id
    delay      = 0
    schedule   = "All"
  }

  notifications {
    contact_id = nodeping_contact.escalation.id
    delay      = 15
    schedule   = "All"
  }
}
```

### Check with Dependency

```hcl
resource "nodeping_check" "router" {
  type    = "PING"
  target  = "192.168.1.1"
  label   = "Edge Router"
  enabled = true
}

resource "nodeping_check" "service" {
  type    = "HTTP"
  target  = "https://internal.example.com"
  label   = "Internal Service"
  enabled = true
  dep     = nodeping_check.router.id
}
```

## Argument Reference

### Common Arguments

- `type` - (Required) The type of check. See [Supported Check Types](#supported-check-types).
- `target` - (Required for most types) The target URL, hostname, or IP address.
- `label` - (Optional) Display label for the check. Defaults to target.
- `enabled` - (Optional) Whether the check is enabled. Defaults to `false`.
- `public` - (Optional) Enable public reports. Defaults to `false`.
- `interval` - (Optional) Check interval in minutes. Can be `0.25`, `0.5`, or any integer >= 1. Defaults to `15`.
- `threshold` - (Optional) Timeout in seconds. Defaults to `5`.
- `sens` - (Optional) Number of rechecks before status change. Defaults to `2`.
- `mute` - (Optional) Mute all notifications. Defaults to `false`.
- `dep` - (Optional) Check ID for notification dependency.
- `description` - (Optional) Description text (max 1000 characters).
- `autodiag` - (Optional) Enable automated diagnostics. Defaults to `false`.

### Location Arguments

- `runlocations` - (Optional) List of probe locations. Can be region codes (`nam`, `lam`, `eur`, `eao`, `wlw`) or probe codes (`ca`, `ny`, `tx`, etc.).
- `homeloc` - (Optional) Preferred probe location or `roam` for rotating.

### Tagging

- `tags` - (Optional) List of tags for grouping checks.

### Notifications Block

- `contact_id` - (Required) Contact or contact group ID to notify.
- `delay` - (Optional) Delay in minutes before sending notification. Defaults to `0`.
- `schedule` - (Optional) Notification schedule name.

### Content Matching Arguments

- `contentstring` - (Optional) String to match in response.
- `regex` - (Optional) Treat contentstring as regular expression.
- `invert` - (Optional) Invert match (does not contain).

### HTTP Arguments

- `follow` - (Optional) Follow redirects (up to 4).
- `method` - (Optional) HTTP method for HTTPADV: `GET`, `POST`, `PUT`, `HEAD`, `TRACE`, `CONNECT`.
- `statuscode` - (Optional) Expected HTTP status code.
- `sendheaders` - (Optional) Map of request headers.
- `receiveheaders` - (Optional) Map of expected response headers.
- `postdata` - (Optional) POST request body.
- `ipv6` - (Optional) Use IPv6.

### DNS Arguments

- `dnstype` - (Optional) DNS query type: `ANY`, `A`, `AAAA`, `CNAME`, `MX`, `NS`, `PTR`, `SOA`, `SRV`, `TXT`.
- `dnstoresolve` - (Optional) FQDN to resolve.
- `dnssection` - (Optional) DNS section to check: `answer`, `authority`, `additional`, `edns_options`.
- `dnsrd` - (Optional) Recursion Desired bit. Defaults to `true`.
- `transport` - (Optional) Transport protocol: `udp`, `tcp`.

### SSL/TLS Arguments

- `warningdays` - (Optional) Days before expiry to fail check.
- `servername` - (Optional) Server name for SNI.
- `verify` - (Optional) Verify SSL certificate.
- `secure` - (Optional) SSL mode: `false`, `ssl`, `starttls`.

### Authentication Arguments

- `username` - (Optional) Authentication username.
- `password_wo` - (Optional, Write-only, Sensitive) Authentication password. The value is sent to the
  NodePing API but is never written to the state file or to a saved plan. Conflicts with `password`.
- `password_wo_version` - (Optional) Integer that triggers re-sending `password_wo`. Increment it
  whenever the password changes.
- `password` - (Optional, Sensitive, **Deprecated**) Authentication password. This value is stored in
  state in plaintext. Use `password_wo` instead. Conflicts with `password_wo`.
- `email` - (Optional) Email address used by `SMTP` checks when verifying mail delivery.
- `sshkey` - (Optional) SSH private key ID.
- `clientcert` - (Optional) Client certificate ID.

#### Write-only passwords

NodePing stores and returns check passwords in plaintext, so the credential cannot be protected on
their side. What the provider controls is the state file, and `password_wo` is how it keeps the
credential out of it.

```hcl
variable "ftp_password" {
  type      = string
  sensitive = true
}

resource "nodeping_check" "ftp" {
  type     = "FTP"
  target   = "ftp.example.com"
  username = "monitor"

  password_wo         = var.ftp_password
  password_wo_version = 1
}
```

Write-only values are invisible to the plan differ: OpenTofu never stores them, so it has nothing to
compare against and a changed `password_wo` on its own produces no diff. `password_wo_version` is
what makes a rotation planable — bump it in the same change that alters the password.

The provider also never copies the password NodePing returns into state, on any code path, including
import and refresh. After an import, `password_wo` and `password` are both null; add `password_wo` to
the configuration to bring the credential back under management.

Requires OpenTofu 1.11 or later. This provider targets OpenTofu 1.12+.

Consumers should still enable OpenTofu state encryption. That protects the state at rest; it is not a
substitute for keeping the secret out of state in the first place.

### Network Arguments

- `port` - (Optional/Required) Port number. Required for PORT and NTP checks.

### Database Arguments

- `database` - (Optional) Database name.
- `query` - (Optional) Query to execute.
- `namespace` - (Optional) MongoDB collection namespace.

### SNMP Arguments

- `snmpv` - (Optional) SNMP version: `1`, `2c`.
- `snmpcom` - (Optional) SNMP community string.

### Type-specific Arguments

- `data` - (Optional) Map of strings. `CLUSTER` maps each member check ID to `"1"` (must be passing)
  or `"0"` (must be failing); `HTTPADV` uses it for form data; `WEBSOCKET`, `SPEC10DNS` and
  `SPEC10RDDS` use it for their own settings.
- `fields` - (Optional) Map of result fields to parse and range check, used by `HTTPPARSE`, `SNMP`,
  `MONGODB`, `MYSQL` and `PGSQL`. Each entry takes `name` (required), `min`, `max`, and `match`.
  `match` is only honoured by `PGSQL`, `MYSQL` and `MONGODB`.
- `hosts` - (Optional) Map of `REDIS` hosts, keyed by an arbitrary identifier. Each entry takes
  `host` (required), `port`, and `password` (sensitive). The per-host password is never read back
  from the API into state.
- `redistype` - (Optional) `standalone` (default), `sentinel` or `cluster`. `REDIS` only.
- `sentinelname` - (Optional) Redis Sentinel master name. **Required** when `redistype` is
  `sentinel`.
- `oldresultfail` - (Optional) Fail an `AGENT` or `PUSH` check when its results are older than the
  check interval.
- `edns` - (Optional) Map of EDNS options for `DOHDOT` checks. Every key must be a positive integer
  option code.
- `dohdot` - (Optional) `doh` (default) or `dot`. `DOHDOT` only.
- `whoisserver` - (Optional) WHOIS server to query. `WHOIS` only.
- `rdapurl` - (Optional) RDAP service URL, an absolute `http` or `https` URL. `RDAP` only.
- `ignore` - (Optional) Values to ignore: a comma separated list of blocklists for `RBL`, or of
  response codes for `SIP`.
- `verifyvolume` - (Optional) Enable volume detection. `AUDIO` only.
- `volumemin` - (Optional) Minimum acceptable volume in dB, between `-90` and `0`. `AUDIO` only.

#### REDIS Sentinel

```hcl
resource "nodeping_check" "redis" {
  type         = "REDIS"
  label        = "Redis Sentinel"
  redistype    = "sentinel"
  sentinelname = "mymaster"

  hosts = {
    node1 = {
      host        = "redis1.example.com"
      port        = 26379
    }
    node2 = {
      host = "redis2.example.com"
      port = 26379
    }
  }
}
```

#### CLUSTER

```hcl
resource "nodeping_check" "cluster" {
  type  = "CLUSTER"
  label = "Web tier"

  data = {
    (nodeping_check.web_a.id) = "1"
    (nodeping_check.web_b.id) = "1"
  }
}
```

#### PGSQL field matching

```hcl
resource "nodeping_check" "pgsql" {
  type     = "PGSQL"
  target   = "postgres://db.example.com:5432"
  database = "app"
  query    = "SELECT status, backlog FROM queue_health"

  fields = {
    "1" = {
      name  = "status"
      match = "healthy"
    }
    "2" = {
      name = "backlog"
      min  = 0
      max  = 500
    }
  }
}
```

## Attribute Reference

- `id` - The unique identifier of the check.
- `customer_id` - The customer ID (account ID) that owns this check.
- `state` - Current state: `0` (failing) or `1` (passing).
- `created` - Creation timestamp (milliseconds).
- `modified` - Last modification timestamp (milliseconds).
- `checktoken` - Token used by `AGENT` and `PUSH` checks to submit results. Generated by NodePing and
  read-only.

## Supported Check Types

| Type | Description |
|------|-------------|
| `AGENT` | NodePing Agent check |
| `AUDIO` | Audio stream check |
| `CLUSTER` | Cluster of checks |
| `DNS` | DNS resolution check |
| `DOHDOT` | DNS over HTTPS/TLS |
| `FTP` | FTP server check |
| `HTTP` | HTTP/HTTPS check |
| `HTTPADV` | Advanced HTTP check |
| `HTTPCONTENT` | HTTP content match |
| `HTTPPARSE` | HTTP response parsing |
| `IMAP4` | IMAP mail server |
| `MONGODB` | MongoDB database |
| `MTR` | MTR traceroute |
| `MYSQL` | MySQL database |
| `NTP` | NTP time server |
| `PGSQL` | PostgreSQL database |
| `PING` | ICMP ping |
| `POP3` | POP3 mail server |
| `PORT` | TCP port check |
| `PUSH` | Push-based check |
| `RBL` | Real-time Blacklist |
| `RDAP` | RDAP domain lookup |
| `RDP` | Remote Desktop |
| `REDIS` | Redis database |
| `SIP` | SIP VoIP check |
| `SMTP` | SMTP mail server |
| `SNMP` | SNMP check |
| `SPEC10DNS` | SPEC10 DNS |
| `SPEC10RDDS` | SPEC10 RDDS |
| `SSH` | SSH server |
| `SSL` | SSL certificate |
| `WEBSOCKET` | WebSocket check |
| `WHOIS` | WHOIS domain lookup |

## Import

Checks can be imported using the check ID:

```shell
terraform import nodeping_check.example 201205050153W2Q4C-0J2HSIRF
```

For SubAccount checks, use the format `customer_id:check_id`:

```shell
terraform import nodeping_check.example 201205050153W2Q4C:201205050153W2Q4C-0J2HSIRF
```

## Notes

- Check IDs are generated by NodePing and cannot be set manually.
- Sub-minute intervals (0.25 and 0.5) may incur additional fees.
- The `dep` (dependency) feature prevents notifications when the dependent check is failing.
- Importing a check never brings a credential with it: `password_wo`, `password`, and any
  `hosts[*].password` are left null so that the plaintext NodePing returns cannot land in state.
