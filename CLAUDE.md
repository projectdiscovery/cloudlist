# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Cloudlist** is a multi-cloud asset discovery tool written in Go that enumerates resources (IPs, DNS names) from various cloud providers using their APIs. It's designed for blue team security operations to maintain centralized cloud asset inventories.

## Build & Development Commands

### Building
```bash
# Build the binary
make build
# or
go build -v -o cloudlist cmd/cloudlist/main.go
```

### Testing
```bash
# Run all tests
make test
# or
go test -v ./...

# Run tests for specific package
go test -v ./pkg/providers/aws/
```

### Linting
```bash
# Run golangci-lint (same as CI)
golangci-lint run --timeout 5m

# Auto-fix issues
golangci-lint run --fix --timeout 5m
```

### Running
```bash
# Basic usage
./cloudlist -pc ~/.config/cloudlist/provider-config.yaml

# Filter by provider
./cloudlist -pc provider-config.yaml -p aws,gcp

# Filter by service
./cloudlist -pc provider-config.yaml -s compute,dns

# Output formats
./cloudlist -pc provider-config.yaml -json > output.json
./cloudlist -pc provider-config.yaml -host  # DNS names only
./cloudlist -pc provider-config.yaml -ip    # IPs only
```

## Architecture

### Core Components

1. **Provider Interface** (`pkg/schema/schema.go:18-28`)
   - All providers implement the `schema.Provider` interface
   - Key methods: `Name()`, `ID()`, `Resources(ctx)`, `Services()`
   - Returns `*schema.Resources` containing `[]*schema.Resource`

2. **Provider Registration** (`pkg/inventory/inventory.go:92-137`)
   - `nameToProvider()` function maps provider names to implementations
   - Add new providers here as a case statement

3. **Resource Model** (`pkg/schema/schema.go:142-163`)
   - Each resource represents a cloud asset with IP/DNS information
   - Fields: `Public`, `Provider`, `Service`, `ID`, `PublicIPv4/v6`, `PrivateIpv4/v6`, `DNSName`, `Metadata`
   - Resources are automatically deduplicated by IP/DNS values

4. **Provider Structure Pattern**
   - Providers in `pkg/providers/<provider>/`
   - Main file: `<provider>.go` with `New()` constructor and `Resources()` implementation
   - Service-specific files: `instances.go`, `dns.go`, etc. for complex providers (see AWS)
   - Each provider has a `Services` slice listing supported service types

### Provider Implementation Pattern

When implementing providers, follow this structure:

1. **Main provider file** (`<provider>.go`):
   - Struct with provider-specific client and configuration
   - `New(block schema.OptionBlock)` constructor that validates config
   - `Resources(ctx context.Context) (*schema.Resources, error)` that orchestrates resource gathering
   - Service-specific helper methods

2. **Service-specific files** (for complex providers):
   - AWS example: `instances.go`, `route53.go`, `s3.go`, `eks.go`, etc.
   - Each file handles one service type's resource enumeration
   - Returns `*schema.Resources` that main `Resources()` method merges

3. **Configuration Parsing**:
   - Use `block.GetMetadata(key)` to read config values
   - Environment variables supported via `$VAR_NAME` syntax (auto-resolved)
   - Return `&schema.ErrNoSuchKey{Name: key}` for missing required config

### Important Patterns

- **Resource Deduplication**: Resources are automatically deduplicated by the `ResourceDeduplicator` in schema
- **Service Filtering**: Users can filter by service via `-s` flag; providers check this in `Services()` method
- **Metadata**: Extended metadata can be added to resources via the `Metadata` map[string]string field
- **Context Handling**: All provider `Resources()` methods receive context for cancellation support

## Adding a New Provider

Required steps:

1. **Create provider directory**: `pkg/providers/<provider-name>/`

2. **Implement provider struct** with these methods:
   ```go
   func New(block schema.OptionBlock) (schema.Provider, error)
   func (p *Provider) Name() string
   func (p *Provider) ID() string
   func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error)
   func (p *Provider) Services() []string
   ```

3. **Register in inventory**: Add case to `nameToProvider()` in `pkg/inventory/inventory.go`

4. **Add to services map**: Add provider and its services to `Providers` map in `pkg/inventory/inventory.go`

5. **Document configuration**: Add provider config documentation to `PROVIDERS.md`

See reference implementations:
- Simple: `pkg/providers/digitalocean/` (single service)
- Complex: `pkg/providers/aws/` (multiple services, sub-files)
- With special auth: `pkg/providers/gcp/` (JSON service account keys)

## Key Files

- `cmd/cloudlist/main.go` - CLI entry point
- `internal/runner/runner.go` - Main enumeration orchestrator
- `internal/runner/options.go` - CLI flag definitions
- `pkg/schema/schema.go` - Core interfaces and data structures
- `pkg/inventory/inventory.go` - Provider registration and factory
- `pkg/providers/*/` - Individual cloud provider implementations

## GCP-Specific Notes

GCP provider supports **two discovery modes**:

1. **Individual Service APIs** (default): Project-level discovery using service-specific APIs
2. **Organization-Level Asset API**: Org-wide discovery when `organization_id` is specified

Key distinction in code:
- Check for `organization_id` in config to determine mode
- Asset API mode uses `cloud.google.com/go/asset` package
- Individual APIs mode uses service-specific clients (compute, dns, etc.)

See `docs/GCP_ASSET_API.md` for detailed implementation guide.

## Testing Guidelines

- Integration tests require cloud provider credentials (usually skipped in CI)
- Unit tests should mock cloud provider clients
- Use table-driven tests for resource parsing logic
- Test error handling for missing/invalid credentials

## GitHub Actions CI

- **lint-test.yml**: Runs `golangci-lint` on Go code changes
- **build-test.yml**: Tests builds on Ubuntu, Windows, macOS with Go 1.22.x
- **release-binary.yml**: Creates releases with GoReleaser

## Common Gotchas

1. **Resource fields**: Either IP or DNS must be populated; empty resources are invalid
2. **Provider IDs**: The `id` field in config is user-defined for filtering, not a cloud resource ID
3. **Service names**: Must match entries in provider's `Services` slice and inventory's `Providers` map
4. **Credential handling**: Use environment variables (`$VAR`) in config rather than hardcoding
5. **Context cancellation**: Always respect context in long-running API calls
6. **Deduplication**: Don't manually deduplicate; use `Resources.Append()` which handles it automatically

## Module Information

- Go version: 1.24+ (see `go.mod`)
- Module path: `github.com/projectdiscovery/cloudlist`
- Key dependencies:
  - Provider SDKs: `aws-sdk-go`, `azure-sdk-for-go`, `google.golang.org/api`, etc.
  - ProjectDiscovery libs: `goflags`, `gologger`, `utils`
  - Concurrency: `github.com/alitto/pond/v2` for worker pools
