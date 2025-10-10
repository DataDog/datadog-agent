# USM Subcommands

This package provides debugging and diagnostic commands for Universal Service Monitoring (USM).

## Commands

### `usm config`

Shows the current USM configuration from the running system-probe instance.

**Usage:**
```bash
sudo ./system-probe usm config
sudo ./system-probe usm config --json  # JSON output
```

**Output:**
- Displays all `service_monitoring_config` settings
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
sudo ./system-probe usm sysinfo --json  # JSON output
```

**Output:**
- Kernel version
- OS type and architecture
- Hostname
- List of all running user-space processes with PIDs and command lines

**Example:**
```
=== USM System Information ===

Kernel Version: 5.15.0-73-generic
OS Type:        linux
Architecture:   amd64
Hostname:       agent-dev-ubuntu-22

Running Processes: 127

PID     | Name                           | Command Line
--------|--------------------------------|--------------------------------------------------
1       | systemd                        | /sbin/init
156     | sshd                           | /usr/sbin/sshd -D
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
sudo ./system-probe usm config --json > usm-config.json
sudo ./system-probe usm sysinfo --json > usm-sysinfo.json
```

This provides complete context about the USM configuration and system environment.

### Checking Process Instrumentation

Use `usm sysinfo` to see what processes are running that USM might be monitoring, helping to:
- Verify target applications are running
- Check if applications are running with expected command line arguments
- Identify processes by PID for further investigation

## Implementation Notes

- Both commands query the running system-probe instance via its API
- `usm config` uses the same configuration fetcher as the `system-probe config` command
- `usm sysinfo` filters out kernel threads (processes without command lines) to focus on user-space applications
- Both commands support `--json` flag for programmatic access