.PHONY: build test test-acc cover lint fmt vet vulncheck tidy install clean help

BINARY      := terraform-provider-nodeping
VERSION     ?= dev
PROVIDER_NS := atlanticbt.com/abt/nodeping

# Local dev install path for the OpenTofu filesystem mirror.
MIRROR_DIR  := $(HOME)/.local/share/opentofu/plugins/$(PROVIDER_NS)/$(VERSION)/$(shell go env GOOS)_$(shell go env GOARCH)

# OpenTofu binary used for acceptance tests. Override to test another build.
TOFU_BIN    ?= $(shell command -v tofu)

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) .

test:
	go test ./... -race -cover

cover:
	go test ./... -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

# Acceptance tests drive a real OpenTofu binary against the live NodePing API.
#
# TF_ACC_TERRAFORM_PATH alone is NOT sufficient. terraform-plugin-testing
# hardcodes host "registry.terraform.io" and tries the legacy "-" namespace when
# building TF_REATTACH_PROVIDERS; OpenTofu rejects "-" under any host other than
# registry.opentofu.org, so init fails with a misleading "Invalid provider
# namespace" error. The two _PROVIDER_ vars below are what make it work.
#
# Without TF_ACC_TERRAFORM_PATH, plugin-testing silently DOWNLOADS HashiCorp
# Terraform and tests the wrong tool. Always set it explicitly.
#
# Requires NODEPING_API_TOKEN for a DEDICATED TEST SUBACCOUNT. These tests
# create and destroy live checks and contacts. Never point them at a client
# subaccount or the main account.
test-acc:
	@test -n "$(TOFU_BIN)" || { echo "tofu not found; set TOFU_BIN"; exit 1; }
	@test -n "$$NODEPING_API_TOKEN" || { echo "NODEPING_API_TOKEN is not set"; exit 1; }
	TF_ACC=1 \
	TF_ACC_TERRAFORM_PATH=$(TOFU_BIN) \
	TF_ACC_PROVIDER_HOST=registry.opentofu.org \
	TF_ACC_PROVIDER_NAMESPACE=hashicorp \
	go test ./... -v -timeout 30m

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.0 run --timeout 5m ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy:
	go mod tidy

# Install into the local OpenTofu filesystem mirror for manual testing.
# Requires a matching provider_installation block in ~/.tofurc — see docs/development.md.
# The destination is removed before the copy. On Apple Silicon a Go binary
# carries an ad-hoc code signature, and copying over an existing file reuses the
# inode with a signature the kernel has already cached, so the replacement is
# SIGKILLed on exec. OpenTofu reports that as "Unrecognized remote plugin
# message ... failed to negotiate the initial go-plugin protocol handshake",
# which points nowhere near the real cause. Only a reinstall is affected.
install: build
	mkdir -p "$(MIRROR_DIR)"
	rm -f "$(MIRROR_DIR)/$(BINARY)_v$(VERSION)"
	cp $(BINARY) "$(MIRROR_DIR)/$(BINARY)_v$(VERSION)"
	@echo "installed to $(MIRROR_DIR)"

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf dist/

help:
	@echo "build      - Build the provider binary"
	@echo "test       - Run unit tests with race detector"
	@echo "cover      - Run tests and write coverage.out/coverage.html"
	@echo "test-acc   - Run acceptance tests against a real tofu binary (needs NODEPING_API_TOKEN)"
	@echo "lint       - Run golangci-lint"
	@echo "fmt        - Format Go code"
	@echo "vet        - Run go vet"
	@echo "vulncheck  - Run govulncheck"
	@echo "tidy       - Run go mod tidy"
	@echo "install    - Build and install into the local OpenTofu filesystem mirror"
	@echo "clean      - Remove build artifacts"
