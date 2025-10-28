# System-Probe Modules

## Overview

System-probe modules are independent components that run within the system-probe process and expose functionality via HTTP endpoints. Modules are registered at initialization time and can provide various monitoring capabilities, especially those requiring kernel-level access through eBPF.

See also: `.cursor/rules/system_probe_modules.mdc` for a concise rule-style overview and links to related eBPF check docs.

## Module Registration

Each module is a `module.Factory` registered in `init()` with the following fields:

```go
var MyModule = &module.Factory{
    Name:             config.MyModuleName,           // Unique identifier
    ConfigNamespaces: []string{},                     // Config sections to load
    Fn:               createModule,                   // Constructor function
    NeedsEBPF:        func() bool { return true },    // eBPF requirement indicator
}

func init() {
    registerModule(MyModule)
}
```

### Factory Fields

- **Name**: Unique module identifier (type `types.ModuleName`)
  - Defined as constant in `pkg/system-probe/config/config.go`
  - Used to route HTTP requests to the module

- **ConfigNamespaces**: List of configuration namespaces this module needs
  - Empty list means no special config loading
  - Module-specific configs are typically accessed via module name

- **Fn**: Constructor function signature:
  ```go
  func(*sysconfigtypes.Config, module.FactoryDependencies) (module.Module, error)
  ```

- **NeedsEBPF**: Function returning whether the module requires eBPF support
  - Used to determine if module should be loaded based on system capabilities

## Module Interface

Modules must implement the `module.Module` interface:

```go
type Module interface {
    Register(router *module.Router) error
    GetStats() map[string]interface{}
    Close() error  // Optional, for cleanup
}
```

### Register(router *module.Router)

Sets up HTTP endpoints for the module:

```go
func (m *myModule) Register(httpMux *module.Router) error {
    httpMux.HandleFunc("/check", m.handleCheck)
    httpMux.HandleFunc("/stats", m.handleStats)
    return nil
}
```

Common patterns:
- `/check` - Main data endpoint for agent checks
- `/stats` - Module health and statistics
- Custom endpoints for specific functionality

### GetStats()

Returns module health metrics:

```go
func (m *myModule) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "last_check": m.lastCheck.Load(),
        "event_count": m.eventCount.Load(),
    }
}
```

Use atomic operations for concurrent-safe counters.

### Close() (Optional)

Cleanup resources when module shuts down:

```go
func (m *myModule) Close() error {
    if m.tracer != nil {
        m.tracer.Close()
    }
    return nil
}
```

## eBPF Modules Pattern

For modules using eBPF probes:

```go
type myModule struct {
    *probe.Tracer            // Embed the probe tracer
    lastCheck atomic.Int64   // Health tracking
}

func createModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
    tracer, err := probe.NewTracer(ebpf.NewConfig())
    if err != nil {
        return nil, fmt.Errorf("unable to start tracer: %w", err)
    }

    return &myModule{Tracer: tracer}, nil
}

func (m *myModule) Register(httpMux *module.Router) error {
    httpMux.HandleFunc("/check", func(w http.ResponseWriter, _ *http.Request) {
        m.lastCheck.Store(time.Now().Unix())
        stats := m.Tracer.GetAndFlush()
        utils.WriteAsJSON(w, stats, utils.CompactOutput)
    })
    return nil
}
```

Key points:
- Embed the probe tracer struct
- Create tracer in constructor
- `/check` endpoint calls `GetAndFlush()` and returns JSON
- Track last check time for health monitoring
- Use `utils.WriteAsJSON()` for consistent response format

## Module Communication

### From Agent to System-Probe

Agent checks use `sysprobeclient.GetCheck[T]()`:

```go
stats, err := sysprobeclient.GetCheck[model.Stats](
    sysProbeClient,
    sysconfig.MyModuleName,
)
```

This performs:
1. HTTP GET to `/modules/{ModuleName}/check`
2. JSON unmarshal into type `T`
3. Error handling with startup grace period

### Response Format

Modules should return JSON-serializable data:

```go
utils.WriteAsJSON(w, data, utils.CompactOutput)
```

Options:
- `utils.CompactOutput` - Compact JSON (no whitespace)
- `utils.PrettyOutput` - Pretty-printed JSON (for debugging)

## Configuration

Module configuration follows the namespace pattern:

```go
// In pkg/config/setup/system_probe.go
const myModuleNS = "my_module"

cfg.BindEnvAndSetDefault(join(myModuleNS, "enabled"), false)
cfg.BindEnvAndSetDefault(join(myModuleNS, "option1"), "default")
```

Access in module:
```go
enabled := cfg.GetBool("my_module.enabled")
```

## Build Tags

eBPF modules require appropriate build tags:

```go
//go:build linux && linux_bpf
```

This ensures the module only compiles on Linux with eBPF support.

## Examples

### Simple eBPF Module

See `tcp_queue_tracer.go`, `oom_kill.go`, `seccomp_tracer.go`

### Complex Module with Multiple Endpoints

See `gpu.go` for a module with multiple endpoints and complex state management

### Network Monitoring

See `network_tracer.go` for large-scale eBPF-based network monitoring

## Testing

Modules are tested indirectly through their probe tests:
- Probe tests verify eBPF functionality
- Integration tests verify end-to-end agent check communication
- Module registration is tested via system-probe startup tests

## Best Practices

1. **Error Handling**: Always wrap errors with context
   ```go
   return nil, fmt.Errorf("failed to initialize: %w", err)
   ```

2. **Atomic Counters**: Use `atomic` package for thread-safe stats
   ```go
   lastCheck atomic.Int64
   lastCheck.Store(time.Now().Unix())
   ```

3. **Resource Cleanup**: Implement Close() for proper shutdown
   ```go
   func (m *myModule) Close() error {
       m.tracer.Close()
       return nil
   }
   ```

4. **Health Tracking**: Track last check time and error counts
   ```go
   m.lastCheck.Store(time.Now().Unix())
   ```

5. **JSON Responses**: Use standard utilities for consistency
   ```go
   utils.WriteAsJSON(w, data, utils.CompactOutput)
   ```

6. **Build Tags**: Ensure proper platform targeting
   ```go
   //go:build linux && linux_bpf
   ```

## Module Lifecycle

1. **Init Time**: `init()` calls `registerModule()`
2. **Startup**: System-probe calls factory `Fn` if module is enabled
3. **Registration**: Module's `Register()` sets up HTTP routes
4. **Runtime**: Agent makes HTTP requests to module endpoints
5. **Shutdown**: Module's `Close()` (if implemented) cleans up resources

## Debugging

- Check module stats: `curl http://localhost:3333/debug/stats`
- View module status: Check system-probe logs for module initialization
- Test endpoint: `curl http://localhost:3333/modules/{module_name}/check`
