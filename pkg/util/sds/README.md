# pkg/util/sds

This package wraps the [Datadog Sensitive Data Scanner (SDS)](https://github.com/DataDog/dd-sensitive-data-scanner)
shared library and exposes a small scanner API: create a scanner, configure it
with rules and scan events (e.g. to redact sensitive data).

## Build tags

The package has two implementations selected at compile time:


| Build            | File               | Behaviour                                                                                                  |
| ---------------- | ------------------ | ---------------------------------------------------------------------------------------------------------- |
| default (no tag) | `scanner_nosds.go` | no-op **mock**: `Scan` returns the event unchanged, `IsReady()` is always `false`. No external dependency. |
| `-tags sds`      | `scanner.go`       | the **real** scanner, linked against the `libdd_sds_go` shared library.                                    |


`SDSEnabled` reflects which one is compiled in.

The real scanner needs the SDS shared library at link time, which is why it is
gated behind the `sds` build tag. Without the tag, the Agent compiles and runs
with the mock.

## Running the tests

### Default (mock) — no shared library needed

```bash
cd pkg/util/sds
go test ./...
```

Only `rules_test.go` runs here (`scanner_test.go` is behind `//go:build sds`).

### With the real scanner (`-tags sds`)

`scanner_test.go` exercises the real scanner and therefore requires the
`libdd_sds_go` shared library to be built and reachable by the linker.

1. Find the SDS Go module version used by this package:
  ```bash
   grep dd-sensitive-data-scanner go.mod
   # github.com/DataDog/dd-sensitive-data-scanner/sds-go/go v0.0.0-20250908201838-4d0ef6614dd4
  ```
   The trailing hash (`4d0ef6614dd4`) is the commit of the
   `dd-sensitive-data-scanner` repository.
2. Build the Rust shared library from that repo (needs the Rust toolchain /
  `cargo`):
3. Point the linker at the built library and run the tests with the tag:
  ```bash
   export CGO_LDFLAGS="-L/absolute/path/to/dd-sensitive-data-scanner/sds-go/rust/target/release"
   cd pkg/util/sds
   go test -tags sds ./...
  ```
  > The package's cgo directives also reference a relative `-L../rust/target/release`
  > path inside the Go module cache; you can ignore the
  > `ld: warning: search path ... not found` warning for it — the linker uses the
  > absolute path provided through `CGO_LDFLAGS`.

To only type-check the `sds` build without linking the shared library:

```bash
cd pkg/util/sds
go vet -tags sds ./...
```

## Usage

```go
import "github.com/DataDog/datadog-agent/pkg/util/sds"

s := sds.CreateScanner()

// configure standard rule definitions (raw JSON), then user rules.
_ = s.Reconfigure(sds.ReconfigureOrder{Type: sds.StandardRules, Config: standardRulesJSON})
_ = s.Reconfigure(sds.ReconfigureOrder{Type: sds.AgentConfig, Config: userRulesJSON})

if s.IsReady() {
    mutated, processed, err := s.Scan([]byte("one two three go!"))
    _ = mutated   // true if the event was modified (e.g. redacted)
    _ = processed // processed event, nil when not mutated
    _ = err
}
```

A process-wide default scanner is also available through the package-level
`sds.DefaultScanner()`, `sds.Reconfigure(...)` and `sds.Scan(...)` helpers. These
back the `datadog_agent.scan(event)` method exposed to Python integrations.