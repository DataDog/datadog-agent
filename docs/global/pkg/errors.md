# pkg/errors

**Import path:** `github.com/DataDog/datadog-agent/pkg/errors`

## Purpose

`pkg/errors` provides a small set of typed errors for use across the Datadog Agent codebase. Rather than comparing error strings or defining ad-hoc sentinel values in every package, callers create errors with a specific *reason* (not found, retriable, disabled, etc.) and test for that reason using the corresponding `Is*` predicate.

This keeps error classification consistent and avoids false positives: a string-matched error `"foo" not found` is **not** equal to one created with `NewNotFound("foo")` — only an actual `AgentError` with the right reason passes the predicate. The predicates use `errors.As` under the hood, so they work correctly with wrapped errors.

## Key Elements

### `AgentError`

```go
type AgentError struct { ... } // implements error
```

The concrete error type. All constructors in this package return `*AgentError`. Its `Error()` method returns a human-readable message; the reason tag is unexported and can only be inspected through the `Is*` predicates.

### Constructors and predicates

| Constructor | Predicate | Typical meaning |
|---|---|---|
| `NewNotFound(object any)` | `IsNotFound(err)` | A requested resource (container, pod, PID, config key…) does not exist |
| `NewRetriable(object any, cause error)` | `IsRetriable(err)` | A fetch failed but the caller should retry |
| `NewDisabled(component, reason string)` | `IsDisabled(err)` | An agent component is disabled (e.g. runtime not detected) |
| `NewRemoteServiceError(target, status string)` | `IsRemoteService(err)` | A remote service (e.g. the Cluster Agent) returned an error status |
| `NewTimeoutError(target string, cause error)` | `IsTimeout(err)` | A call timed out |

There is also `IsPartial(err)` for checking the `partialError` reason; no public constructor exists for that reason — it is reserved for internal use.

## Usage

### Creating typed errors

```go
import dderrors "github.com/DataDog/datadog-agent/pkg/errors"

// Signal that a resource was not found
return dderrors.NewNotFound(pid)

// Signal that a fetch failed but is worth retrying
return dderrors.NewRetriable("podlist", fmt.Errorf("HTTP %d: %s", code, body))

// Signal that a collector component is not available at runtime
return dderrors.NewDisabled("crio", "CRI-O not detected")

// Signal that a downstream service is unavailable
return dderrors.NewRemoteServiceError("datadog cluster agent", resp.Status)

// Signal a timeout
return dderrors.NewTimeoutError("kubelet", ctx.Err())
```

### Testing error kinds

The predicates work with plain `*AgentError` values and with errors wrapped at any depth via `fmt.Errorf("...: %w", err)`:

```go
if dderrors.IsNotFound(err) {
    // silently skip — resource may not exist yet
} else if err != nil {
    log.Errorf("unexpected error: %v", err)
}

if dderrors.IsRetriable(err) {
    // schedule a retry
}
```

### Real-world patterns

- **workloadmeta collectors** (`comp/core/workloadmeta/collectors/`) return `NewDisabled` from their `Start` method when the underlying runtime (CRI-O, Podman) is not present. The workloadmeta store uses `IsDisabled` to suppress the error.
- **GPU check** (`pkg/collector/corechecks/gpu/`) returns `NewNotFound(pid)` when a PID has no associated container, and callers use `IsNotFound` to distinguish a normal "no data yet" state from a real error.
- **Kubelet client** (`pkg/util/kubernetes/kubelet/`) wraps HTTP and network errors with `NewRetriable`, letting the collector framework retry on transient failures.
- **Language detection** (`pkg/languagedetection/`) uses `NewNotFound` as a sentinel to signal a missing runtime artifact.
