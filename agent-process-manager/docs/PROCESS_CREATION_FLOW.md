# Process Creation Flow

This document describes the complete flow for creating a process in the Process Manager, from initial request to running process.

## High-Level Flow Diagram

```mermaid
flowchart TD
    Start([Client Request]) --> Entry{Entry Point}
    
    Entry -->|gRPC API| GRPC[gRPC Adapter<br/>adapters/grpc/service.rs]
    Entry -->|YAML Config| YAML[Config Parser<br/>services/config_parsing_service.rs]
    Entry -->|C FFI| FFI[FFI Wrapper<br/>ffi.rs]
    
    GRPC --> MapGRPC[Map protobuf to<br/>CreateProcessCommand]
    YAML --> MapYAML[Parse YAML to<br/>CreateProcessCommand]
    FFI --> MapFFI[Convert C types to<br/>CreateProcessCommand]
    
    MapGRPC --> UseCase
    MapYAML --> UseCase
    MapFFI --> UseCase
    
    UseCase[CreateProcess Use Case<br/>use_cases/create_process.rs] --> Service[ProcessCreationService<br/>services/process_creation.rs]
    
    Service --> Check{Name<br/>Exists?}
    Check -->|Yes| Error1[Return DuplicateProcess Error]
    Check -->|No| Builder
    
    Builder[Process::builder<br/>entities/process_builder.rs] --> BuildSteps
    
    BuildSteps[Apply Configuration:<br/>• Mandatory: name, command<br/>• Optional: args, restart, env, etc.<br/>• Validate fields] --> Build[build Method]
    
    Build --> Validate{Validation<br/>Passes?}
    Validate -->|No| Error2[Return Validation Error]
    Validate -->|Yes| Entity[Process Entity Created<br/>entities/process.rs]
    
    Entity --> Init[Initialize:<br/>• Generate ProcessId<br/>• Set defaults from constants<br/>• State = Created]
    
    Init --> Save[Save to Repository<br/>infrastructure/in_memory_repository.rs]
    
    Save --> AutoStart{start_behavior<br/>== Automatic?}
    
    AutoStart -->|No| Return1[Return ProcessId & Name]
    AutoStart -->|Yes| Start[Call StartProcess Use Case]
    
    Start --> Spawn[ProcessSpawningService:<br/>• Execute pre-start hooks<br/>• Spawn OS process<br/>• Setup monitoring]
    
    Spawn --> Running[State = Running]
    Running --> Return2[Return ProcessId & Name]
    
    Return1 --> End([Response to Client])
    Return2 --> End
    Error1 --> End
    Error2 --> End
    
    style Entry fill:#e1f5ff
    style UseCase fill:#fff4e1
    style Service fill:#fff4e1
    style Builder fill:#e8f5e9
    style Entity fill:#e8f5e9
    style Save fill:#f3e5f5
    style Start fill:#fff4e1
    style End fill:#e1f5ff
```

## Detailed Step-by-Step Flow

### Step 1: Entry Point (Adapter Layer)

```mermaid
flowchart LR
    subgraph "Entry Points"
        A[gRPC Client] -->|CreateRequest| B[gRPC Adapter]
        C[YAML Config] -->|auto_start: true| D[Config Parser]
        E[C/Go Client] -->|pm_create| F[FFI Wrapper]
    end
    
    B --> G[CreateProcessCommand]
    D --> G
    F --> G
    
    style G fill:#fff4e1
```

**Responsibilities:**
- Receive external request (gRPC, YAML, FFI)
- Validate input format
- Map to domain `CreateProcessCommand`

### Step 2: Use Case Execution (Application Layer)

```mermaid
flowchart TD
    A[CreateProcessCommand] --> B[CreateProcess Use Case]
    B --> C{Business Rules}
    C -->|Valid| D[Call ProcessCreationService]
    C -->|Invalid| E[Return Error]
    
    style B fill:#fff4e1
```

**Responsibilities:**
- Orchestrate the creation flow
- Enforce business rules
- Coordinate between services

### Step 3: Domain Service (Domain Layer)

```mermaid
flowchart TD
    A[ProcessCreationService] --> B[Check Name Uniqueness]
    B -->|Exists| C[DuplicateProcess Error]
    B -->|Available| D[Build Process Entity]
    D --> E[Save to Repository]
    E --> F[Return ProcessId]
    
    style A fill:#fff4e1
    style D fill:#e8f5e9
```

**Responsibilities:**
- Validate business constraints
- Build process entity
- Persist to repository

### Step 4: Builder Pattern (Domain Layer)

```mermaid
flowchart TD
    A[Process::builder<br/>name, command] --> B[ProcessBuilder]
    
    B --> C[.args]
    C --> D[.restart_policy]
    D --> E[.restart_delay_sec]
    E --> F[.env]
    F --> G[.resource_limits]
    G --> H[... more config ...]
    H --> I[.build]
    
    I --> J{Validate}
    J -->|Invalid| K[ValidationError]
    J -->|Valid| L[Process::new]
    
    L --> M[Initialize:<br/>• ProcessId::generate<br/>• Set defaults<br/>• State = Created]
    
    M --> N[Process Entity]
    
    style B fill:#e8f5e9
    style L fill:#e8f5e9
    style N fill:#e8f5e9
```

**Responsibilities:**
- Fluent interface for configuration
- Distinguish mandatory vs optional fields
- Validate configuration
- Create Process entity with defaults

### Step 5: Auto-Start (Optional)

```mermaid
flowchart TD
    A[Process Created] --> B{start_behavior?}
    B -->|Manual| C[Return ProcessId<br/>State = Created]
    B -->|Automatic| D[StartProcess Use Case]
    
    D --> E[ProcessSpawningService]
    E --> F[Execute pre-start hooks]
    F --> G[Spawn OS process]
    G --> H[Setup monitoring]
    H --> I[Return ProcessId<br/>State = Running]
    
    style D fill:#fff4e1
    style E fill:#fff4e1
```

**Responsibilities:**
- Conditionally start process
- Execute lifecycle hooks
- Spawn actual OS process
- Setup health monitoring

## Architecture Layers

```mermaid
flowchart TD
    subgraph "Adapter Layer"
        A1[gRPC Service]
        A2[Config Parser]
        A3[FFI Wrapper]
    end
    
    subgraph "Application Layer"
        B1[CreateProcess Use Case]
        B2[StartProcess Use Case]
    end
    
    subgraph "Domain Layer"
        C1[ProcessCreationService]
        C2[ProcessBuilder]
        C3[Process Entity]
        C4[ProcessSpawningService]
    end
    
    subgraph "Infrastructure Layer"
        D1[ProcessRepository]
        D2[TokioExecutor]
    end
    
    A1 --> B1
    A2 --> B1
    A3 --> B1
    
    B1 --> C1
    B2 --> C4
    
    C1 --> C2
    C2 --> C3
    C1 --> D1
    C4 --> D2
    
    style A1 fill:#e1f5ff
    style A2 fill:#e1f5ff
    style A3 fill:#e1f5ff
    style B1 fill:#fff4e1
    style B2 fill:#fff4e1
    style C1 fill:#e8f5e9
    style C2 fill:#e8f5e9
    style C3 fill:#e8f5e9
    style C4 fill:#e8f5e9
    style D1 fill:#f3e5f5
    style D2 fill:#f3e5f5
```

## Key Components

### CreateProcessCommand (DTO)
```rust
pub struct CreateProcessCommand {
    // Mandatory
    pub name: String,
    pub command: String,
    
    // Optional
    pub args: Vec<String>,
    pub restart: Option<RestartPolicy>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay_sec: Option<u64>,
    pub start_behavior: StartBehavior,
    // ... 40+ more fields
}
```

### ProcessBuilder (Fluent Interface)
```rust
Process::builder("web-server", "/usr/bin/nginx")
    .args(vec!["-g".to_string(), "daemon off;".to_string()])
    .restart_policy(RestartPolicy::Always)
    .restart_delay_sec(5)
    .resource_limits(limits)
    .start_behavior(StartBehavior::Automatic)
    .build()?
```

### Process Entity (Domain Model)
```rust
pub struct Process {
    id: ProcessId,
    name: String,
    command: String,
    state: ProcessState,
    // ... 50+ more fields
}
```

## Error Handling Flow

```mermaid
flowchart TD
    A[Request] --> B{Validation}
    B -->|Invalid Format| C[Return 400 Bad Request]
    B -->|Valid| D{Name Check}
    D -->|Duplicate| E[Return 409 Conflict]
    D -->|Unique| F{Build Process}
    F -->|Validation Error| G[Return 400 Bad Request]
    F -->|Success| H{Save}
    H -->|Repository Error| I[Return 500 Internal Error]
    H -->|Success| J{Auto-Start}
    J -->|Start Failed| K[Return 500 Internal Error]
    J -->|Success| L[Return 200 OK]
    
    style C fill:#ffcdd2
    style E fill:#ffcdd2
    style G fill:#ffcdd2
    style I fill:#ffcdd2
    style K fill:#ffcdd2
    style L fill:#c8e6c9
```

## Performance Considerations

1. **Uniqueness Check**: O(1) lookup in repository
2. **Builder Pattern**: Zero-cost abstraction (compile-time)
3. **Validation**: Happens once during build
4. **Repository Save**: Async operation, non-blocking
5. **Auto-Start**: Optional, happens asynchronously

## Recent Improvements (Refactoring)

### Phase 1: Builder Pattern
- **Before**: `Process::new()` + 12 setter methods
- **After**: Fluent builder with compile-time validation
- **Benefit**: Clearer API, shorter code

### Phase 2: Consistent Naming
- **Before**: `restart_max_delay` (no unit suffix)
- **After**: `restart_max_delay_sec` (clear units)
- **Benefit**: Self-documenting, no ambiguity

### Phase 3: Boolean Trap Fix
- **Before**: `auto_start: bool` (unclear intent)
- **After**: `start_behavior: StartBehavior` enum
- **Benefit**: Type-safe, extensible, clear intent

## Example: Complete Flow

```mermaid
sequenceDiagram
    participant Client
    participant gRPC as gRPC Adapter
    participant UseCase as CreateProcess UseCase
    participant Service as ProcessCreationService
    participant Builder as ProcessBuilder
    participant Entity as Process Entity
    participant Repo as Repository
    participant Start as StartProcess UseCase
    
    Client->>gRPC: CreateRequest(name="web", cmd="/usr/bin/nginx")
    gRPC->>gRPC: Map protobuf → CreateProcessCommand
    gRPC->>UseCase: execute(command)
    UseCase->>Service: create_from_command(command)
    Service->>Repo: exists_by_name("web")?
    Repo-->>Service: false
    Service->>Builder: Process::builder("web", "/usr/bin/nginx")
    Builder->>Builder: .args(["-g", "daemon off;"])
    Builder->>Builder: .restart_policy(Always)
    Builder->>Builder: .start_behavior(Automatic)
    Builder->>Builder: .build()
    Builder->>Entity: Process::new(validated_config)
    Entity-->>Builder: Process { id, state: Created }
    Builder-->>Service: Process entity
    Service->>Repo: save(process)
    Repo-->>Service: Ok(())
    Service-->>UseCase: Ok((id, name))
    UseCase->>gRPC: CreateProcessResponse { id, name }
    gRPC->>gRPC: Check start_behavior == Automatic
    gRPC->>Start: execute(StartProcessCommand)
    Start->>Entity: spawn_process()
    Entity-->>Start: Ok(pid)
    Start-->>gRPC: Ok(state: Running)
    gRPC-->>Client: CreateResponse { id, state: Running }
```

## See Also

- [Architecture Overview](../ARCHITECTURE.md)
- [Use Cases](../USE_CASES.md)
- [Builder Pattern RFC](../RFC_PROCESS_MANAGER.md)
- [Code Smells Analysis](../CODE_SMELLS_CREATE_PROCESS.md)

