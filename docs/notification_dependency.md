# Notification Dependency (dep)

## Overview

The `dep` (notification dependency) attribute allows you to create hierarchical monitoring relationships where notifications for a check are suppressed when a dependent check is failing. This is particularly useful for reducing alert noise when core infrastructure fails.

## Use Cases

### 1. Network Infrastructure Dependencies

When a router or network device fails, all services behind it will also fail. Instead of receiving alerts for every service, you can suppress notifications for dependent services when the router is down.

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

### 2. Database Dependencies

Application checks can depend on database availability:

```hcl
resource "nodeping_check" "database" {
  type     = "PORT"
  target   = "db.example.com"
  label    = "PostgreSQL Database"
  enabled  = true
  port     = 5432
}

resource "nodeping_check" "api" {
  type    = "HTTP"
  target  = "https://api.example.com/health"
  label   = "API Health Check"
  enabled = true
  dep     = nodeping_check.database.id
}
```

### 3. Multi-Tier Application Stack

Create a dependency chain for complex applications:

```hcl
# Layer 1: Network
resource "nodeping_check" "vpn" {
  type    = "PING"
  target  = "vpn.example.com"
  label   = "VPN Gateway"
  enabled = true
}

# Layer 2: Database (depends on network)
resource "nodeping_check" "database" {
  type    = "PGSQL"
  target  = "db.internal.example.com"
  label   = "Database"
  enabled = true
  dep     = nodeping_check.vpn.id
}

# Layer 3: Application (depends on database)
resource "nodeping_check" "app" {
  type    = "HTTP"
  target  = "https://app.internal.example.com"
  label   = "Application"
  enabled = true
  dep     = nodeping_check.database.id
}
```

## Behavior

### When Dependency Check is Passing
- Dependent check operates normally
- Notifications are sent based on the check's own status

### When Dependency Check is Failing
- Dependent check continues to run
- **No notifications are sent** for the dependent check
- Check status is still updated in NodePing

### When Dependency Check Recovers
- Dependent check resumes normal notification behavior
- If dependent check is still failing, notifications will be sent

## Configuration

### Setting a Dependency

```hcl
resource "nodeping_check" "dependent" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Dependent Check"
  enabled = true
  dep     = nodeping_check.primary.id  # Reference to primary check
}
```

### Removing a Dependency

To remove a dependency, simply remove the `dep` attribute:

```hcl
resource "nodeping_check" "dependent" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Dependent Check"
  enabled = true
  # dep attribute removed - check is now independent
}
```

### Changing a Dependency

Update the `dep` attribute to reference a different check:

```hcl
resource "nodeping_check" "dependent" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Dependent Check"
  enabled = true
  dep     = nodeping_check.new_primary.id  # Changed dependency
}
```

## Reading Dependency Information

### Using Data Sources

You can read the `dep` value from existing checks:

```hcl
data "nodeping_check" "existing" {
  id = "201205050153W2Q4C-0J2HSIRF"
}

output "dependency_check_id" {
  value = data.nodeping_check.existing.dep
}
```

### Listing Checks with Dependencies

```hcl
data "nodeping_checks" "all" {}

output "checks_with_dependencies" {
  value = [
    for check in data.nodeping_checks.all.checks :
    check.id if check.dep != null && check.dep != ""
  ]
}
```

## Best Practices

### 1. Avoid Circular Dependencies

❌ **Don't do this:**
```hcl
resource "nodeping_check" "a" {
  type = "HTTP"
  target = "https://a.example.com"
  dep = nodeping_check.b.id
}

resource "nodeping_check" "b" {
  type = "HTTP"
  target = "https://b.example.com"
  dep = nodeping_check.a.id  # Circular dependency!
}
```

### 2. Use for Infrastructure, Not Application Logic

✅ **Good use case:** Router → Services behind router
❌ **Poor use case:** Microservice A → Microservice B (application-level dependencies)

### 3. Keep Dependency Chains Short

✅ **Recommended:** 1-2 levels deep
❌ **Not recommended:** 5+ levels deep

### 4. Document Your Dependencies

```hcl
# Core infrastructure check - all internal services depend on this
resource "nodeping_check" "core_router" {
  type    = "PING"
  target  = "10.0.0.1"
  label   = "Core Router - Primary Dependency"
  enabled = true
}

# Internal service - depends on core router
resource "nodeping_check" "internal_api" {
  type    = "HTTP"
  target  = "https://api.internal.example.com"
  label   = "Internal API"
  enabled = true
  dep     = nodeping_check.core_router.id  # Suppress alerts when router is down
}
```

## Limitations

1. **Single Dependency Only**: Each check can only have one `dep` value
2. **No Automatic Validation**: Terraform doesn't validate that the referenced check exists in NodePing
3. **API Limitation**: The NodePing API controls the actual dependency behavior

## Troubleshooting

### Dependency Not Working

1. Verify the check ID is correct:
   ```bash
   terraform state show nodeping_check.primary
   ```

2. Check that both checks are in the same account/subaccount

3. Verify the dependency check is enabled

### Import Existing Checks with Dependencies

When importing checks that have dependencies:

```bash
terraform import nodeping_check.dependent 201205050153W2Q4C-0J2HSIRF
```

The `dep` value will be automatically imported from the API.

## API Reference

For more information, see the [NodePing API documentation](https://nodeping.com/docs-api-checks.html) under the `dep` parameter.
