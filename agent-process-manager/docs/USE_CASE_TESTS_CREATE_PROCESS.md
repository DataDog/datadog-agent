# CreateProcess Use Case - Test Scenarios

## Overview

The CreateProcess use case is responsible for registering a new process definition in the system. It validates the input, ensures uniqueness, and persists the process entity to the repository.

---

## Happy Path Scenarios

### Scenario 1: Create Process with Minimal Configuration

**Description**: Create a process with only required fields (name and command)

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with minimal configuration
let command = CreateProcessCommand {
    name: "my-service".to_string(),
    command: "/usr/bin/myapp".to_string(),
    args: vec![],
    env: HashMap::new(),
    working_dir: None,
    user: None,
    group: None,
    restart_policy: RestartPolicy::Never,
    resource_limits: ResourceLimits::default(),
    health_check: None,
    requires: vec![],
    wants: vec![],
    binds_to: vec![],
    after: vec![],
    before: vec![],
    conflicts: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());
let response = result.unwrap();

// And: The response contains the process name
assert_eq!(response.name, "my-service");

// And: The response contains a valid process ID
assert!(!response.id.to_string().is_empty());

// And: The process is saved in the repository
let found = repo.find_by_id(&response.id).await.unwrap();
assert!(found.is_some());

// And: The process has correct attributes
let process = found.unwrap();
assert_eq!(process.name(), "my-service");
assert_eq!(process.command(), "/usr/bin/myapp");
assert_eq!(process.state(), ProcessState::Created);
assert_eq!(process.run_count(), 0);
assert_eq!(process.pid(), None);
```

**Expected Outcome**: Process created with default values for optional fields

---

### Scenario 2: Create Process with Full Configuration

**Description**: Create a process with all available configuration options

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with full configuration
let mut env = HashMap::new();
env.insert("LOG_LEVEL".to_string(), "debug".to_string());
env.insert("APP_PORT".to_string(), "8080".to_string());

let command = CreateProcessCommand {
    name: "full-featured-service".to_string(),
    command: "/opt/myapp/bin/server".to_string(),
    args: vec!["--config".to_string(), "/etc/myapp/config.yaml".to_string()],
    env: env.clone(),
    environment_file: Some("/etc/myapp/env".to_string()),
    working_dir: Some("/opt/myapp".to_string()),
    user: Some("appuser".to_string()),
    group: Some("appgroup".to_string()),
    restart_policy: RestartPolicy::OnFailure,
    restart_sec: 10,
    restart_max_delay: 300,
    timeout_start_sec: 60,
    timeout_stop_sec: 30,
    resource_limits: ResourceLimits {
        cpu_millis: Some(2000),      // 2 cores
        memory_bytes: Some(1073741824), // 1GB
        pids_limit: Some(500),
    },
    health_check: Some(HealthCheck {
        check_type: HealthCheckType::Http,
        endpoint: "http://localhost:8080/health".to_string(),
        interval_sec: 30,
        timeout_sec: 5,
        retries: 3,
    }),
    requires: vec!["database".to_string()],
    wants: vec!["cache".to_string()],
    after: vec!["database".to_string(), "cache".to_string()],
    conflicts: vec!["legacy-service".to_string()],
    runtime_directory: vec!["/run/myapp".to_string()],
    pidfile: Some("/var/run/myapp.pid".to_string()),
    ambient_capabilities: vec!["CAP_NET_BIND_SERVICE".to_string()],
    kill_mode: KillMode::ProcessGroup,
    kill_signal: "SIGTERM".to_string(),
    success_exit_status: vec![0, 143],
    start_limit_burst: 5,
    start_limit_interval: 60,
    condition_path_exists: vec!["/opt/myapp/bin/server".to_string()],
    exec_start_pre: vec!["/opt/myapp/scripts/pre-start.sh".to_string()],
    exec_start_post: vec!["/opt/myapp/scripts/post-start.sh".to_string()],
    exec_stop_post: vec!["/opt/myapp/scripts/cleanup.sh".to_string()],
    auto_start: true,
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());
let response = result.unwrap();

// And: The process is saved in the repository
let found = repo.find_by_name("full-featured-service").await.unwrap();
assert!(found.is_some());

// And: All configuration fields are correctly set
let process = found.unwrap();
assert_eq!(process.name(), "full-featured-service");
assert_eq!(process.command(), "/opt/myapp/bin/server");
assert_eq!(process.args(), &["--config", "/etc/myapp/config.yaml"]);
assert_eq!(process.env().get("LOG_LEVEL"), Some(&"debug".to_string()));
assert_eq!(process.environment_file(), Some("/etc/myapp/env"));
assert_eq!(process.working_dir(), Some("/opt/myapp"));
assert_eq!(process.user(), Some("appuser"));
assert_eq!(process.group(), Some("appgroup"));
assert_eq!(process.restart_policy(), RestartPolicy::OnFailure);
assert_eq!(process.restart_sec(), 10);
assert_eq!(process.restart_max_delay(), 300);
assert_eq!(process.timeout_start_sec(), 60);
assert_eq!(process.timeout_stop_sec(), 30);

// And: Resource limits are set
let limits = process.resource_limits();
assert_eq!(limits.cpu_millis, Some(2000));
assert_eq!(limits.memory_bytes, Some(1073741824));
assert_eq!(limits.pids_limit, Some(500));

// And: Health check is configured
let health_check = process.health_check();
assert!(health_check.is_some());
let hc = health_check.unwrap();
assert_eq!(hc.check_type, HealthCheckType::Http);
assert_eq!(hc.endpoint, "http://localhost:8080/health");
assert_eq!(hc.interval_sec, 30);

// And: Dependencies are set
assert_eq!(process.requires(), &["database"]);
assert_eq!(process.wants(), &["cache"]);
assert_eq!(process.after(), &["database", "cache"]);
assert_eq!(process.conflicts(), &["legacy-service"]);

// And: Security settings are set
assert_eq!(process.ambient_capabilities(), &["CAP_NET_BIND_SERVICE"]);
assert_eq!(process.kill_mode(), KillMode::ProcessGroup);
assert_eq!(process.kill_signal(), "SIGTERM");

// And: Runtime settings are set
assert_eq!(process.runtime_directory(), &["/run/myapp"]);
assert_eq!(process.pidfile(), Some("/var/run/myapp.pid"));

// And: Start limit is configured
assert_eq!(process.start_limit_burst(), 5);
assert_eq!(process.start_limit_interval(), 60);

// And: Hooks are configured
assert_eq!(process.exec_start_pre(), &["/opt/myapp/scripts/pre-start.sh"]);
assert_eq!(process.exec_start_post(), &["/opt/myapp/scripts/post-start.sh"]);
assert_eq!(process.exec_stop_post(), &["/opt/myapp/scripts/cleanup.sh"]);
```

**Expected Outcome**: Process created with all configuration preserved

---

### Scenario 3: Create Multiple Processes with Different Names

**Description**: Verify that multiple processes can be created without conflicts

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: Multiple process commands
let process_names = vec!["service-1", "service-2", "service-3"];
```

**Act**
```rust
// When: Creating multiple processes
let mut responses = Vec::new();
for name in &process_names {
    let command = CreateProcessCommand {
        name: name.to_string(),
        command: format!("/usr/bin/{}", name),
        args: vec![],
        ..Default::default()
    };
    
    let result = use_case.execute(command).await;
    assert!(result.is_ok());
    responses.push(result.unwrap());
}
```

**Assert**
```rust
// Then: All processes are created successfully
assert_eq!(responses.len(), 3);

// And: Each process has a unique ID
let ids: Vec<_> = responses.iter().map(|r| r.id).collect();
assert_eq!(ids.len(), 3);
assert_ne!(ids[0], ids[1]);
assert_ne!(ids[1], ids[2]);
assert_ne!(ids[0], ids[2]);

// And: All processes are in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 3);

// And: Each process has the correct name
let names: Vec<_> = all_processes.iter().map(|p| p.name()).collect();
assert!(names.contains(&"service-1"));
assert!(names.contains(&"service-2"));
assert!(names.contains(&"service-3"));
```

**Expected Outcome**: Multiple processes created without conflicts

---

### Scenario 4: Create Process with Special Characters in Name (Valid)

**Description**: Create a process with valid special characters (hyphens, underscores)

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with special characters in name
let command = CreateProcessCommand {
    name: "my-service_v2.0".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());
let response = result.unwrap();
assert_eq!(response.name, "my-service_v2.0");

// And: The process is retrievable by name
let found = repo.find_by_name("my-service_v2.0").await.unwrap();
assert!(found.is_some());
```

**Expected Outcome**: Process created with special characters in name

---

## Error Path Scenarios

### Error Scenario 1: Empty Process Name

**Description**: Attempt to create a process with an empty name

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with empty name
let command = CreateProcessCommand {
    name: "".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidName
match result.unwrap_err() {
    DomainError::InvalidName(msg) => {
        assert!(msg.contains("empty") || msg.contains("blank"));
    }
    _ => panic!("Expected InvalidName error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidName error

---

### Error Scenario 2: Process Name with Whitespace

**Description**: Attempt to create a process with whitespace in the name

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with whitespace in name
let command = CreateProcessCommand {
    name: "my service".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidName
match result.unwrap_err() {
    DomainError::InvalidName(msg) => {
        assert!(msg.contains("whitespace") || msg.contains("space"));
    }
    _ => panic!("Expected InvalidName error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidName error

---

### Error Scenario 3: Empty Command Path

**Description**: Attempt to create a process with an empty command

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with empty command path
let command = CreateProcessCommand {
    name: "valid-name".to_string(),
    command: "".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidCommand
match result.unwrap_err() {
    DomainError::InvalidCommand(msg) => {
        assert!(msg.contains("empty") || msg.contains("command"));
    }
    _ => panic!("Expected InvalidCommand error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidCommand error

---

### Error Scenario 4: Duplicate Process Name

**Description**: Attempt to create a process with a name that already exists

**Arrange**
```rust
// Given: A repository with an existing process
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: An existing process named "duplicate"
let first_command = CreateProcessCommand {
    name: "duplicate".to_string(),
    command: "/usr/bin/app1".to_string(),
    args: vec![],
    ..Default::default()
};
let first_result = use_case.execute(first_command).await;
assert!(first_result.is_ok());

// And: A second command with the same name
let second_command = CreateProcessCommand {
    name: "duplicate".to_string(),
    command: "/usr/bin/app2".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Attempting to create the duplicate process
let result = use_case.execute(second_command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is DuplicateProcess
match result.unwrap_err() {
    DomainError::DuplicateProcess(name) => {
        assert_eq!(name, "duplicate");
    }
    _ => panic!("Expected DuplicateProcess error"),
}

// And: Only one process exists in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 1);

// And: The original process is unchanged
let found = repo.find_by_name("duplicate").await.unwrap().unwrap();
assert_eq!(found.command(), "/usr/bin/app1");
```

**Expected Outcome**: Operation fails with DuplicateProcess error

---

### Error Scenario 5: Invalid Runtime Directory Path

**Description**: Attempt to create a process with an invalid runtime directory

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with absolute path in runtime_directory (not allowed)
let command = CreateProcessCommand {
    name: "invalid-runtime".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    runtime_directory: vec!["/absolute/path".to_string()],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidCommand
match result.unwrap_err() {
    DomainError::InvalidCommand(msg) => {
        assert!(msg.contains("runtime") || msg.contains("absolute"));
    }
    _ => panic!("Expected InvalidCommand error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidCommand error

---

### Error Scenario 6: Process Name Too Long

**Description**: Attempt to create a process with an excessively long name

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with a very long name (> 255 characters)
let long_name = "a".repeat(300);
let command = CreateProcessCommand {
    name: long_name.clone(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidName
match result.unwrap_err() {
    DomainError::InvalidName(msg) => {
        assert!(msg.contains("long") || msg.contains("length"));
    }
    _ => panic!("Expected InvalidName error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidName error

---

### Error Scenario 7: Invalid Capability Name

**Description**: Attempt to create a process with an invalid Linux capability

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with invalid capability
let command = CreateProcessCommand {
    name: "invalid-cap".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ambient_capabilities: vec!["CAP_INVALID_CAPABILITY".to_string()],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidCommand
match result.unwrap_err() {
    DomainError::InvalidCommand(msg) => {
        assert!(msg.contains("capability") || msg.contains("CAP_"));
    }
    _ => panic!("Expected InvalidCommand error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidCommand error

---

### Error Scenario 8: Negative Resource Limits

**Description**: Attempt to create a process with negative resource limits

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with negative CPU limit (if type allows, or zero)
let command = CreateProcessCommand {
    name: "negative-limits".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    resource_limits: ResourceLimits {
        cpu_millis: Some(0),  // Zero is invalid
        memory_bytes: Some(0), // Zero is invalid
        pids_limit: Some(0),   // Zero is invalid
    },
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The operation fails
assert!(result.is_err());

// And: The error is InvalidCommand
match result.unwrap_err() {
    DomainError::InvalidCommand(msg) => {
        assert!(msg.contains("resource") || msg.contains("limit") || msg.contains("positive"));
    }
    _ => panic!("Expected InvalidCommand error"),
}

// And: No process is created in the repository
let all_processes = repo.find_all().await.unwrap();
assert_eq!(all_processes.len(), 0);
```

**Expected Outcome**: Operation fails with InvalidCommand error

---

## Edge Case Scenarios

### Edge Case 1: Create Process with Maximum Valid Name Length

**Description**: Create a process with a name at the maximum allowed length (255 chars)

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with maximum length name
let max_name = "a".repeat(255);
let command = CreateProcessCommand {
    name: max_name.clone(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());
let response = result.unwrap();
assert_eq!(response.name.len(), 255);

// And: The process is retrievable
let found = repo.find_by_name(&max_name).await.unwrap();
assert!(found.is_some());
```

**Expected Outcome**: Process created successfully with maximum name length

---

### Edge Case 2: Create Process with Empty Args Array

**Description**: Verify that empty args array is valid

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with empty args
let command = CreateProcessCommand {
    name: "no-args".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());

// And: The process has empty args
let found = repo.find_by_name("no-args").await.unwrap().unwrap();
assert_eq!(found.args().len(), 0);
```

**Expected Outcome**: Process created successfully with empty args

---

### Edge Case 3: Create Process with Very Large Args Array

**Description**: Create a process with many command-line arguments

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with 100 arguments
let args: Vec<String> = (0..100).map(|i| format!("arg{}", i)).collect();
let command = CreateProcessCommand {
    name: "many-args".to_string(),
    command: "/usr/bin/app".to_string(),
    args: args.clone(),
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());

// And: All args are preserved
let found = repo.find_by_name("many-args").await.unwrap().unwrap();
assert_eq!(found.args().len(), 100);
assert_eq!(found.args()[0], "arg0");
assert_eq!(found.args()[99], "arg99");
```

**Expected Outcome**: Process created successfully with all arguments preserved

---

### Edge Case 4: Create Process with Empty Environment Map

**Description**: Verify that empty environment map is valid

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command with empty environment
let command = CreateProcessCommand {
    name: "no-env".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    env: HashMap::new(),
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created successfully
assert!(result.is_ok());

// And: The process has empty environment
let found = repo.find_by_name("no-env").await.unwrap().unwrap();
assert_eq!(found.env().len(), 0);
```

**Expected Outcome**: Process created successfully with empty environment

---

### Edge Case 5: Create Process with Circular Dependency Reference

**Description**: Attempt to create a process that references itself in dependencies

**Arrange**
```rust
// Given: An empty repository
let repo = Arc::new(MockRepository::new());
let creation_service = Arc::new(ProcessCreationService::new(repo.clone()));
let use_case = CreateProcessUseCase::new(creation_service);

// And: A command that requires itself
let command = CreateProcessCommand {
    name: "self-dependent".to_string(),
    command: "/usr/bin/app".to_string(),
    args: vec![],
    requires: vec!["self-dependent".to_string()],
    ..Default::default()
};
```

**Act**
```rust
// When: Executing the use case
let result = use_case.execute(command).await;
```

**Assert**
```rust
// Then: The process is created (validation happens at start time)
// Note: Circular dependency detection occurs during StartProcess, not CreateProcess
assert!(result.is_ok());

// And: The dependency is recorded
let found = repo.find_by_name("self-dependent").await.unwrap().unwrap();
assert_eq!(found.requires(), &["self-dependent"]);
```

**Expected Outcome**: Process created (circular dependency detected at start time)

---

## Test Summary

### Happy Path Coverage
- ✅ Minimal configuration
- ✅ Full configuration with all fields
- ✅ Multiple processes
- ✅ Special characters in name

### Error Path Coverage
- ✅ Empty name
- ✅ Name with whitespace
- ✅ Empty command
- ✅ Duplicate name
- ✅ Invalid runtime directory
- ✅ Name too long
- ✅ Invalid capability
- ✅ Invalid resource limits

### Edge Case Coverage
- ✅ Maximum name length
- ✅ Empty args array
- ✅ Large args array
- ✅ Empty environment
- ✅ Self-referential dependency

---

## Implementation Notes

### Validation Order
1. **Name validation** (empty, whitespace, length)
2. **Command validation** (empty)
3. **Uniqueness check** (duplicate name)
4. **Configuration validation** (runtime dirs, capabilities, resource limits)
5. **Entity creation** (delegate to ProcessCreationService)
6. **Persistence** (save to repository)

### Error Handling Strategy
- **Fail fast**: Validate all inputs before creating entity
- **Atomic**: Either create completely or fail completely (no partial state)
- **Descriptive errors**: Include context (field name, constraint violated)
- **Idempotent**: Repository operations are idempotent

### Performance Considerations
- **O(1) lookup** for duplicate name check (hash-based repository)
- **No blocking I/O** during validation (all in-memory)
- **Minimal allocations** (reuse command fields where possible)

---

## Related Use Cases

- **StartProcess**: Depends on CreateProcess (must exist before starting)
- **LoadConfig**: Bulk creates processes from YAML
- **UpdateProcess**: Modifies existing process (requires CreateProcess first)
- **DeleteProcess**: Removes process created by CreateProcess


