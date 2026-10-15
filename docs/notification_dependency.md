<!--
doc: guide
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Notification Dependency (`dep`)

## Overview

The `dep` attribute makes one check depend on a different check. When the parent check
fails, NodePing sends no notifications for the dependent check. Use `dep` to decrease alert
noise when core infrastructure fails.

| Term | Meaning |
|---|---|
| Parent check | The check that `dep` refers to. For example, a router check. |
| Dependent check | The check that has the `dep` attribute. For example, a web server behind the router. |

## Behavior

| State of the parent check | Result for the dependent check |
|---|---|
| Passing | The dependent check operates normally. NodePing sends notifications for its own status. |
| Failing | The dependent check continues to run. NodePing updates its status. **NodePing sends no notifications for it.** |
| Recovered | The dependent check sends notifications again. If it still fails, NodePing sends a notification. |

## Examples

### Network infrastructure

A router failure causes all services behind it to fail. Make each service depend on the
router. Then you get one alert for the router, not one alert for each service.

```hcl
resource "nodeping_check" "edge_router" {
  type    = "PING"
  target  = "192.168.1.1"
  label   = "Edge Router"
  enabled = true
}

resource "nodeping_check" "web_server" {
  type    = "HTTP"
  target  = "https://internal.example.com"
  label   = "Web Server"
  enabled = true
  dep     = nodeping_check.edge_router.id
}
```

### Database

An application check can depend on a database check:

```hcl
resource "nodeping_check" "database" {
  type    = "PORT"
  target  = "db.example.com"
  label   = "PostgreSQL Database"
  enabled = true
  port    = 5432
}

resource "nodeping_check" "api" {
  type    = "HTTP"
  target  = "https://api.example.com/health"
  label   = "API Health Check"
  enabled = true
  dep     = nodeping_check.database.id
}
```

### Application with three tiers

Each tier depends on the tier below it:

```hcl
# Tier 1: network
resource "nodeping_check" "vpn" {
  type    = "PING"
  target  = "vpn.example.com"
  label   = "VPN Gateway"
  enabled = true
}

# Tier 2: database, depends on the network
resource "nodeping_check" "database" {
  type    = "PGSQL"
  target  = "db.internal.example.com"
  label   = "Database"
  enabled = true
  dep     = nodeping_check.vpn.id
}

# Tier 3: application, depends on the database
resource "nodeping_check" "app" {
  type    = "HTTP"
  target  = "https://app.internal.example.com"
  label   = "Application"
  enabled = true
  dep     = nodeping_check.database.id
}
```

## Configuration

### Add a dependency

1. Set `dep` to the ID of the parent check:

   ```hcl
   dep = nodeping_check.primary.id
   ```

2. Run `tofu plan`. Make sure that the plan changes only `dep`.
3. Run `tofu apply`.

### Remove a dependency

1. Delete the `dep` line from the check.
2. Run `tofu plan`. Make sure that the plan shows `dep` as removed.
3. Run `tofu apply`. The provider sends the removal to NodePing.

`dep` is an Optional attribute without a computed value. Thus a removal always shows in the
plan.

### Change a dependency

1. Set `dep` to the ID of the new parent check:

   ```hcl
   dep = nodeping_check.new_primary.id
   ```

2. Run `tofu plan`, then `tofu apply`.

## Read dependency information

### One check

```hcl
data "nodeping_check" "existing" {
  id = "201205050153W2Q4C-0J2HSIRF"
}

output "dependency_check_id" {
  value = data.nodeping_check.existing.dep
}
```

### All checks that have a dependency

```hcl
data "nodeping_checks" "all" {}

output "checks_with_dependencies" {
  value = [
    for check in data.nodeping_checks.all.checks :
    check.id if check.dep != null && check.dep != ""
  ]
}
```

## Rules

| Rule | Reason |
|---|---|
| Do not make a circular dependency. | Two checks that depend on each other can hide both failures. |
| Use `dep` for infrastructure, not for application logic. | A router and the services behind it is a good use. Two microservices that call each other is a bad use. |
| Keep dependency chains short: 1 or 2 levels. | A long chain makes it hard to find the real failure. |
| Write a comment for each dependency. | The next engineer must know why an alert did not come. |

Example of a circular dependency. **Do not do this:**

```hcl
resource "nodeping_check" "a" {
  type   = "HTTP"
  target = "https://a.example.com"
  dep    = nodeping_check.b.id
}

resource "nodeping_check" "b" {
  type   = "HTTP"
  target = "https://b.example.com"
  dep    = nodeping_check.a.id # circular
}
```

Example with comments:

```hcl
# Core infrastructure. All internal services depend on this check.
resource "nodeping_check" "core_router" {
  type    = "PING"
  target  = "10.0.0.1"
  label   = "Core Router - Primary Dependency"
  enabled = true
}

# Internal service. NodePing sends no alerts for it when the router is down.
resource "nodeping_check" "internal_api" {
  type    = "HTTP"
  target  = "https://api.internal.example.com"
  label   = "Internal API"
  enabled = true
  dep     = nodeping_check.core_router.id
}
```

## Limitations

| Limitation | Description |
|---|---|
| One dependency only | A check can have one `dep` value only. |
| No existence check | The provider does not make sure that the parent check exists in NodePing. |
| API behavior | NodePing controls the dependency behavior, not the provider. |

## Troubleshooting

### The dependency does not suppress notifications

1. Make sure that the parent check ID is correct:

   ```sh
   tofu state show nodeping_check.primary
   ```

2. Make sure that both checks are in the same account or subaccount.
3. Make sure that the parent check is enabled.

### Import a check that has a dependency

1. Import the check:

   ```sh
   tofu import nodeping_check.dependent 201205050153W2Q4C-0J2HSIRF
   ```

2. The provider reads `dep` from the API. You do not set it manually.

## API reference

See the `dep` parameter in the [NodePing API documentation](https://nodeping.com/docs-api-checks.html).
