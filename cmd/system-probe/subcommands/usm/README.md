# USM Subcommands

This package provides debugging and diagnostic commands for Universal Service Monitoring (USM).

## Commands

### `usm config`

Shows the current USM configuration from the running system-probe instance.

**Usage:**
```bash
sudo ./system-probe usm config
```

**Output:**
- Displays all `service_monitoring_config` settings in YAML format
- Shows enabled protocols (HTTP, HTTP/2, Kafka, Postgres, Redis)
- Shows TLS monitoring settings (Native, Go, NodeJS, Istio)
- Queries the running system-probe via API to get actual runtime configuration

**Example:**
```
service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
  enable_http2_monitoring: true
  http:
    enabled: true
    max_stats_buffered: 100000
    max_tracked_connections: 1024
  ...
```

### `usm sysinfo`

Shows system information relevant to USM debugging, including detected services and programming languages.

**Usage:**
```bash
sudo ./system-probe usm sysinfo
sudo ./system-probe usm sysinfo --max-cmdline-length 100  # Extended command line display
sudo ./system-probe usm sysinfo --max-name-length 50      # Extended process name display
sudo ./system-probe usm sysinfo --max-service-length 30   # Extended service name display
sudo ./system-probe usm sysinfo --max-cmdline-length 0 --max-name-length 0 --max-service-length 0  # Unlimited
```

**Options:**
- `--max-cmdline-length` - Maximum command line length to display (default: 50, 0 for unlimited)
- `--max-name-length` - Maximum process name length to display (default: 25, 0 for unlimited)
- `--max-service-length` - Maximum service name length to display (default: 20, 0 for unlimited)

**Output:**
- Kernel version
- OS type and architecture
- Hostname
- List of all running processes with:
  - PIDs and PPIDs
  - Process names
  - Detected service names (using the same logic as the process-agent)
  - Detected programming language and version
  - Command lines

**Output Example:**
```
=== USM System Information ===

Kernel Version: 5.15.0-73-generic
OS Type:        linux
Architecture:   amd64
Hostname:       agent-dev-ubuntu-22

Running Processes: 127

PID     | PPID    | Name                      | Service              | Language     | Command
--------|---------|---------------------------|----------------------|--------------|--------------------------------------------------
1       | 0       | systemd                   | systemd              | -            | /sbin/init autoinstall
774     | 1       | containerd                | containerd           | go/go1.23.7  | /usr/bin/containerd
1046    | 1       | dockerd                   | dockerd              | go/go1.23.8  | /usr/bin/dockerd -H fd:// --containerd=/run/con...
...
```


### `usm netstat`

Shows network connections similar to `netstat -antpu`. Displays TCP and UDP connections with process information.

**Usage:**
```bash
sudo ./system-probe usm netstat                    # Show all TCP and UDP connections
sudo ./system-probe usm netstat --tcp=false        # Show only UDP connections
sudo ./system-probe usm netstat --udp=false        # Show only TCP connections
```

**Options:**
- `--tcp` / `-t` - Show TCP connections (default: true)
- `--udp` / `-u` - Show UDP connections (default: true)

**Output:**
- Protocol (tcp, tcp6, udp, udp6)
- Local address and port
- Remote address and port
- Connection state (ESTABLISHED, LISTEN, etc. for TCP)
- PID and process name

**Output Example:**
```
Proto | Local Address           | Foreign Address         | State       | PID/Program
------|-------------------------|-------------------------|-------------|------------------
tcp   | 0.0.0.0:22             | 0.0.0.0:0              | LISTEN      | 1234/sshd
tcp   | 127.0.0.1:8080         | 127.0.0.1:45678        | ESTABLISHED | 5678/python
tcp6  | :::80                  | :::0                   | LISTEN      | 9012/nginx
udp   | 0.0.0.0:53             | 0.0.0.0:0              |             | 3456/systemd-resolved
```

**Use Cases:**
- Debug USM connectivity issues
- Verify which processes are listening on ports
- Check established connections for monitored services
- Identify which processes own specific network connections
- Troubleshoot port conflicts

### `usm symbols ls`

Lists symbols from ELF binaries, similar to the Unix `nm` utility. Useful for analyzing symbol visibility, library versions, and linkage in monitored applications.

**Usage:**
```bash
sudo ./system-probe usm symbols ls <binary-file>          # List static symbols
sudo ./system-probe usm symbols ls --dynamic <binary-file> # List dynamic symbols
```

**Options:**
- `--dynamic` - Display dynamic symbols instead of static symbols

**Output:**
- Symbol address (16 hex digits or blank for undefined symbols)
- Symbol type (single character: T=text, D=data, B=BSS, U=undefined, w=weak, etc.)
- Symbol name with version information for dynamic symbols (e.g., `abort@GLIBC_2.17`)

**Example (dynamic symbols):**
```
                 U abort@GLIBC_2.17
                 U acos@GLIBC_2.17
                 w __cxa_finalize@GLIBC_2.17
0000000002fbe4d0 T lua_atpanic
0000000002fbe4e0 T lua_error
```

**Example (static symbols):**
```
0000000000000000 a $d.0
0000000000000008 d my_global_var
0000000000000020 D _ZN6myclass5valueE
0000000000000090 b thread_local_data
0000000002fbe4d0 T my_function
```

**Use Cases:**
- Check which GLIBC versions are required by a binary
- Verify symbol visibility (global vs local, weak vs strong)
- Debug symbol resolution issues in monitored applications
- Analyze library dependencies and symbol versions
- Identify exported/imported functions for troubleshooting

## Use Cases

### Debugging USM Configuration Issues

When USM is not working as expected, use `usm config` to verify:
- Is USM enabled? (`enabled: true`)
- Are the expected protocols enabled?
- Are TLS monitoring settings correct?

### Gathering System Information for Bug Reports

When reporting USM issues, include output from both commands:
```bash
sudo ./system-probe usm config > usm-config.yaml
sudo ./system-probe usm sysinfo > usm-sysinfo.txt
```

This provides complete context about the USM configuration and system environment.

### Checking Process Instrumentation

Use `usm sysinfo` to see what processes are running that USM might be monitoring, helping to:
- Verify target applications are running
- Check if applications are running with expected command line arguments
- Identify detected service names for processes (e.g., "nginx", "postgres", "node")
- See which programming languages are detected and their versions
- Identify processes by PID for further investigation

### Inspecting eBPF Maps

For eBPF map inspection and debugging, use the top-level `ebpf` commands:
```bash
sudo ./system-probe ebpf map list        # List all eBPF maps
sudo ./system-probe ebpf map dump name <map-name>  # Dump map contents
```

See the [eBPF subcommands README](../ebpf/README.md) for full documentation on eBPF inspection commands.

## Implementation Notes

### Configuration Command
- Queries the running system-probe instance via its API
- Uses the same configuration fetcher as `system-probe config`
- Extracts only the `service_monitoring_config` section
- Uses `yaml.v3` for parsing
- Outputs in YAML format

### Sysinfo Command
- Collects process information using `procutil.NewProcessProbe()` (same as process-agent)
- Uses `kernel.Release()` for kernel version detection
- Detects service names using `parser.NewServiceExtractor()` (same logic as process-agent service discovery)
- Processes are sorted by PID
- Output truncates long process names (default 25 chars), service names (default 20 chars), and command lines (default 50 chars) for readability
- All truncation limits are configurable via flags
- Use `--max-cmdline-length 0`, `--max-name-length 0`, and `--max-service-length 0` for unlimited display

### Netstat Command
- Uses `procnet.GetTCPConnections()` for robust TCP connection parsing with PID/FD mapping
- Reads UDP connections from `/proc/net/udp` and `/proc/net/udp6` (manual parsing)
- Maps socket inodes to processes by reading `/proc/*/fd/*` symlinks for UDP
- Parses hexadecimal IP addresses and ports to human-readable format
- Shows TCP connection states (ESTABLISHED, LISTEN, TIME_WAIT, etc.)
- Filters connections based on protocol flags (`--tcp`, `--udp`)
- Connections sorted by protocol and local port
- Use standard Unix tools like `grep` for additional filtering (e.g., `| grep LISTEN`)
- Linux with eBPF support only (requires `linux_bpf` build tag)

### Symbols Ls Command
- Parses ELF binaries using `pkg/util/safeelf` package for safe symbol table reading
- Supports both static symbols (`.symtab`) and dynamic symbols (`.dynsym`)
- Extracts symbol version information from `.gnu.version`, `.gnu.version_r`, and `.gnu.version_d` sections
- Handles off-by-one indexing between Go's `DynamicSymbols()` and `.gnu.version` array
- Filters out FILE-type symbols by default (matching standard `nm` behavior)
- Symbol types determined based on ELF section properties (SHF_EXECINSTR, SHF_ALLOC, SHF_WRITE, etc.)
- Version information shown with `@` prefix for requirements and `@@` for definitions
- Weak undefined symbols displayed as lowercase 'w' (not 'U')
- Symbols sorted by address for consistent output
- Linux-only implementation (returns nil on other platforms)
