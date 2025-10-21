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

Shows system information relevant to USM debugging.

**Usage:**
```bash
sudo ./system-probe usm sysinfo
sudo ./system-probe usm sysinfo --max-cmdline-length 100  # Extended command line display
sudo ./system-probe usm sysinfo --max-name-length 50      # Extended process name display
sudo ./system-probe usm sysinfo --max-cmdline-length 0 --max-name-length 0  # Unlimited
```

**Options:**
- `--max-cmdline-length` - Maximum command line length to display (default: 50, 0 for unlimited)
- `--max-name-length` - Maximum process name length to display (default: 25, 0 for unlimited)

**Output:**
- Kernel version
- OS type and architecture
- Hostname
- List of all running processes with PIDs, PPIDs, names, and command lines

**Output Example:**
```
=== USM System Information ===

Kernel Version: 5.15.0-73-generic
OS Type:        linux
Architecture:   amd64
Hostname:       agent-dev-ubuntu-22

Running Processes: 127

PID     | PPID    | Name                      | Command
--------|---------|---------------------------|--------------------------------------------------
1       | 0       | systemd                   | /sbin/init
156     | 1       | sshd                      | /usr/sbin/sshd -D
...
```

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
- Identify processes by PID for further investigation

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
- Processes are sorted by PID
- Output truncates long process names (default 25 chars) and command lines (default 50 chars) for readability
- Both truncation limits are configurable via flags
- Use `--max-cmdline-length 0` and `--max-name-length 0` for unlimited display