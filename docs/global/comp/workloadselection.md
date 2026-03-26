# comp/workloadselection — Workload Selection Component

**Import path:** `github.com/DataDog/datadog-agent/comp/workloadselection/def`
**Team:** injection-platform
**Importers:** `cmd/agent/subcommands/run` (core agent)

## Purpose

`comp/workloadselection` manages APM Single-Step Instrumentation (SSI) workload selection policies. It subscribes to Remote Config to receive org-wide APM policies, merges them in priority order, compiles them to a binary format, and writes the result to a well-known file path where the APM injector reads them.

This allows operators to centrally control which workloads receive automatic APM instrumentation without redeploying the agent or injector.

## Package layout

| Package | Role |
|---|---|
| `comp/workloadselection/def` | Component interface |
| `comp/workloadselection/impl` | Full implementation (RC listener + policy compilation) |
| `comp/workloadselection/fx` | fx `Module()` wiring `impl` |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/workloadselection/def
type Component interface{}
```

The interface carries no exported methods. The component operates entirely through its Remote Config listener, which is registered as an `rctypes.ListenerProvider` side-output from `NewComponent`.

## Implementation details

When `NewComponent` runs, it checks two conditions before registering the RC listener:

1. `apm_config.workload_selection` is `true` in the agent configuration.
2. The `dd-compile-policy` binary is present and executable at `<install_path>/embedded/bin/dd-compile-policy` (Unix) or the equivalent Windows path.

If either condition fails, the RC listener is not registered and the component is effectively a no-op.

**RC listener:** The component subscribes to the `APM_POLICIES` Remote Config product (`state.ProductApmPolicies`). When the backend pushes a config update:

1. Each incoming policy config path is parsed to extract a numeric ordering prefix (format `N.<name>`).
2. All configs are sorted by that prefix (then alphabetically by path for a deterministic tie-break).
3. The sorted `policies` arrays from each JSON config are concatenated into a single merged JSON document.
4. The merged document is passed to `dd-compile-policy` via `exec.Command`, which writes a compiled binary to a temporary file.
5. The temporary file is atomically renamed to `<conf_path>/managed/rc-orgwide-wls-policy.bin`.

When the RC update contains zero configs (policy removed), the binary file is deleted.

The apply-state callback reports `ApplyStateAcknowledged` on success or `ApplyStateError` on any failure to the RC client, so the backend knows whether the policy was applied.

## fx wiring

```go
import workloadselectionfx "github.com/DataDog/datadog-agent/comp/workloadselection/fx"

// In your fx app:
workloadselectionfx.Module()
```

The module requires `log.Component` and `config.Component`. In addition to `workloadselection.Component`, it provides an `rctypes.ListenerProvider` that is automatically picked up by the RC client to register the `APM_POLICIES` subscription.

## Usage

The component does not expose a call interface to other components. Consumers interact with it indirectly:

- **APM injector** reads the compiled binary at the well-known path (`<conf_path>/managed/rc-orgwide-wls-policy.bin`) to determine which workloads should be instrumented.
- **Remote Config backend** pushes policy JSON documents under the `APM_POLICIES` product whenever an operator changes the org-wide APM SSI policy.

The only wiring needed in the agent's fx app is to include the module:

```go
workloadselectionfx.Module()
```

The RC client will discover and register the listener automatically through the `rctypes.ListenerProvider` side-output.

## Notes

- The `dd-compile-policy` binary is a separate tool bundled with the agent installation. If it is absent (e.g. on a minimal install or unsupported platform), the component silently disables itself rather than erroring at startup.
- Policy ordering relies on the numeric prefix convention `N.<policy-name>` embedded in the Remote Config policy ID. Policies without a numeric prefix are assigned order 0 and sorted alphabetically among themselves.
- The output file write is atomic (write to temp file, then `os.Rename`) to prevent the injector from reading a partially written binary.
- Platform-specific files handle file permission differences: Unix uses mode `0644` for the compiled binary; Windows uses ACLs.
