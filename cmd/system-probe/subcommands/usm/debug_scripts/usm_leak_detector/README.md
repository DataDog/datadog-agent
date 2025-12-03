# USM Leak Detector

Detects leaked entries in USM eBPF maps by comparing map contents against system state.

## Requirements

- Python 3.6+ (stdlib only, no pip dependencies)
- `bpftool` in PATH **OR** `system-probe` binary accessible
- Root privileges (for accessing eBPF maps and /proc)

## Usage

```bash
# Run as a module (recommended) - checks both ConnTuple and PID-keyed maps
sudo python3 -m usm_leak_detector

# Verbose output
sudo python3 -m usm_leak_detector -v

# Check only ConnTuple-keyed maps (connection-based validation)
sudo python3 -m usm_leak_detector --conn-tuple-only

# Check only PID-keyed maps (process existence validation)
sudo python3 -m usm_leak_detector --pid-only

# Explicitly specify system-probe path
sudo python3 -m usm_leak_detector --system-probe /path/to/system-probe

# Check specific maps only
sudo python3 -m usm_leak_detector --maps pid_fd_by_tuple,http_in_flight

# Disable race condition re-check (faster, but may have false positives)
sudo python3 -m usm_leak_detector --recheck-delay 0

# Custom /proc path (useful in containers)
sudo python3 -m usm_leak_detector --proc-root /host/proc
```

## Deployment to Customer Environments

The `usm_leak_detector` directory can be zipped and transferred to any environment:

```bash
# Create a zip for deployment
cd debug_scripts
zip -r usm_leak_detector.zip usm_leak_detector/

# On the target machine, extract and run
unzip usm_leak_detector.zip
sudo python3 -m usm_leak_detector -v
```

## Backend Selection

The script automatically selects the best available backend:
1. **bpftool** (preferred) - Outputs hex arrays, requires parsing
2. **system-probe** (fallback) - Uses `system-probe ebpf map` commands with BTF-formatted output

If neither is available, the script will exit with an error.

## What It Does

### ConnTuple-Keyed Maps
For maps keyed by connection tuples (48-byte ConnTuple structures):
1. Parses ConnTuple structures from map entries
2. Validates entries against active TCP connections in /proc/net/tcp[6]
3. Reports entries that don't correspond to active connections

Maps checked: `connection_states`, `pid_fd_by_tuple`, `ssl_ctx_by_tuple`, `http_in_flight`,
`redis_in_flight`, `postgres_in_flight`, `http2_in_flight`, `connection_protocol_by_tuple`,
`tls_enhanced_tags`

### PID-Keyed Maps
For maps keyed by pid_tgid (8-byte uint64):
1. Extracts PID from the upper 32 bits of each key
2. Checks if the process still exists in /proc
3. Reports entries where the process no longer exists

Maps checked: `ssl_read_args`, `ssl_read_ex_args`, `ssl_write_args`, `ssl_write_ex_args`,
`bio_new_socket_args`, `ssl_ctx_by_pid_tgid`

## Race Condition Handling

The detector has a TOCTOU (time-of-check to time-of-use) race condition: connections may be
established between reading /proc/net/tcp and reading the eBPF maps. To filter these false
positives, the detector re-checks leaked entries after a delay (default: 2 seconds).

- `--recheck-delay 2.0` (default): Wait 2 seconds, rebuild connection index, re-validate
- `--recheck-delay 0`: Disable re-check for faster runs (may report false positives)

The report shows how many false positives were filtered by this mechanism.

## Package Structure

```
usm_leak_detector/
├── __init__.py           # Package metadata
├── __main__.py           # Entry point for python -m
├── cli.py                # Command-line interface
├── constants.py          # Target maps and constants
├── models.py             # Data classes (ConnTuple, MapLeakInfo, PIDLeakInfo, etc.)
├── map_discovery.py      # ConnTuple map discovery logic
├── network.py            # Network namespace and TCP parsing
├── parser.py             # ConnTuple byte parsing
├── validator.py          # Tuple validation against active connections
├── pid_validator.py      # PID-based leak detection
├── analyzer.py           # Map analysis logic
├── report.py             # Report generation
└── backends/
    ├── __init__.py       # Backend exports
    ├── ebpf_backend.py   # Abstract backend interface
    ├── bpftool.py        # bpftool backend implementation
    ├── system_probe.py   # system-probe backend implementation
    └── backend_selector.py  # Backend selection logic
```