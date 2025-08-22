# USM-RS: Universal Service Metadata Detection Library

A Rust implementation of the Universal Service Metadata (USM) detection library for the Datadog Agent. This library detects service names and metadata from running processes by analyzing command-line arguments, environment variables, and filesystem contents.

## Features

- **Multi-language support**: Detects services for Java, Python, Node.js, PHP, Ruby, .NET, and more
- **Framework detection**: Specialized detectors for Spring Boot, Rails, Laravel, Gunicorn, and Java EE servers
- **Environment integration**: Extracts DD_SERVICE and related metadata from environment variables
- **Filesystem abstraction**: Supports real filesystem and in-memory testing
- **FFI interface**: C-compatible API for Go integration
- **Memory safe**: Leverages Rust's safety guarantees to prevent common bugs

## Architecture

The library consists of several key components:

- **Core types**: `ServiceMetadata`, `DetectionContext`, `Language` enums
- **Detectors**: Language-specific service detection logic
- **Frameworks**: Framework-specific detection (Spring, Rails, etc.)
- **Utilities**: Path handling, normalization, file parsing
- **Filesystem**: Abstracted file operations for testability

## Usage

### Rust API

```rust
use usm_rs::{
    context::{DetectionContext, Environment},
    filesystem::RealFileSystem,
    language::Language,
    service::extract_service_metadata,
};
use std::sync::Arc;

let fs = Arc::new(RealFileSystem::new());
let args = vec!["java".to_string(), "-jar".to_string(), "myapp.jar".to_string()];
let env = Environment::new();

let mut ctx = DetectionContext::new(args, env, fs);
let metadata = extract_service_metadata(Language::Java, &mut ctx)?;

println!("Service: {}", metadata.name);
```

### C FFI API

```c
#include "usm_rs.h"

const char* args[] = {"java", "-jar", "myapp.jar"};
const char* envs[] = {"DD_SERVICE", "my-service"};

CServiceMetadata* metadata = usm_extract_service_metadata(
    USM_LANG_JAVA,
    1234, // PID
    args, 3,
    envs, 2
);

if (metadata) {
    printf("Service: %s\n", metadata->name);
    usm_free_service_metadata(metadata);
}
```

## Building

### Development Build

```bash
cargo build
```

### Release Build

```bash
cargo build --release
```

### Running Tests

```bash
cargo test
```

### Features

- `yaml`: YAML configuration file parsing
- `xml`: XML configuration file parsing  
- `properties`: Java properties file parsing

## Integration with Datadog Agent

This Rust library is designed to replace the existing Go implementation in the Datadog Agent. It provides a C FFI interface that can be called from Go code, allowing for gradual migration.

## Current Status

- ✅ Core architecture and types
- ✅ Filesystem abstraction  
- ✅ Utility functions and normalization
- ✅ Simple and .NET detectors
- ✅ FFI interface for Go integration
- ✅ Java detector (Spring Boot, JEE servers)
- ✅ Python detector (Gunicorn, Django)  
- ✅ Node.js detector (package.json)
- ❌ PHP and Ruby detectors - **TODO**

## Performance Benefits

- **Zero-cost abstractions**: Compile-time optimizations with no runtime overhead
- **Memory safety**: Eliminates buffer overflows and memory leaks
- **No garbage collection**: Predictable performance without GC pauses
- **Optimized binary**: Aggressive compiler optimizations in release mode

## License

Licensed under the Apache License Version 2.0, consistent with the Datadog Agent.