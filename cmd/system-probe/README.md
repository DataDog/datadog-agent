# System-Probe Dual Mode Wrapper

This directory contains an innovative dual-mode implementation that provides both system-probe and service-discovery functionality through a single lightweight wrapper binary.

## Overview

The system-probe binary has been redesigned to consume minimal memory by default while retaining the ability to dynamically load full system-probe functionality when needed. This is achieved through:

- **Lightweight C wrapper** (~17KB) that serves as the main binary
- **Dynamic library loading** based on environment variable detection
- **Two modes of operation**: service-discovery (default) and full system-probe

## Architecture

```
system-probe (17KB C wrapper)
├── Default: libservicediscovery.so (32MB)
└── DD_SYSTEM_PROBE_*: libsystemprobe.so (86MB)
```

### Components

1. **`wrapper/main.c`** - Lightweight C wrapper that determines which mode to run
2. **`library/main.go`** - CGO-exportable version of system-probe 
3. **`service-discovery/library/main.go`** - CGO-exportable version of service-discovery
4. **Build system integration** in `tasks/system_probe.py`

## Modes of Operation

### Service-Discovery Mode (Default)

**When**: No `DD_SYSTEM_PROBE_*` environment variables are set or they contain empty values

**Memory usage**: ~1.3MB RSS

**Functionality**: Standalone service discovery with HTTP API endpoints

**Example**:
```bash
# Default mode - runs service-discovery
./bin/system-probe/system-probe --help
./bin/system-probe/system-probe -socket /tmp/discovery.sock
```

### System-Probe Mode

**When**: Any `DD_SYSTEM_PROBE_*` environment variable is set with a non-empty value

**Memory usage**: Full system-probe memory footprint

**Functionality**: Complete system-probe with all modules and capabilities

**Examples**:
```bash
# Any DD_SYSTEM_PROBE_* variable triggers full system-probe
DD_SYSTEM_PROBE_ENABLED=1 ./bin/system-probe/system-probe version
DD_SYSTEM_PROBE_DEBUG=true ./bin/system-probe/system-probe run
DD_SYSTEM_PROBE_CONFIG_DIR=/etc ./bin/system-probe/system-probe config
```

## Environment Variable Detection

The wrapper checks for any environment variable starting with `DD_SYSTEM_PROBE_` and containing a non-empty value:

- ✅ `DD_SYSTEM_PROBE_ENABLED=1` → system-probe mode
- ✅ `DD_SYSTEM_PROBE_DEBUG=true` → system-probe mode
- ✅ `DD_SYSTEM_PROBE_CONFIG_DIR=/tmp` → system-probe mode
- ❌ `DD_SYSTEM_PROBE_ENABLED=` → service-discovery mode (empty value)
- ❌ `OTHER_VARIABLE=1` → service-discovery mode (wrong prefix)

## Building

### Build All Components
```bash
source .venv/bin/activate
dda inv system-probe.build-dual-wrapper
```

### Build Individual Components
```bash
# System-probe shared library only
dda inv system-probe.build-shared-library

# Service-discovery shared library only  
dda inv system-probe.build-service-discovery-library

# C wrapper only
dda inv system-probe.build-dual-wrapper-binary
```

### Standalone Service-Discovery Binary
You can also build the service-discovery as a standalone binary (separate from the dual-mode wrapper):

```bash
# Build standalone service-discovery binary
go build -tags "linux,grpcnotrace" -o bin/service-discovery cmd/service-discovery/main.go

# Or with additional build flags
go build -tags "linux,grpcnotrace" \
  -ldflags "-s -w" \
  -trimpath \
  -o bin/service-discovery \
  cmd/service-discovery/main.go
```

**Important**: Always include the `grpcnotrace` build tag when building service-discovery to avoid tracing overhead and reduce binary size.

#### Standalone Service-Discovery Usage
```bash
# Run with default settings
./bin/service-discovery

# Specify custom socket path
./bin/service-discovery -socket /tmp/custom-discovery.sock

# Specify configuration file
./bin/service-discovery -config /path/to/config.yaml

# Show help
./bin/service-discovery -h
```

The standalone binary provides the same functionality as the service-discovery library but as an independent executable (~25-30MB) rather than being dynamically loaded by the wrapper.

### Traditional System-Probe Build
```bash
# Original system-probe build (for compatibility)
dda inv system-probe.build
```

## File Structure

```
cmd/system-probe/
├── README.md                 # This file
├── main.go                   # Original system-probe entry point
├── wrapper/
│   └── main.c               # Lightweight C wrapper (17KB binary)
├── library/
│   └── main.go              # CGO-exportable system-probe
└── service-discovery/
    └── library/
        └── main.go          # CGO-exportable service-discovery

bin/
├── service-discovery        # Standalone service-discovery binary (~20MB)
└── system-probe/
    ├── system-probe             # Main binary (C wrapper)
    ├── system-probe-wrapper     # Same as above
    ├── libsystemprobe.so        # System-probe shared library (86MB)
    └── libservicediscovery.so   # Service-discovery shared library (32MB)
```

## Performance Comparison

| Mode | Binary Size | Memory Usage | Loaded Libraries |
|------|------------|-------------|------------------|
| **Dual-Mode Wrapper (Service-Discovery)** | 17KB | ~1.3MB RSS | libservicediscovery.so |
| **Dual-Mode Wrapper (System-Probe)** | 17KB | Full system-probe | libsystemprobe.so |
| **Standalone Service-Discovery** | ~20MB | ~15-20MB RSS | N/A |
| **Original System-Probe** | ~120MB | Full system-probe | N/A |

**Memory savings**: ~99% reduction in default operation (dual-mode wrapper)

## Deployment Options

You have multiple options for deploying service-discovery functionality:

### Option 1: Dual-Mode Wrapper (Recommended)
**Best for**: Environments where you might need both service-discovery and system-probe functionality
- **Ultra-lightweight**: 17KB wrapper + 32MB library (loaded only when needed)
- **Flexible**: Automatic mode switching based on environment variables
- **Future-proof**: Can easily enable full system-probe by setting environment variables

### Option 2: Standalone Service-Discovery Binary  
**Best for**: Pure service-discovery deployments where system-probe will never be needed
- **Self-contained**: Single binary with no external dependencies
- **Simpler deployment**: No shared libraries to manage
- **Moderate size**: ~20MB binary vs 17KB + 32MB library

### Option 3: Original System-Probe (Legacy)
**Best for**: Environments requiring full system-probe functionality by default
- **Traditional**: Original monolithic approach
- **Largest**: ~120MB binary with full functionality always loaded

### Choosing the Right Option

| Use Case | Recommended Option | Rationale |
|----------|-------------------|-----------|
| **Cloud containers** | Dual-Mode Wrapper | Minimal base image size, load functionality on demand |
| **Kubernetes sidecar** | Dual-Mode Wrapper | Ultra-low resource usage by default |
| **Dedicated service-discovery** | Standalone Binary | Simpler deployment, no library management |
| **Development/testing** | Dual-Mode Wrapper | Easy to switch between modes for testing |
| **Legacy deployments** | Original System-Probe | Maintain existing deployment patterns |

## Help System

The wrapper provides mode-appropriate help:

### Service-Discovery Help
```bash
$ ./bin/system-probe/system-probe --help
Datadog Service Discovery (Lightweight)
Usage: ./bin/system-probe/system-probe [options]
Options:
  -h, --help               show this help message
  -socket PATH             Unix socket path
  -config PATH             Path to configuration file

Environment Variables:
  DD_SYSTEM_PROBE_*        any DD_SYSTEM_PROBE_ variable enables full system-probe
                          (default: service-discovery mode)

To see full system-probe options, set a DD_SYSTEM_PROBE_ variable:
  DD_SYSTEM_PROBE_ENABLED=1 ./bin/system-probe/system-probe --help
```

### System-Probe Help
```bash
$ DD_SYSTEM_PROBE_ENABLED=1 ./bin/system-probe/system-probe --help
The Datadog Agent System Probe runs as superuser in order to instrument
your machine at a deeper level. It is required for features such as Network Performance Monitoring,
Runtime Security Monitoring, Universal Service Monitoring, and others.

Usage:
  ./bin/system-probe/system-probe [command]

Available Commands:
  completion     Generate the autocompletion script for the specified shell
  config         Print the runtime configuration of a running agent
  debug          Print the runtime state of a running system-probe
  help           Help about any command
  module-restart Restart a given system-probe module
  run            Run the System Probe
  runtime        Runtime Security Agent (CWS) utility commands
  version        Print the version info
```

## Testing

Run the comprehensive test suite:
```bash
./test_dual_mode.sh
```

This tests:
- Default service-discovery mode
- System-probe mode with various environment variables
- Help system for both modes
- Service startup and shutdown
- Binary sizes and memory usage

## Error Handling

The wrapper gracefully handles various error conditions:

- **Missing shared libraries**: Clear error messages with library path
- **Symbol resolution failures**: Reports missing exported functions
- **Permission issues**: Service-discovery socket permissions
- **Library loading errors**: Detailed `dlopen`/`dlsym` error reporting

## Compatibility

- **Full backward compatibility**: All existing system-probe commands and arguments work unchanged
- **Environment variable compatibility**: All existing `DD_SYSTEM_PROBE_*` variables trigger system-probe mode
- **API compatibility**: Service-discovery provides compatible HTTP endpoints
- **Signal handling**: Both modes handle SIGINT/SIGTERM gracefully

## Implementation Details

### C Wrapper Logic
1. Parse command line for help flags
2. Check environment variables for `DD_SYSTEM_PROBE_*` pattern
3. Determine mode based on environment detection
4. Load appropriate shared library (`libsystemprobe.so` or `libservicediscovery.so`)
5. Call exported function (`RunSystemProbe` or `RunServiceDiscovery`)
6. Clean up and exit with proper return code

### Go Library Exports
Both Go libraries export a single function:
- **System-probe**: `RunSystemProbe() C.int` - wraps the original main logic
- **Service-discovery**: `RunServiceDiscovery() C.int` - wraps service-discovery main logic

### Build Integration
The build system (`tasks/system_probe.py`) includes new tasks:
- `build_shared_library` - Builds system-probe as shared library
- `build_service_discovery_library` - Builds service-discovery as shared library  
- `build_dual_wrapper` - Builds complete dual-mode system
- `build_dual_wrapper_binary` - Builds just the C wrapper

## Security Considerations

- **Dynamic loading security**: Libraries are loaded from the same directory as the binary
- **Environment variable validation**: Only non-empty values trigger mode changes
- **Signal handling**: Proper cleanup on shutdown signals
- **Socket permissions**: Service-discovery sets appropriate socket permissions (0770)

## Future Enhancements

- **Configuration-driven mode selection**: Allow mode selection via config file
- **Plugin architecture**: Support for additional lightweight modules
- **Runtime mode switching**: Hot-swap between modes without restart
- **Memory profiling**: Built-in memory usage monitoring and reporting

## Troubleshooting

### Common Issues

**"Failed to load system probe library"**
- Ensure `libsystemprobe.so` exists in the same directory as the binary
- Check file permissions on the shared library

**"Failed to load service discovery library"**  
- Ensure `libservicediscovery.so` exists in the same directory as the binary
- Verify the library was built successfully

**"Service discovery socket permission denied"**
- Default socket path requires elevated permissions
- Use `-socket /tmp/custom.sock` for testing
- Run as appropriate user for production socket paths

**System-probe not activating**
- Verify `DD_SYSTEM_PROBE_*` environment variable has a non-empty value
- Check environment variable is exported: `export DD_SYSTEM_PROBE_ENABLED=1`
- Use `env DD_SYSTEM_PROBE_ENABLED=1 ./system-probe version` for testing

### Debug Mode

Enable debug output by setting `DD_SYSTEM_PROBE_DEBUG=true` and observing the wrapper's environment detection messages.