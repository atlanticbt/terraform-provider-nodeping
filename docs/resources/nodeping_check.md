---
page_title: "nodeping_check Resource - terraform-provider-nodeping"
subcategory: ""
description: |-
  Manages a NodePing monitoring check.
---
<!--
doc: registry-resource
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# nodeping_check (Resource)

Manages a NodePing check. A check tests a service at a fixed interval.
When the result changes from pass to fail, or from fail to pass, NodePing sends notifications to the addresses and contact groups that you list.
The provider supports all 33 NodePing check types.

> **WARNING:** A change to `customer_id` replaces the check. OpenTofu deletes the live check and creates a new one, so the result history of the old check is lost.

> **SECURITY:** Use `password_wo` for check credentials. The deprecated `password` attribute writes the plaintext value to the state file. Also enable OpenTofu state encryption.

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
  target      = "https://example.com"
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

`contact_id` takes a contact **address** ID or a contact group ID. It does not take a contact ID.
The `nodeping_contact` resource gives each address ID as `address[*].id`.

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
    contact_id = nodeping_contact.ops.address[0].id
    delay      = 0
    schedule   = "All"
  }

  notifications {
    contact_id = nodeping_contactgroup.escalation.id
    delay      = 15
    schedule   = "All"
  }
}
```

### Check with Dependency

When the router check fails, NodePing sends no notifications for the service check.
Refer to [notification dependency](../notification_dependency.md).

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

### FTP Check with a Write-Only Password

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

Refer to [Write-Only Passwords](#write-only-passwords) for the rotation procedure.

### REDIS Sentinel

```hcl
resource "nodeping_check" "redis" {
  type         = "REDIS"
  target       = "redis://redis1.example.com:26379"
  label        = "Redis Sentinel"
  redistype    = "sentinel"
  sentinelname = "mymaster"

  hosts = {
    node1 = {
      host = "redis1.example.com"
      port = 26379
    }
    node2 = {
      host = "redis2.example.com"
      port = 26379
    }
  }
}
```

### CLUSTER

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

### PGSQL Field Matching

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

## Schema

### Required

- `type` (String) The check type. Must be one of the values in [Supported Check Types](#supported-check-types). The value is case-sensitive. A change to `type` updates the check in place.

### Optional

The provider validates the attributes below at plan time.
Some attributes apply only to some check types. Refer to [Type Applicability](#type-applicability).

#### General

- `target` (String) The URL, host name, or IP address that the check tests. At most 2048 characters. Required for all check types except `AGENT`, `CLUSTER`, `DNS`, `PUSH`, `SPEC10DNS` and `SPEC10RDDS`. Refer to [Target Normalization](#target-normalization).
- `label` (String) The display label. At most 1024 characters. If you do not set it, NodePing uses the target.
- `enabled` (Boolean) Set to `true` to run the check. Default `false`.
- `public` (Boolean) Set to `true` to enable public reports. Default `false`.
- `interval` (Number) The time between two runs, in minutes. Must be `0.25`, `0.5`, or a whole number of 1 or more. Default `15`. For `WHOIS` and `RDAP`, the default is `1440`. Refer to [Interval Minimums](#interval-minimums).
- `threshold` (Number) The timeout, in seconds. Must be 1 or more. Default `5`.
- `sens` (Number) The number of rechecks before the status changes. Must be 0 or more. `0` changes the status on the first failed result. Default `2`.
- `mute` (Boolean) Set to `true` to mute all notifications. Default `false`.
- `dep` (String) The ID of the parent check. When the parent check fails, NodePing sends no notifications for this check. Refer to [notification dependency](../notification_dependency.md).
- `description` (String) Notes about the check. At most 1000 characters.
- `autodiag` (Boolean) Set to `true` to enable automated diagnostics. Default `false`.
- `customer_id` (String) The customer ID of the subaccount that owns the check. If you do not set it, the provider uses its own `customer_id`, or the account of the API token. Must not be an empty string. Changing it forces replacement.

#### Locations

- `runlocations` (List of String) The locations that run the check. Each entry must be one of these:
  - A region code: `nam`, `lam`, `eur`, `eao` or `wlw`. The match ignores letter case.
  - A two-letter probe code, for example `ca` or `ny`.
  - The ID of an `AGENT` check.

  If you do not set it, the value comes from NodePing.
- `homeloc` (String) The preferred probe location. Set `roam` to rotate between locations. If you do not set it, the value comes from NodePing.

#### Tags

- `tags` (Set of String) The tags that your configuration declares. Each tag must not be an empty string. The provider `default_tags` are not in this set.

Refer to [Tags and Default Tags](#tags-and-default-tags).

#### Notifications

- `notifications` (Block List) One block for each notification target. Refer to [Nested Schema for `notifications`](#nested-schema-for-notifications).

#### Content Matching

- `contentstring` (String) The text to find in the response. At most 16384 characters.
- `regex` (Boolean) Set to `true` to read `contentstring` as a regular expression.
- `invert` (Boolean) Set to `true` to fail the check when the response contains `contentstring`.

#### HTTP

- `follow` (Boolean) Set to `true` to follow redirects, up to 4.
- `method` (String) The HTTP method for `HTTPADV`. Must be `GET`, `POST`, `PUT`, `HEAD`, `TRACE` or `CONNECT`.
- `statuscode` (Number) The expected HTTP status code. Must be `0` or between `100` and `599`. NodePing returns `0` on some existing `HTTPADV` checks.
- `sendheaders` (Map of String) The request headers to send. This attribute is not marked sensitive. Refer to [Request Headers](#request-headers).
- `receiveheaders` (Map of String) The response headers to expect.
- `postdata` (String) The request body for `HTTPADV`. At most 65536 characters.
- `ipv6` (Boolean) Set to `true` to use IPv6.

#### DNS

- `dnstype` (String) The DNS query type. Must be `ANY`, `A`, `AAAA`, `CNAME`, `MX`, `NS`, `PTR`, `SOA`, `SRV` or `TXT`.
- `dnstoresolve` (String) The FQDN to resolve.
- `dnssection` (String) The section of the DNS reply to check. Must be `answer`, `authority`, `additional` or `edns_options`.
- `dnsrd` (Boolean) The Recursion Desired bit. If you do not set it, the value comes from NodePing.
- `transport` (String) The transport protocol for `DNS` and `SIP` checks. Must be `udp`, `tcp`, `tls`, `ws` or `wss`.
- `dohdot` (String) The protocol for `DOHDOT` checks: `doh` (DNS over HTTPS) or `dot` (DNS over TLS). Default `doh`. The provider sends it only for `DOHDOT` checks.
- `edns` (Map of String) EDNS options for `DOHDOT` checks. Each key must be a positive integer option code.

#### SSL and TLS

- `warningdays` (Number) The number of days before the certificate or domain expires that the check fails. The provider enforces no minimum, because NodePing does not document one. NodePing accepts and keeps `0`.
- `servername` (String) The server name for SNI.
- `verify` (Boolean) Set to `true` to verify the SSL certificate or DNSSEC.
- `secure` (String) The SSL mode. Must be `false`, `ssl` or `starttls`.
- `clientcert` (String) The ID of a client certificate, for `HTTPADV` and `DOHDOT` checks.

#### Authentication

- `username` (String) The user name for authentication.
- `password_wo` (String, Sensitive, Write-only) The password for authentication. The provider sends it to NodePing. OpenTofu does not write it to state or to a saved plan. Conflicts with `password`. Refer to [Write-Only Passwords](#write-only-passwords).
- `password_wo_version` (Number) Increment this value to send `password_wo` again. A plan error occurs if you set it without `password_wo`.
- `password` (String, Sensitive, **Deprecated**) The password for authentication. OpenTofu writes this value to state in plaintext. Use `password_wo` instead. Conflicts with `password_wo`.
- `email` (String) The email address that `SMTP` checks use to verify mail delivery.
- `sshkey` (String) The ID of an SSH private key, for `SSH` checks.

#### Network

- `port` (Number) The port number. Must be between `1` and `65535`. Required for PORT and NTP checks.

#### Database

- `database` (String) The database name.
- `query` (String) The query to run.
- `namespace` (String) The MongoDB collection namespace.

#### SNMP

- `snmpv` (String) The SNMP version. Must be `1` or `2c`.
- `snmpcom` (String, Sensitive) The SNMP community string. The provider reads it back from NodePing, so the value is in state.

#### Type-Specific

- `data` (Map of String) Settings for some check types:
  - `CLUSTER`: each key is a member check ID. The value is `"1"` (must pass) or `"0"` (must fail).
  - `HTTPADV`: form data to send with the request.
  - `WEBSOCKET`, `SPEC10DNS` and `SPEC10RDDS`: the settings of that type.
- `fields` (Attributes Map) The result fields to parse and range check, for `HTTPPARSE`, `SNMP`, `MONGODB`, `MYSQL` and `PGSQL`. The map key is the field ID in the NodePing results. Refer to [Nested Schema for `fields`](#nested-schema-for-fields).
- `hosts` (Attributes Map) The `REDIS` hosts, keyed by an identifier that you choose. Refer to [Nested Schema for `hosts`](#nested-schema-for-hosts).
- `redistype` (String) The Redis deployment type. Must be `standalone`, `sentinel` or `cluster`. Default `standalone`. The provider sends it only for `REDIS` checks.
- `sentinelname` (String) The Redis Sentinel master name. Required when `redistype` is `sentinel`.
- `oldresultfail` (Boolean) Set to `true` to fail an `AGENT` or `PUSH` check when its results are older than the check interval.
- `whoisserver` (String) The WHOIS server to query. For `WHOIS` only.
- `rdapurl` (String) The RDAP service URL. Must be an absolute `http` or `https` URL. For `RDAP` only.
- `ignore` (String) A comma-separated list of values to ignore. For `RBL`, the values are blocklists. For `SIP`, the values are response codes.
- `verifyvolume` (Boolean) Set to `true` to enable volume detection. For `AUDIO` only.
- `volumemin` (Number) The minimum volume, in dB. Must be between `-90` and `0`. For `AUDIO` only.

### Read-Only

- `id` (String) The check ID. NodePing generates it. You cannot set it.
- `tags_all` (Set of String) The tags that NodePing keeps on the check: `tags` plus the provider `default_tags`, without duplicates.
- `state` (Number) The current state: `0` means failing, `1` means passing.
- `created` (Number) The creation time, in milliseconds since the Unix epoch.
- `modified` (Number) The time of the last change, in milliseconds since the Unix epoch.
- `checktoken` (String, Sensitive) The token that `AGENT` and `PUSH` checks use to submit results. NodePing generates it.

### Nested Schema for `notifications`

Required:

- `contact_id` (String) The address ID of a contact (`nodeping_contact.x.address[N].id`) or a contact group ID. Do not use a contact ID.

Optional:

- `delay` (Number) The time to wait before NodePing sends the notification, in minutes. Default `0`.
- `schedule` (String) The name of the schedule. If you do not set it, the value comes from NodePing.

### Nested Schema for `fields`

Required:

- `name` (String) The name of the field in the parsed result.

Optional:

- `min` (Number) The minimum value that passes.
- `max` (Number) The maximum value that passes.
- `match` (String) The text that the field must match. Only `PGSQL`, `MYSQL` and `MONGODB` use it.

### Nested Schema for `hosts`

Required:

- `host` (String) The host name or IP address of the Redis node.

Optional:

- `port` (Number) The port of the Redis node. Must be between `1` and `65535`.
- `password_wo` (String, Sensitive, Write-only) The password for the node. OpenTofu does not write it to state or to a saved plan. Conflicts with `password` on the same host.
- `password_wo_version` (Number) Increment this value to send `password_wo` for this host again. A plan error occurs if you set it without `password_wo` on the same host.
- `password` (String, Sensitive, **Deprecated**) The password for the node. OpenTofu writes this value to state in plaintext. Use `password_wo` instead.

## Import

> **SECURITY:** NodePing returns check passwords in plaintext. The provider does not copy them into state. After an import, `password`, `password_wo` and every `hosts[*].password` are null.

Import a check with its check ID:

```shell
tofu import nodeping_check.example 201205050153W2Q4C-0J2HSIRF
```

For a check in a subaccount, use the format `customer_id:check_id`:

```shell
tofu import nodeping_check.example 201205050153W2Q4C:201205050153W2Q4C-0J2HSIRF
```

You can also use an `import` block with an `identity`. Set `id`. For a subaccount, also set `customer_id`.

```hcl
import {
  to = nodeping_check.example
  identity = {
    id          = "201205050153W2Q4C-0J2HSIRF"
    customer_id = "201205050153W2Q4C"
  }
}
```

The import writes `customer_id` to state. The provider uses it for all later API calls for this check.
Import with identity requires OpenTofu 1.12 or later.

To manage the credential after an import, add `password_wo` and `password_wo_version` to the configuration.

## Write-Only Passwords

NodePing keeps check passwords in plaintext and returns them on each read.
The provider cannot protect the credential on the NodePing side. It keeps the credential out of your state file.

The provider never copies the password from NodePing into state.
This applies to create, update, refresh and import.
`password_wo` requires OpenTofu 1.11 or later. This provider targets OpenTofu 1.12 or later.

> **CAUTION:** OpenTofu does not store `password_wo`, so it cannot compare the old and new values. A change to `password_wo` alone produces no plan. Increment `password_wo_version` in the same change.

To rotate a password:

1. Set the new value in the variable that `password_wo` reads.
2. Increment `password_wo_version` by 1.
3. Run `tofu apply`.

The same procedure applies to `hosts[*].password_wo` and `hosts[*].password_wo_version` on `REDIS` checks.

State encryption protects the state file at rest. It does not replace write-only attributes.
Commands such as `tofu show` and `tofu state pull` print state values to any operator who can run them.

## Tags and Default Tags

The provider uses two attributes for tags:

| Attribute | Contents |
|---|---|
| `tags` | Only the tags that your configuration declares. |
| `tags_all` | `tags` plus the provider `default_tags`. The provider sends this set to NodePing. |

To use the tags that NodePing keeps, refer to `tags_all` in outputs and other resources.

The provider cannot merge `default_tags` into `tags`.
OpenTofu requires the planned value of an optional argument to match the configuration.
A merged `tags` value fails the plan when a check declares its own tags.
The AWS provider uses the same two-attribute design.

On refresh, the provider reads `tags_all` from NodePing.
It then calculates `tags` as `tags_all` without the configured `default_tags`.
If you declare a tag that is also a default tag, the provider keeps it in `tags`.
Thus the tag does not show as a removal in the next plan.
An import has no prior state, so the provider removes all default tags from `tags`.

## Target Normalization

NodePing changes some URL targets into a canonical form before it stores them.
This applies to `AUDIO`, `HTTP`, `HTTPADV`, `HTTPCONTENT`, `HTTPPARSE`, `MONGODB`, `PGSQL`, `REDIS` and `SSL` checks.

NodePing makes these changes:

- It changes the scheme and host to lower case.
- It adds `/` to an empty path.

For example, NodePing stores `https://EXAMPLE.com` as `https://example.com/`.
The provider recognizes these changes and does not show them as a difference. Thus both spellings are stable in your configuration.

The provider removes leading and trailing spaces from the target for all check types.
Other types keep the target exactly as you write it.
`WEBSOCKET`, `FTP`, `RDAP`, `SIP` and `WHOIS` accept a URL, but NodePing does not change it.

The provider does not copy two NodePing changes: punycode for a non-ASCII host, and percent-encoding of non-ASCII path bytes.
For such a target, the first plan shows a difference. The difference goes away after one apply.

After an import, state holds the canonical target that NodePing returns.
If your configuration has a different spelling, the next plan is still empty.

## Interval Minimums

NodePing replaces a `WHOIS` interval below 240 minutes with 1440.
It replaces an `RDAP` interval below 60 minutes with 1440.

> **CAUTION:** Such a value never matches state, so the plan always shows a change. The provider stops it with a plan error.

If you do not set `interval` for a `WHOIS` or `RDAP` check, the provider plans 1440.

## Type Applicability

Some attributes apply only to some check types.
If you set one on a different type, the provider reports it at plan time.

| Attributes | Check types | Result on a different type |
|---|---|---|
| `contentstring` | `HTTPCONTENT`, `HTTPADV`, `HTTPPARSE`, `DNS`, `DOHDOT` | Warning |
| `regex`, `invert` | `HTTPCONTENT`, `HTTPADV`, `HTTPPARSE`, `DNS`, `DOHDOT` | Error if `true` |
| `dnstype`, `dnstoresolve`, `dnssection` | `DNS`, `DOHDOT` | Warning |
| `dnsrd` | `DNS`, `DOHDOT` | Error if `true` |
| `warningdays` | `SSL`, `WHOIS`, `RDAP`, `SMTP`, `IMAP4`, `POP3`, `DOHDOT`, `SPEC10RDDS` | Warning |

The boolean flags accept `false` on all types.
NodePing stores them as `false` on each check type and returns them on read.
Without this rule, an imported `PUSH` check does not validate.

The other attributes cause only a warning.
NodePing keeps these values when a check changes type, and it returns them on read.
An error makes such checks impossible to import.

`SPEC10DNS` checks keep none of the four DNS attributes.

## Request Headers

`sendheaders` is not marked sensitive. Its values show in plan output and in generated configuration.
This is intentional. `tofu plan -generate-config-out` writes a sensitive map as `null`. An apply of that output deletes the live headers.

> **SECURITY:** If a header contains a credential, encrypt the state. Limit who can read the plan output.

## Supported Check Types

| Type | Description | `target` required |
|------|-------------|-------------------|
| `AGENT` | NodePing Agent check | No |
| `AUDIO` | Audio stream check | Yes |
| `CLUSTER` | Cluster of checks | No |
| `DNS` | DNS resolution check | No |
| `DOHDOT` | DNS over HTTPS or TLS | Yes |
| `FTP` | FTP server check | Yes |
| `HTTP` | HTTP or HTTPS check | Yes |
| `HTTPADV` | Advanced HTTP check | Yes |
| `HTTPCONTENT` | HTTP content match | Yes |
| `HTTPPARSE` | HTTP response parsing | Yes |
| `IMAP4` | IMAP mail server | Yes |
| `MONGODB` | MongoDB database | Yes |
| `MTR` | MTR traceroute | Yes |
| `MYSQL` | MySQL database | Yes |
| `NTP` | NTP time server | Yes |
| `PGSQL` | PostgreSQL database | Yes |
| `PING` | ICMP ping | Yes |
| `POP3` | POP3 mail server | Yes |
| `PORT` | TCP port check | Yes |
| `PUSH` | Push-based check | No |
| `RBL` | Real-time blocklist | Yes |
| `RDAP` | RDAP domain lookup | Yes |
| `RDP` | Remote Desktop | Yes |
| `REDIS` | Redis database | Yes |
| `SIP` | SIP VoIP check | Yes |
| `SMTP` | SMTP mail server | Yes |
| `SNMP` | SNMP check | Yes |
| `SPEC10DNS` | SPEC10 DNS | No |
| `SPEC10RDDS` | SPEC10 RDDS | No |
| `SSH` | SSH server | Yes |
| `SSL` | SSL certificate | Yes |
| `WEBSOCKET` | WebSocket check | Yes |
| `WHOIS` | WHOIS domain lookup | Yes |

## Notes

- **Zero values:** The provider sends a `0` in `sens`, `statuscode`, `warningdays` or `volumemin` as the string `"0"`. NodePing reads a numeric `0` as empty.
- **Length limits:** NodePing enforces no length limit on `label`, `target`, `contentstring` or `postdata`. The provider sets its own limits, far above the values in typical use.
- **Fees:** Sub-minute intervals (`0.25` and `0.5`) can incur additional fees.
- **Deleted outside OpenTofu:** If NodePing cannot find the check on refresh, the provider removes it from state. The next plan creates it again.
- **Failed create:** If NodePing creates the check but a later step fails, the provider writes the check ID to state. The next apply or destroy can then manage the check.
