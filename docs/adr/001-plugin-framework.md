# ADR-001: Use Terraform Plugin Framework

**Status**: Accepted  
**Date**: 2026-01-27  
**Decision Makers**: Terraform Provider Engineering

## Context

We need to choose between two approaches for building the NodePing Terraform provider:

1. **Terraform Plugin SDK v2** - The older, more established SDK
2. **Terraform Plugin Framework** - The newer, recommended approach

## Decision

We will use the **Terraform Plugin Framework** (`github.com/hashicorp/terraform-plugin-framework`).

## Rationale

### Advantages of Plugin Framework

1. **Official Recommendation**: HashiCorp recommends the Plugin Framework for all new providers
2. **Better Type Safety**: Strongly-typed schema definitions reduce runtime errors
3. **Improved Developer Experience**: More intuitive API design
4. **Future-Proof**: Active development and new features
5. **Better Diagnostics**: Improved error handling and reporting
6. **Native Protocol 6 Support**: Modern Terraform protocol support

### Disadvantages

1. **Fewer Examples**: Less community documentation compared to SDK v2
2. **Learning Curve**: Team may need to learn new patterns

### Mitigations

- Use official HashiCorp provider examples as reference
- Follow terraform-plugin-framework documentation closely
- Start with simpler resources (contact) before complex ones (check)

## Consequences

- All resources and data sources will use Plugin Framework patterns
- Provider configuration will use `provider.Provider` interface
- Schema definitions will use `schema.Schema` types
- CRUD operations will implement `resource.Resource` interface

## References

- [Terraform Plugin Framework Documentation](https://developer.hashicorp.com/terraform/plugin/framework)
- [Plugin Framework vs SDK Comparison](https://developer.hashicorp.com/terraform/plugin/framework-benefits)
