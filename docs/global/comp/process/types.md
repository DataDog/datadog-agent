> **TL;DR:** A shared-types package that defines the common contracts (`CheckComponent`, `ProvidesCheck`, `Payload`, `RTResponse`) used by all process-agent check components to participate in the fx dependency-injection graph without creating circular imports.

# comp/process/types — Shared Process Types

**Import path:** `github.com/DataDog/datadog-agent/comp/process/types`
**Team:** container-experiences
**Importers:** ~19 packages

## Purpose

`comp/process/types` is a small shared-types package that defines the common contracts used by all process-agent check components to participate in the fx dependency-injection graph. It avoids circular imports by keeping these types separate from any specific check or runner implementation.

The package defines:

- How individual check components advertise themselves to the fx container (via `ProvidesCheck` and the `"check"` value group).
- The payload type (`Payload`) used to pass check results from the runner to the submitter.
- The real-time notification channel type (`RTResponse`) that the submitter uses to signal the runner about real-time mode changes.
- Test helpers for mocking check components.

## Key elements

### Key interfaces

#### `CheckComponent`

```go
type CheckComponent interface {
    Object() checks.Check
}
```

The interface implemented by every process check component (process, connections, container, rt-container, process-discovery). `Object()` returns the underlying `pkg/process/checks.Check` implementation, which carries `Name()`, `IsEnabled()`, `Run()`, `Realtime()`, and other check lifecycle methods.

### Key types

#### `ProvidesCheck`

```go
type ProvidesCheck struct {
    fx.Out
    CheckComponent CheckComponent `group:"check"`
}
```

A convenience fx output struct that check component constructors embed (or return) to register themselves in the `"check"` value group. The runner, submitter, and agent components all inject this group as `[]types.CheckComponent` tagged with `group:"check"`.

**Pattern used by every check component:**

```go
// Inside a check component constructor (e.g. processcheckimpl)
type result struct {
    fx.Out
    Check     types.ProvidesCheck    // registers into the "check" group
    Component processcheck.Component  // also provides the typed component
}
```

#### `Payload`

```go
type Payload struct {
    CheckName string
    Message   []model.MessageBody
}
```

Carries the serialised output of a single check execution. `CheckName` identifies the source check; `Message` is a slice of protobuf-encoded `agent-payload` messages ready for submission.

#### `RTResponse`

```go
type RTResponse []*model.CollectorStatus
```

A slice of `CollectorStatus` messages returned by the Datadog intake in response to process payloads. The submitter publishes these on a `<-chan types.RTResponse` channel so the runner can adjust its real-time collection schedule (e.g. start or stop the short-interval process check loop).

## fx wiring

`comp/process/types` does not define an fx `Module()`. It is a plain Go package imported directly wherever its types are needed. The `"check"` group injection pattern is the primary integration point:

```go
// Consumer side (runner, submitter, agent component)
type dependencies struct {
    fx.In
    Checks []types.CheckComponent `group:"check"`
}

// Provider side (each check component constructor)
return types.ProvidesCheck{
    CheckComponent: myCheck,
}
```

`fxutil.GetAndFilterGroup` is used by consumers to strip the `nil` entries that fx inserts for optional providers in the group.

## Test mock

`mock.go` (build tag `test`) provides `NewMockCheckComponent`:

```go
func NewMockCheckComponent(t *testing.T, name string, isEnabled bool) CheckComponent
```

Returns a `CheckComponent` backed by a `checkMocks.Check` testify mock with `Name()` and `IsEnabled()` pre-stubbed. Use `MockCheckParams[T]` when additional mock orchestration is needed via an optional fx-injected callback.

## Usage patterns

**Adding a new check component:**

1. Create a new package under `comp/process/<checkname>/`.
2. Implement `types.CheckComponent` (i.e., provide an `Object() checks.Check` method).
3. Return a `types.ProvidesCheck{CheckComponent: c}` from the fx constructor, along with any other outputs.
4. Register the module in `comp/process/bundle.go`.

**Consuming the check group:**

```go
import "github.com/DataDog/datadog-agent/comp/process/types"

type deps struct {
    fx.In
    Checks []types.CheckComponent `group:"check"`
}

func newRunner(deps deps) runner.Component {
    for _, c := range fxutil.GetAndFilterGroup(deps.Checks) {
        if c.Object().IsEnabled() {
            // schedule c.Object()
        }
    }
}
```

**Key consumers:**

| Package | How it uses `types` |
|---|---|
| `comp/process/runner/runnerimpl` | Collects all checks from the group; passes enabled ones to `processRunner.NewRunner` |
| `comp/process/submitter/submitterimpl` | Collects all checks from the group to configure per-check forwarder routing; publishes `RTResponse` channel |
| `comp/process/agent/agentimpl` | Collects all checks from the group to determine if the agent should be enabled |
| Each `*checkimpl` package | Provides a `ProvidesCheck` to register itself in the group |

## Related documentation

| Document | Relationship |
|---|---|
| [comp/process/runner](runner.md) | Primary consumer of `ProvidesCheck`; filters the `"check"` group to enabled checks and drives the `CheckRunner` scheduling loop |
| [comp/process/submitter](submitter.md) | Consumes `[]types.CheckComponent` to configure per-check intake routing; produces the `<-chan types.RTResponse` channel consumed by the runner |
| [comp/process/agent](agent.md) | Consumes `[]types.CheckComponent` to decide whether the process subsystem should be enabled at all |
| [pkg/process/checks](../../../global/pkg/process/checks.md) | Defines the `Check` interface that `CheckComponent.Object()` returns, all check name constants, and `RunResult` types |
