# Process Manager Migration Plan

## Overview

Migrating the full-featured Rust process manager from `/Users/josemanuel.almaza/projects/process-manager/` to `/Users/josemanuel.almaza/go/src/github.com/DataDog/datadog-agent/process_manager/`.

## Current State

**Original Project** (`~/projects/process-manager/`):
- ✅ Full process lifecycle management (systemd-like)
- ✅ Engine library with core functionality
- ✅ gRPC daemon
- ✅ CLI client
- ✅ Go FFI client
- ✅ Comprehensive test suite
- ✅ Examples and documentation

**Target Location** (`datadog-agent/process_manager/`):
- ✅ Directory structure created
- ✅ Basic daemon skeleton
- ⏳ Ready for full migration

## Directory Structure (After Migration)

```
process_manager/
├── Cargo.toml              # Workspace manifest
├── README.md               # Main documentation
├── MIGRATION_PLAN.md       # This file
│
├── engine/                 # Core library (from original)
│   ├── Cargo.toml
│   ├── build.rs
│   ├── src/
│   │   ├── lib.rs
│   │   ├── process_registry.rs
│   │   ├── dependencies.rs
│   │   ├── health_monitor.rs
│   │   ├── socket_manager.rs
│   │   ├── resources.rs
│   │   ├── event_hub.rs
│   │   └── ffi.rs         # C FFI for Go integration
│   └── tests/
│
├── daemon/                 # gRPC server (update existing)
│   ├── Cargo.toml
│   ├── src/
│   │   └── main.rs        # Update to use engine
│   └── proto_descriptor.bin
│
├── cli/                    # Command-line client (from original)
│   ├── Cargo.toml
│   ├── src/
│   │   ├── main.rs
│   │   ├── commands.rs
│   │   ├── formatters.rs
│   │   └── options.rs
│   └── build.rs
│
├── proto/                  # Protocol Buffers (from original)
│   └── process_manager.proto
│
├── go-client/             # Go FFI client (from original)
│   ├── client.go
│   ├── errors.go
│   ├── go.mod
│   ├── go.sum
│   └── README.md
│
├── tests/                 # Integration tests (from original)
│   ├── Cargo.toml
│   ├── common.rs
│   ├── e2e_*.rs
│   └── ...
│
├── examples/              # YAML configs (from original)
│   ├── simple-webserver.yaml
│   ├── dependencies.yaml
│   ├── health-checks.yaml
│   └── ...
│
└── docs/                  # Documentation (from original)
    ├── ARCHITECTURE.md
    ├── HEALTH_CHECKS.md
    ├── RESOURCE_LIMITS.md
    └── ...
```

## Migration Steps

### Phase 1: Copy Core Components ⏳

**Step 1.1: Copy Engine Library**
```bash
cp -r ~/projects/process-manager/engine \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

**Step 1.2: Copy CLI**
```bash
cp -r ~/projects/process-manager/cli \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

**Step 1.3: Copy Proto Definitions**
```bash
cp -r ~/projects/process-manager/proto \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

### Phase 2: Create Workspace ⏳

**Step 2.1: Create Workspace Cargo.toml**

Create top-level workspace manifest to manage all crates together.

**Step 2.2: Update Daemon**

Update the existing daemon to use the engine library and implement full gRPC functionality.

### Phase 3: Integration Testing ⏳

**Step 3.1: Copy Tests**
```bash
cp -r ~/projects/process-manager/tests \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

**Step 3.2: Copy Examples**
```bash
cp -r ~/projects/process-manager/examples \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

### Phase 4: Build Integration ⏳

**Step 4.1: Create Invoke Tasks**

Add build tasks to `tasks/` for building Rust components:
- `dda inv process-manager.build`
- `dda inv process-manager.test`
- `dda inv process-manager.build-release`

**Step 4.2: Integration with Agent Build**

Add process manager to main agent build pipeline.

### Phase 5: Documentation ⏳

**Step 5.1: Copy Docs**
```bash
cp -r ~/projects/process-manager/docs \
  ~/go/src/github.com/DataDog/datadog-agent/process_manager/
```

**Step 5.2: Update READMEs**

Update documentation to reflect new location and Datadog Agent integration.

## Build Commands

After migration, these commands will be available:

```bash
# Build all components
dda inv process-manager.build

# Build release version
dda inv process-manager.build --release

# Run tests
dda inv process-manager.test

# Run specific test suite
dda inv process-manager.test --suite e2e

# Build just the daemon
dda inv process-manager.build-daemon

# Build just the CLI
dda inv process-manager.build-cli

# Install binaries
dda inv process-manager.install
```

## Integration Points

### With Datadog Agent

1. **Configuration**: Process manager config will be part of `datadog.yaml`
2. **Managed Components**: Can manage trace-agent, process-agent, etc.
3. **Telemetry**: Integrated with agent's telemetry system
4. **Logging**: Uses agent's logging infrastructure

### Example Configuration

```yaml
# datadog.yaml
process_manager:
  enabled: true
  
  # Manage agent components
  managed_processes:
    trace-agent:
      command: /opt/datadog-agent/bin/trace-agent
      restart_policy: always
      health_check:
        type: http
        endpoint: http://localhost:8126/info
        interval: 30
      auto_start: true
    
    dogstatsd:
      command: /opt/datadog-agent/bin/dogstatsd
      restart_policy: on-failure
      auto_start: true
      binds_to: [trace-agent]  # Stop if trace-agent stops
```

## Testing Strategy

1. **Unit Tests**: Test each crate independently
2. **Integration Tests**: Test daemon + CLI + engine together
3. **E2E Tests**: Full workflow tests
4. **Agent Integration Tests**: Test with Datadog Agent components

## Migration Checklist

- [ ] Phase 1: Copy core components
  - [ ] Engine library
  - [ ] CLI client
  - [ ] Proto definitions
- [ ] Phase 2: Create workspace
  - [ ] Workspace Cargo.toml
  - [ ] Update daemon
  - [ ] Build verification
- [ ] Phase 3: Integration testing
  - [ ] Copy tests
  - [ ] Copy examples
  - [ ] Verify all tests pass
- [ ] Phase 4: Build integration
  - [ ] Create invoke tasks
  - [ ] Add to agent build
  - [ ] CI/CD integration
- [ ] Phase 5: Documentation
  - [ ] Copy docs
  - [ ] Update READMEs
  - [ ] Integration guides

## Next Steps

1. Start with Phase 1: Copy engine, CLI, and proto
2. Create workspace Cargo.toml
3. Update daemon to use engine
4. Test build and basic functionality
5. Iterate on integration

## Notes

- Keep original project intact during migration
- Test each phase before moving to next
- Document any changes needed for Datadog Agent integration
- Consider backward compatibility for existing users

## Success Criteria

✅ All components build successfully
✅ All tests pass
✅ CLI can manage processes
✅ Daemon runs as systemd service
✅ Integrated with agent build system
✅ Documentation is complete

