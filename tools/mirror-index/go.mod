// Separate module so the build tooling's dependencies stay out of the
// provider's own dependency graph.
module github.com/atlanticbt/terraform-provider-nodeping/tools/mirror-index

go 1.25

require golang.org/x/mod v0.29.0
