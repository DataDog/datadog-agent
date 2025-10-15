# Telemetry Component Refactor: def/impl/impl-noop Architecture

## Implementation Status

### ✅ Completed Phases

- **Phase 1**: Created `comp/core/telemetry/def/` module
  - ✅ Interface definitions extracted (Component, Counter, Gauge, Histogram, etc.)
  - ✅ Zero dependencies (no prometheus, no fx, no fxutil)
  - ✅ go.mod created and tidied
  - ✅ Verified no prometheus in dependency graph

- **Phase 2**: Created `comp/core/telemetry/impl/` module
  - ✅ Renamed from telemetryimpl/ to impl/
  - ✅ Package declaration updated to `package impl`
  - ✅ Created impl/component.go with extended interface (RegisterCollector, UnregisterCollector, Gather)
  - ✅ All files updated to import from def/
  - ✅ go.mod created with prometheus dependencies
  - ✅ Module verified and tidied

- **Phase 3**: Created `comp/core/telemetry/impl-noop/` module
  - ✅ Renamed from noopsimpl/ to impl-noop/
  - ✅ Package declaration updated to `package implnoop`
  - ✅ All files updated to import from def/
  - ✅ go.mod created with NO direct prometheus imports
  - ✅ Module verified and tidied

### 🚧 In Progress

- **Phase 4**: Creating FX modules
  - ✅ comp/core/telemetry/fx/ directory created
  - ✅ fx/fx.go created
  - ✅ fx/go.mod created
  - ⏳ fx-noop/ module pending
  - ⏳ fx-full/ module pending
  - ⏳ modules.yml update pending

### ⏳ Remaining Phases

- **Phase 5**: Update pkg/trace to use def/
- **Phase 6**: Update trace-agent and all consumers
- **Phase 7**: Delete old telemetry files and run modules maintenance
- **Phase 8**: Testing and verification

---

## Problem Statement

The trace-agent needs to expose a Prometheus telemetry endpoint. However, the current `comp/core/telemetry` component is a single module that includes Prometheus dependencies in its interface definition. This means any third-party application importing `pkg/trace` transitively pulls in the entire Prometheus client library dependency tree, even when using no-op telemetry implementations.

### Current Issues

1. **Single module with Prometheus types in interfaces**: `comp/core/telemetry` defines type aliases for `prometheus.Collector` and `dto.MetricFamily` in its Component interface
2. **Leaked dependencies**: The no-op implementation (`noopsimpl/`) inherits Prometheus dependencies despite not using them
3. **Third-party burden**: External consumers of `pkg/trace` are forced to include ~50 Prometheus-related dependencies
4. **Unnecessary coupling**: `pkg/trace` only uses basic Counter/Gauge/Histogram interfaces, not Collector/MetricFamily/Gather

## Solution: Multi-Module Architecture

Following the Datadog Agent's established component architecture pattern, we'll split the telemetry component into separate modules with clear dependency boundaries.

### Architecture Overview

```
comp/core/telemetry/def/           # Interface definitions - NO prometheus
  ├── go.mod                       # Dependencies: fx, fxutil only
  ├── component.go                 # Core Component interface
  ├── counter.go                   # Counter interface
  ├── gauge.go                     # Gauge interface
  ├── histogram.go                 # Histogram interface
  ├── simple_counter.go            # SimpleCounter interface
  ├── simple_gauge.go              # SimpleGauge interface
  ├── simple_histogram.go          # SimpleHistogram interface
  └── options.go                   # Options struct

comp/core/telemetry/impl/          # Prometheus implementation
  ├── go.mod                       # Dependencies: def/, prometheus
  ├── component.go                 # Extended Component interface
  ├── telemetry.go                 # Implementation
  ├── prom_counter.go              # Prometheus counter
  ├── prom_gauge.go                # Prometheus gauge
  ├── prom_histogram.go            # Prometheus histogram
  └── simple_prom_*.go             # Simple metric implementations

comp/core/telemetry/impl-noop/     # No-op implementation
  ├── go.mod                       # Dependencies: def/ only
  ├── telemetry.go                 # No-op implementation
  ├── counter.go                   # No-op counter
  ├── gauge.go                     # No-op gauge
  └── histogram.go                 # No-op histogram

comp/core/telemetry/fx/            # FX module for standard usage
  ├── go.mod                       # Dependencies: def/, impl/, fx
  └── fx.go                        # Provides def.Component

comp/core/telemetry/fx-noop/       # FX module for no-op
  ├── go.mod                       # Dependencies: def/, impl-noop/, fx
  └── fx.go                        # Provides def.Component (no-op)

comp/core/telemetry/fx-full/       # FX module for prometheus features
  ├── go.mod                       # Dependencies: def/, impl/, fx
  └── fx.go                        # Provides impl.Component
```

### Key Design Decisions

#### 1. Clean Interface Separation

**comp/core/telemetry/def/component.go** defines the minimal interface that most consumers need:

```go
package telemetry

import "net/http"

// Component is the core telemetry component interface
type Component interface {
    // Metric creation
    NewCounter(subsystem, name string, tags []string, help string) Counter
    NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter
    NewGauge(subsystem, name string, tags []string, help string) Gauge
    NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge
    NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram
    NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram

    // Simple variants
    NewSimpleCounter(subsystem, name, help string) SimpleCounter
    NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter
    NewSimpleGauge(subsystem, name, help string) SimpleGauge
    NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge
    NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram
    NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram

    // HTTP handler for exposing metrics
    Handler() http.Handler

    // Reset for testing
    Reset()
}

// NO RegisterCollector, UnregisterCollector, or Gather methods
// These are prometheus-specific and belong in impl/ only
```

#### 2. Extended Interface for Prometheus Features

**comp/core/telemetry/impl/component.go** extends the base interface:

```go
package impl

import (
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
    "github.com/prometheus/client_golang/prometheus"
    dto "github.com/prometheus/client_model/go"
)

// Component extends telemetry.Component with prometheus-specific features
type Component interface {
    telemetry.Component

    // Prometheus-specific methods
    RegisterCollector(c prometheus.Collector)
    UnregisterCollector(c prometheus.Collector) bool
    Gather(defaultGather bool) ([]*dto.MetricFamily, error)
}

// Implementation
type telemetryImpl struct {
    // ... fields
}

var _ Component = (*telemetryImpl)(nil)
var _ telemetry.Component = (*telemetryImpl)(nil)
```

#### 3. Three FX Modules for Different Use Cases

**Standard Usage (fx/)**: For most consumers including trace-agent
```go
package fx

import (
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
    "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(func(impl impl.Component) telemetry.Component {
            return impl  // Upcast to base interface
        }),
        fx.Provide(impl.NewTelemetryComponent),
    )
}
```

**No-op Usage (fx-noop/)**: For testing or minimal builds
```go
package fxnoop

import (
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
    noopimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
)

func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(func() telemetry.Component {
            return noopimpl.NewTelemetry()
        }),
    )
}
```

**Full Prometheus Usage (fx-full/)**: For consumers needing RegisterCollector/Gather
```go
package fxfull

import (
    "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(impl.NewTelemetryComponent),
    )
}
```

### Consumer Patterns

#### Pattern 1: trace-agent (FX injection, basic interface)

```go
// cmd/trace-agent/subcommands/run/command.go
import telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx"

fx.New(
    telemetryfx.Module(),  // Injects def.Component
    // ...
)

// comp/trace/agent/impl/agent.go
import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

type dependencies struct {
    fx.In
    Telemetry telemetry.Component  // Basic interface, no prometheus types
}

func NewAgent(deps dependencies) (traceagent.Component, error) {
    receiverTelem := info.NewReceiverTelemetry(deps.Telemetry)
    statsWriterTelem := writer.NewStatsWriterTelemetry(deps.Telemetry)
    traceWriterTelem := writer.NewTraceWriterTelemetry(deps.Telemetry)
    // ...
}
```

#### Pattern 2: Third-party apps (direct instantiation, no prometheus)

```go
import (
    "github.com/DataDog/datadog-agent/pkg/trace/agent"
    "github.com/DataDog/datadog-agent/pkg/trace/config"
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
    "github.com/DataDog/datadog-agent/pkg/trace/info"
)

func main() {
    cfg := config.New()
    noopTelem := telemetry.NewTelemetry()  // NO prometheus dependency!

    receiverTelem := info.NewReceiverTelemetry(noopTelem)
    // ...

    agent.NewAgent(ctx, cfg, nil, nil, nil, receiverTelem, statsWriterTelem, traceWriterTelem)
}
```

#### Pattern 3: system-probe (FX injection, prometheus features)

```go
// cmd/system-probe/subcommands/run/command.go
import telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-full"

fx.New(
    telemetryfx.Module(),  // Injects impl.Component
    // ...
)

// Later in system-probe code:
import telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

type dependencies struct {
    fx.In
    Telemetry telemetryimpl.Component  // Full interface with prometheus methods
}

func startSystemProbe(deps dependencies) {
    deps.Telemetry.RegisterCollector(ebpftelemetry.NewDebugFsStatCollector())
}
```

#### Pattern 4: pkg/telemetry wrapper (backwards compatibility)

```go
// pkg/telemetry/telemetry.go
import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

// Uses def/ interface, no prometheus dependency
var globalTelemetry telemetry.Component

func NewCounter(subsystem, name, help string, tags []string) telemetry.Counter {
    return globalTelemetry.NewCounter(subsystem, name, tags, help)
}
```

## Implementation Plan

### Phase 1: Create def/ Module (2 hours)

**Steps:**
1. Create directory `comp/core/telemetry/def/`
2. Copy interface files from `comp/core/telemetry/`:
   - `component.go` (remove RegisterCollector/UnregisterCollector/Gather)
   - `counter.go`
   - `gauge.go`
   - `histogram.go`
   - `simple_counter.go`
   - `simple_gauge.go`
   - `simple_histogram.go`
   - `options.go`
3. Create `go.mod`:
   ```go
   module github.com/DataDog/datadog-agent/comp/core/telemetry/def

   go 1.24.0

   require (
       github.com/DataDog/datadog-agent/pkg/util/fxutil v0.61.0
       go.uber.org/fx v1.24.0
   )
   ```
4. Remove all prometheus type references
5. Update package to `package telemetry`
6. Run `dda inv go.tidy` in def/

**Validation:**
- `go mod graph | grep prometheus` returns nothing
- All interface files compile

### Phase 2: Move and Update impl/ (1.5 hours)

**Steps:**
1. Rename `comp/core/telemetry/telemetryimpl/` → `comp/core/telemetry/impl/`
2. Update `go.mod`:
   ```go
   module github.com/DataDog/datadog-agent/comp/core/telemetry/impl

   require (
       github.com/DataDog/datadog-agent/comp/core/telemetry/def v0.72.0-devel
       github.com/prometheus/client_golang v1.23.2
       github.com/prometheus/client_model v0.6.2
       // ... other deps
   )
   ```
3. Create `component.go` with extended interface:
   ```go
   package impl

   import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

   type Component interface {
       telemetry.Component
       RegisterCollector(c prometheus.Collector)
       UnregisterCollector(c prometheus.Collector) bool
       Gather(defaultGather bool) ([]*dto.MetricFamily, error)
   }
   ```
4. Update all imports:
   - Replace `github.com/DataDog/datadog-agent/comp/core/telemetry`
   - With `telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"`
5. Run `dda inv go.tidy` in impl/

**Validation:**
- impl/ builds successfully
- Implements both telemetry.Component and impl.Component

### Phase 3: Move and Update impl-noop/ (1 hour)

**Steps:**
1. Rename `comp/core/telemetry/noopsimpl/` → `comp/core/telemetry/impl-noop/`
2. Update `go.mod`:
   ```go
   module github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop

   require (
       github.com/DataDog/datadog-agent/comp/core/telemetry/def v0.72.0-devel
       go.uber.org/fx v1.24.0
   )
   ```
3. Update imports to use def/
4. Remove any prometheus-related code
5. Implement only telemetry.Component interface
6. Run `dda inv go.tidy` in impl-noop/

**Validation:**
- `go mod graph | grep prometheus` returns nothing
- impl-noop/ builds successfully
- Implements telemetry.Component

### Phase 4: Create FX Modules (30 minutes)

**Create comp/core/telemetry/fx/:**
```go
// fx.go
package fx

import (
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
    "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the standard telemetry implementation
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(impl.NewTelemetryComponent),
    )
}
```

**Create comp/core/telemetry/fx-noop/:**
```go
// fx.go
package fxnoop

import (
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
    noopimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the no-op telemetry implementation
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(noopimpl.NewTelemetry),
    )
}
```

**Create comp/core/telemetry/fx-full/:**
```go
// fx.go
package fxfull

import (
    "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the full prometheus telemetry implementation
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(impl.NewTelemetryComponent),
    )
}
```

**Update modules.yml:**
```yaml
modules:
  comp/core/telemetry/def:
    independent: true
    used_by_otel: true
  comp/core/telemetry/impl:
    independent: true
  comp/core/telemetry/impl-noop:
    independent: true
  comp/core/telemetry/fx:
    independent: false
  comp/core/telemetry/fx-noop:
    independent: false
  comp/core/telemetry/fx-full:
    independent: false
```

### Phase 5: Update trace-agent (1 hour)

**Steps:**
1. Update `cmd/trace-agent/subcommands/run/command.go`:
   ```go
   // OLD
   import "github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
   telemetryimpl.Module(),

   // NEW
   import telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx"
   telemetryfx.Module(),
   ```

2. Update `comp/trace/agent/impl/agent.go`:
   ```go
   import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

   type dependencies struct {
       fx.In
       Telemetry telemetry.Component
   }
   ```

3. Update `pkg/trace` telemetry constructors:
   ```go
   // pkg/trace/info/telemetry.go
   import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

   func NewReceiverTelemetry(telemetryComp telemetry.Component) *ReceiverTelemetry {
       return &ReceiverTelemetry{
           tracesReceived: telemetryComp.NewCounterWithOpts(...),
           // ...
       }
   }
   ```

4. Update `pkg/trace/agent.NewAgent()` signature:
   ```go
   func NewAgent(
       ctx context.Context,
       conf *config.AgentConfig,
       telemetryCollector telemetry.TelemetryCollector,
       statsd statsd.ClientInterface,
       comp compression.Component,
       receiverTelem *info.ReceiverTelemetry,
       statsWriterTelem *writer.StatsWriterTelemetry,
       traceWriterTelem *writer.TraceWriterTelemetry,
   ) *Agent
   ```

### Phase 6: Update pkg/telemetry Wrapper (30 minutes)

**Steps:**
1. Update imports in `pkg/telemetry/*.go`:
   ```go
   // OLD
   import "github.com/DataDog/datadog-agent/comp/core/telemetry"

   // NEW
   import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
   ```

2. Update GetCompatComponent() if needed (may use impl-noop by default)

### Phase 7: Update system-probe (30 minutes)

**Steps:**
1. Update `cmd/system-probe/subcommands/run/command.go`:
   ```go
   import telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-full"
   telemetryfx.Module(),
   ```

2. Update injection sites that use RegisterCollector:
   ```go
   import telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

   type dependencies struct {
       fx.In
       Telemetry telemetryimpl.Component
   }
   ```

### Phase 8: Update Other Consumers (1 hour)

**Steps:**
1. Search for all imports of `comp/core/telemetry/telemetryimpl`:
   ```bash
   dda inv modules.for-each "grep -r 'comp/core/telemetry/telemetryimpl' . || true"
   ```

2. Update each to use appropriate FX module:
   - If using RegisterCollector/Gather: use `fx-full/`
   - If only using metrics: use `fx/`

3. Update any GetCompatComponent() calls

### Phase 9: Clean Up Old Files (15 minutes)

**Steps:**
1. Remove old build-tagged files from `comp/core/telemetry/`:
   - `collector.go`
   - `collector_noop.go`
   - `metric.go`
   - `metric_noop.go`

2. Update main `comp/core/telemetry/go.mod` to reduce deps (or deprecate the module)

### Phase 10: Update Replace Directives (15 minutes)

**Steps:**
1. Run `dda inv modules.add-all-replace` to update replace directives
2. Run `dda inv go.tidy-all` across all modules
3. Verify replace directives in main `go.mod`:
   ```go
   replace (
       github.com/DataDog/datadog-agent/comp/core/telemetry/def => ./comp/core/telemetry/def
       github.com/DataDog/datadog-agent/comp/core/telemetry/impl => ./comp/core/telemetry/impl
       github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop => ./comp/core/telemetry/impl-noop
   )
   ```

## Testing Strategy

### Test 1: Third-party Import - NO Prometheus (Critical)

```bash
mkdir /tmp/test-trace-import
cd /tmp/test-trace-import

cat > go.mod <<EOF
module testapp
go 1.24.0
require github.com/DataDog/datadog-agent/pkg/trace v0.72.0-devel
EOF

cat > main.go <<EOF
package main

import (
    "context"
    "github.com/DataDog/datadog-agent/pkg/trace/agent"
    "github.com/DataDog/datadog-agent/pkg/trace/config"
    telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
    "github.com/DataDog/datadog-agent/pkg/trace/info"
)

func main() {
    cfg := config.New()
    noopTelem := telemetry.NewTelemetry()
    receiverTelem := info.NewReceiverTelemetry(noopTelem)
    _ = agent.NewAgent(context.Background(), cfg, nil, nil, nil, receiverTelem, nil, nil)
}
EOF

go mod tidy
go mod graph | grep prometheus
# MUST output: NOTHING (no prometheus!)

go build
echo "SUCCESS: No prometheus dependencies!"
```

### Test 2: trace-agent Build with Prometheus (Critical)

```bash
cd datadog-agent
dda inv trace-agent.build

# Run and verify prometheus endpoint
./bin/trace-agent/trace-agent run -c dev/dist/datadog.yaml &
sleep 5

curl -s http://localhost:5012/telemetry | head -20
# MUST output: prometheus metrics format

pkill trace-agent
echo "SUCCESS: Prometheus endpoint working!"
```

### Test 3: system-probe with RegisterCollector (Important)

```bash
dda inv system-probe.build

# Verify it compiles and RegisterCollector is available
grep -r "RegisterCollector" cmd/system-probe/
echo "SUCCESS: system-probe built with prometheus features!"
```

### Test 4: Unit Tests (Critical)

```bash
# Test all telemetry modules
dda inv test --targets=./comp/core/telemetry/def
dda inv test --targets=./comp/core/telemetry/impl
dda inv test --targets=./comp/core/telemetry/impl-noop

# Test trace components
dda inv test --targets=./comp/trace/agent
dda inv test --targets=./pkg/trace/agent
dda inv test --targets=./pkg/trace/info

echo "SUCCESS: All tests pass!"
```

### Test 5: Backwards Compatibility (Important)

```bash
# Verify other agents still build
dda inv agent.build --build-exclude=systemd
dda inv dogstatsd.build
dda inv cluster-agent.build

echo "SUCCESS: All agents build successfully!"
```

## Success Criteria

After completing the refactor, verify:

1. ✅ **No prometheus in third-party imports**
   - `go mod graph` on test app shows NO prometheus dependencies

2. ✅ **trace-agent has working prometheus endpoint**
   - Binary builds successfully
   - `/telemetry` endpoint returns prometheus metrics

3. ✅ **system-probe retains prometheus features**
   - Builds successfully
   - Can use RegisterCollector and Gather methods

4. ✅ **All tests pass**
   - Unit tests for def/, impl/, impl-noop/
   - Integration tests for trace-agent
   - Regression tests for other agents

5. ✅ **Clean module dependency graph**
   - def/ has no prometheus deps
   - impl-noop/ has no prometheus deps
   - impl/ is the only module with prometheus deps

6. ✅ **Following established patterns**
   - Structure matches other components (log, config, etc.)
   - Properly registered in modules.yml
   - Replace directives in place

## Benefits

### For Third-party Consumers
- **Zero prometheus dependencies** when importing pkg/trace
- **Reduced binary size** (50+ fewer transitive dependencies)
- **Faster builds** (fewer packages to compile)
- **Cleaner dependency trees** (easier to audit and manage)

### For Datadog Agent Development
- **Clear separation of concerns** (interfaces vs implementations)
- **Multiple implementations** (prometheus, no-op, future alternatives)
- **Better testability** (mock implementations without prometheus)
- **Flexible dependency injection** (FX chooses appropriate impl)

### For Maintenance
- **No build tags needed** (module structure handles it)
- **Follows established patterns** (consistent with other components)
- **Future-proof** (easy to add new implementations)
- **Backwards compatible** (existing code continues to work)

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking changes for consumers | High | Comprehensive testing, phased rollout |
| Test failures from signature changes | Medium | Update all test files, create test helpers |
| Import cycle issues | Medium | def/ has minimal deps, careful import ordering |
| FX injection conflicts | Low | Follow existing FX patterns, clear documentation |
| Performance regression | Low | No logic changes, just module reorganization |

## Documentation Updates Required

1. **Component README**: Update comp/core/telemetry/README.md
2. **Migration guide**: Create guide for external consumers
3. **API documentation**: Update godoc comments
4. **Architecture docs**: Update docs/components/
5. **CHANGELOG**: Document breaking changes and migration path

## Success Metrics

Post-deployment, track:
- **Dependency count**: Third-party imports should have ~50 fewer deps
- **Build times**: Measure improvement for external consumers
- **Binary size**: Compare trace-agent binary size (should be similar)
- **Performance**: Verify no regression in metrics collection
- **Adoption**: Monitor external projects updating to new structure

## Conclusion

This refactor provides a clean, maintainable solution that:
- ✅ Eliminates prometheus dependencies for third-party consumers
- ✅ Follows established Datadog Agent module patterns
- ✅ Maintains full backwards compatibility
- ✅ Enables flexible implementation choices via FX
- ✅ Improves separation of concerns and testability

The multi-module approach is well-proven in the codebase (see log, config, tagger components) and provides the cleanest path forward without build tags or compatibility hacks.
