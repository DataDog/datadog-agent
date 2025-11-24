# Domain Services Naming Standardization

## Current State Analysis

### Existing Domain Services

| File Name | Struct Name | Pattern | Consistency |
|-----------|-------------|---------|-------------|
| `config_parser.rs` | `ConfigParser` | `<Noun>Parser` | ❌ Different pattern |
| `conflict_service.rs` | `ConflictService` | `<Noun>Service` | ✅ Consistent |
| `dependency_service.rs` | `DependencyService` | `<Noun>Service` | ✅ Consistent |
| `health_monitor.rs` | `HealthMonitor` | `<Noun>Monitor` | ❌ Different pattern |
| `hook_executor.rs` | `execute_hooks()` | Function only | ❌ No struct |
| `process_creation.rs` | `ProcessCreationService` | `Process<Noun>Service` | ⚠️ Verbose |
| `process_lifecycle.rs` | `ProcessLifecycleService` | `Process<Noun>Service` | ⚠️ Verbose |
| `process_spawn_service.rs` | `ProcessSpawnService` | `Process<Noun>Service` | ⚠️ Verbose |
| `process_supervisor.rs` | `ProcessSupervisor` | `Process<Noun>` | ❌ Different pattern |
| `process_watcher.rs` | `ProcessWatcher` | `Process<Noun>` | ❌ Different pattern |
| `runtime_directory.rs` | Functions only | N/A | ❌ No struct |
| `socket_activation.rs` | `SocketActivationManager` | `<Noun>Manager` | ❌ Different pattern |

### Problems Identified

1. **Inconsistent Suffixes**: Service, Parser, Monitor, Supervisor, Watcher, Manager
2. **Redundant Prefixes**: "Process" appears in 5 service names
3. **Mixed Patterns**: Some are structs, some are just functions
4. **Unclear Naming**: Not obvious what's a domain service vs infrastructure

---

## Proposed Naming Standard

### Core Principle

**Domain services should use the `<Domain><Action>Service` pattern**

- **Domain**: The aggregate or concept (Process, Socket, Config, Health)
- **Action**: What the service does (Creation, Lifecycle, Activation, Monitoring)
- **Service**: Suffix indicating it's a domain service

### Benefits

✅ **Consistency**: All domain services follow same pattern  
✅ **Clarity**: Easy to distinguish domain services from other components  
✅ **Discoverability**: Predictable naming makes code easier to navigate  
✅ **Scalability**: Pattern works for new services  

---

## Proposed Renaming

### Option A: Verbose & Explicit (Recommended)

| Current Name | Proposed Name | Rationale |
|--------------|---------------|-----------|
| `ConfigParser` | `ConfigParsingService` | Parsing is an action, Config is the domain |
| `ConflictService` | `ConflictResolutionService` | More explicit about what it does |
| `DependencyService` | `DependencyResolutionService` | More explicit about what it does |
| `HealthMonitor` | `HealthMonitoringService` | Monitoring is an action |
| `ProcessCreationService` | ✅ Keep as-is | Already follows pattern |
| `ProcessLifecycleService` | ✅ Keep as-is | Already follows pattern |
| `ProcessSpawnService` | `ProcessSpawningService` | Consistent verb form (-ing) |
| `ProcessSupervisor` | `ProcessSupervisionService` | Supervision is an action |
| `ProcessWatcher` | `ProcessWatchingService` | Watching is an action |
| `SocketActivationManager` | `SocketActivationService` | Manager → Service for consistency |

**File Names** (match struct names):
- `config_parser.rs` → `config_parsing_service.rs`
- `conflict_service.rs` → `conflict_resolution_service.rs`
- `dependency_service.rs` → `dependency_resolution_service.rs`
- `health_monitor.rs` → `health_monitoring_service.rs`
- `process_supervisor.rs` → `process_supervision_service.rs`
- `process_watcher.rs` → `process_watching_service.rs`
- `socket_activation.rs` → `socket_activation_service.rs`

---

### Option B: Concise & Domain-Focused

| Current Name | Proposed Name | Rationale |
|--------------|---------------|-----------|
| `ConfigParser` | `ConfigService` | Config is the domain |
| `ConflictService` | ✅ Keep as-is | Already good |
| `DependencyService` | ✅ Keep as-is | Already good |
| `HealthMonitor` | `HealthService` | Health is the domain |
| `ProcessCreationService` | ✅ Keep as-is | Already good |
| `ProcessLifecycleService` | ✅ Keep as-is | Already good |
| `ProcessSpawnService` | ✅ Keep as-is | Already good |
| `ProcessSupervisor` | `ProcessSupervisionService` | Add Service suffix |
| `ProcessWatcher` | `ProcessWatchingService` | Add Service suffix |
| `SocketActivationManager` | `SocketActivationService` | Manager → Service |

**File Names**:
- `config_parser.rs` → `config_service.rs`
- `health_monitor.rs` → `health_service.rs`
- `process_supervisor.rs` → `process_supervision_service.rs`
- `process_watcher.rs` → `process_watching_service.rs`
- `socket_activation.rs` → `socket_activation_service.rs`

---

### Option C: Hybrid (Balance Verbosity & Clarity)

| Current Name | Proposed Name | Rationale |
|--------------|---------------|-----------|
| `ConfigParser` | `ConfigService` | Short & clear |
| `ConflictService` | ✅ Keep as-is | Already good |
| `DependencyService` | ✅ Keep as-is | Already good |
| `HealthMonitor` | `HealthService` | Short & clear |
| `ProcessCreationService` | ✅ Keep as-is | Already good |
| `ProcessLifecycleService` | ✅ Keep as-is | Already good |
| `ProcessSpawnService` | ✅ Keep as-is | Already good |
| `ProcessSupervisor` | `SupervisionService` | Drop "Process" prefix (implied) |
| `ProcessWatcher` | `WatchingService` | Drop "Process" prefix (implied) |
| `SocketActivationManager` | `SocketActivationService` | Manager → Service |

**Rationale for dropping "Process" prefix**:
- All services in `domain/services/` operate on processes
- Reduces redundancy: `ProcessCreationService`, `ProcessLifecycleService`, `ProcessSupervisionService`
- Keeps names shorter while maintaining clarity

---

## Recommendation: Option A (Verbose & Explicit)

### Why Option A?

1. **Clarity Over Brevity**: Explicit names make code self-documenting
2. **Consistency**: All services follow `<Domain><Action>Service` pattern
3. **Searchability**: Full names are easier to grep/search
4. **Onboarding**: New developers understand what each service does
5. **Future-Proof**: Pattern scales as codebase grows

### Trade-offs

**Pros**:
- ✅ Maximum clarity
- ✅ Perfect consistency
- ✅ Self-documenting code
- ✅ Easy to understand for new developers

**Cons**:
- ❌ Longer names (more typing)
- ❌ More verbose imports

**Verdict**: The pros outweigh the cons for a production system.

---

## Special Cases

### Utility Functions (Not Services)

Some files contain utility functions, not services:

| File | Current | Proposed | Type |
|------|---------|----------|------|
| `hook_executor.rs` | `execute_hooks()` | Keep as-is | Utility function |
| `runtime_directory.rs` | `create_runtime_directories()`, `cleanup_runtime_directories()` | Keep as-is | Utility functions |

**Rationale**: These are pure functions without state. They don't need to be services.

**Alternative**: If we want consistency, we could create services:

```rust
// Option 1: Keep as utility functions (recommended)
pub fn execute_hooks(hooks: &[String], stage: &str, process_name: &str) -> Result<(), DomainError>;

// Option 2: Create a service (more verbose, but consistent)
pub struct HookExecutionService;
impl HookExecutionService {
    pub fn execute(&self, hooks: &[String], stage: &str, process_name: &str) -> Result<(), DomainError>;
}
```

**Recommendation**: Keep utility functions as-is. Not everything needs to be a service.

---

## Implementation Plan

### Phase 1: Rename Files & Structs

**Step 1**: Rename files (preserves git history with `git mv`)
```bash
git mv engine/src/domain/services/config_parser.rs \
       engine/src/domain/services/config_parsing_service.rs

git mv engine/src/domain/services/health_monitor.rs \
       engine/src/domain/services/health_monitoring_service.rs

git mv engine/src/domain/services/process_supervisor.rs \
       engine/src/domain/services/process_supervision_service.rs

git mv engine/src/domain/services/process_watcher.rs \
       engine/src/domain/services/process_watching_service.rs

git mv engine/src/domain/services/socket_activation.rs \
       engine/src/domain/services/socket_activation_service.rs

git mv engine/src/domain/services/conflict_service.rs \
       engine/src/domain/services/conflict_resolution_service.rs

git mv engine/src/domain/services/dependency_service.rs \
       engine/src/domain/services/dependency_resolution_service.rs

git mv engine/src/domain/services/process_spawn_service.rs \
       engine/src/domain/services/process_spawning_service.rs
```

**Step 2**: Rename structs in each file
```rust
// Before
pub struct ConfigParser;

// After
pub struct ConfigParsingService;
```

**Step 3**: Update `mod.rs`
```rust
// Before
pub mod config_parser;
pub use config_parser::ConfigParser;

// After
pub mod config_parsing_service;
pub use config_parsing_service::ConfigParsingService;
```

### Phase 2: Update All Imports

**Find all usages**:
```bash
rg "ConfigParser" --type rust
rg "HealthMonitor" --type rust
rg "ProcessSupervisor" --type rust
rg "ProcessWatcher" --type rust
rg "SocketActivationManager" --type rust
rg "ConflictService" --type rust
rg "DependencyService" --type rust
rg "ProcessSpawnService" --type rust
```

**Update imports**:
```rust
// Before
use crate::domain::services::ConfigParser;
use crate::domain::services::HealthMonitor;

// After
use crate::domain::services::ConfigParsingService;
use crate::domain::services::HealthMonitoringService;
```

### Phase 3: Update Tests

Update all test files to use new names.

### Phase 4: Update Documentation

Update:
- README.md
- Architecture docs
- Code comments
- RFC

---

## Migration Script

```bash
#!/bin/bash
# Domain Services Renaming Script

set -e

echo "Phase 1: Renaming files..."
git mv engine/src/domain/services/config_parser.rs engine/src/domain/services/config_parsing_service.rs
git mv engine/src/domain/services/health_monitor.rs engine/src/domain/services/health_monitoring_service.rs
git mv engine/src/domain/services/process_supervisor.rs engine/src/domain/services/process_supervision_service.rs
git mv engine/src/domain/services/process_watcher.rs engine/src/domain/services/process_watching_service.rs
git mv engine/src/domain/services/socket_activation.rs engine/src/domain/services/socket_activation_service.rs
git mv engine/src/domain/services/conflict_service.rs engine/src/domain/services/conflict_resolution_service.rs
git mv engine/src/domain/services/dependency_service.rs engine/src/domain/services/dependency_resolution_service.rs
git mv engine/src/domain/services/process_spawn_service.rs engine/src/domain/services/process_spawning_service.rs

echo "Phase 2: Updating struct names..."
# Use sed to replace struct names in files
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct ConfigParser/pub struct ConfigParsingService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct HealthMonitor/pub struct HealthMonitoringService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct ProcessSupervisor/pub struct ProcessSupervisionService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct ProcessWatcher/pub struct ProcessWatchingService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct SocketActivationManager/pub struct SocketActivationService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct ConflictService/pub struct ConflictResolutionService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct DependencyService/pub struct DependencyResolutionService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/pub struct ProcessSpawnService/pub struct ProcessSpawningService/g' {} +

echo "Phase 3: Updating imports..."
find engine/src -type f -name "*.rs" -exec sed -i 's/use.*ConfigParser/use crate::domain::services::ConfigParsingService/g' {} +
find engine/src -type f -name "*.rs" -exec sed -i 's/use.*HealthMonitor/use crate::domain::services::HealthMonitoringService/g' {} +
# ... more replacements

echo "Phase 4: Running tests..."
cargo test

echo "Phase 5: Running formatter..."
cargo fmt

echo "Phase 6: Running clippy..."
cargo clippy

echo "Done! Please review changes with 'git diff' before committing."
```

---

## After Standardization

### Final Service List

| Service Name | Responsibility | Domain |
|--------------|----------------|--------|
| `ConfigParsingService` | Parse YAML configs into commands | Config |
| `ConflictResolutionService` | Resolve process conflicts | Process |
| `DependencyResolutionService` | Resolve process dependencies | Process |
| `HealthMonitoringService` | Monitor process health checks | Health |
| `ProcessCreationService` | Create and configure process entities | Process |
| `ProcessLifecycleService` | Manage process lifecycle (spawn, hooks, register) | Process |
| `ProcessSpawningService` | Spawn system processes | Process |
| `ProcessSupervisionService` | Supervise running processes (exit handling) | Process |
| `ProcessWatchingService` | Watch for process exit events | Process |
| `SocketActivationService` | Manage socket activation | Socket |

### Utility Functions (Not Services)

| Function | Responsibility |
|----------|----------------|
| `execute_hooks()` | Execute lifecycle hooks |
| `create_runtime_directories()` | Create runtime directories |
| `cleanup_runtime_directories()` | Clean up runtime directories |

---

## Benefits After Standardization

### 1. Predictable Naming

```rust
// Before (inconsistent)
use crate::domain::services::{
    ConfigParser,           // Parser
    HealthMonitor,          // Monitor
    ProcessSupervisor,      // Supervisor
    SocketActivationManager // Manager
};

// After (consistent)
use crate::domain::services::{
    ConfigParsingService,
    HealthMonitoringService,
    ProcessSupervisionService,
    SocketActivationService
};
```

### 2. Clear Responsibility

Service names now clearly indicate what they do:
- `ConfigParsingService` → Parses configs
- `HealthMonitoringService` → Monitors health
- `ConflictResolutionService` → Resolves conflicts
- `DependencyResolutionService` → Resolves dependencies

### 3. Easy Discovery

All services end with `Service`, making them easy to find:
```bash
# Find all domain services
ls engine/src/domain/services/*_service.rs

# Find all service usages
rg "Service" --type rust
```

### 4. Scalable Pattern

Adding new services is straightforward:
```rust
// New service follows same pattern
pub struct ProcessScalingService;      // Scales processes
pub struct ProcessMigrationService;    // Migrates processes
pub struct ProcessBackupService;       // Backs up process state
```

---

## Alternative Patterns Considered

### Pattern 1: Manager Suffix
```rust
pub struct ConfigManager;
pub struct HealthManager;
pub struct ProcessManager;  // Conflicts with top-level name!
```
**Rejected**: "Manager" is too generic and conflicts with existing names.

### Pattern 2: Handler Suffix
```rust
pub struct ConfigHandler;
pub struct HealthHandler;
pub struct ProcessHandler;
```
**Rejected**: "Handler" implies event handling, not business logic.

### Pattern 3: No Suffix
```rust
pub struct Config;
pub struct Health;
pub struct Process;  // Conflicts with entity!
```
**Rejected**: Conflicts with entity names and lacks clarity.

### Pattern 4: Verb-Only Names
```rust
pub struct Parser;
pub struct Monitor;
pub struct Supervisor;
```
**Rejected**: Too generic, unclear what domain they operate on.

---

## Conclusion

**Recommended Approach**: **Option A (Verbose & Explicit)**

**Next Steps**:
1. Review and approve this proposal
2. Run migration script
3. Update tests
4. Update documentation
5. Commit with clear message: `refactor: standardize domain service naming`

**Estimated Effort**: 2-3 hours (mostly automated with script)

**Risk**: Low (mechanical refactoring, caught by compiler)

**Impact**: High (improved code clarity and maintainability)


