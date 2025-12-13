# USM Leak Detector

Detects leaked entries in USM eBPF maps by comparing map contents against system state.

## Quick Start

```bash
sudo python3 -m usm_leak_detector
```

## What It Does

### ConnTuple-Keyed Maps
For maps keyed by connection tuples (48-byte ConnTuple structures):
1. Parses ConnTuple structures from map entries
2. Validates entries against active TCP connections in /proc/net/tcp[6]
3. Reports entries that don't correspond to active connections

Maps checked:
- `connection_states`, `pid_fd_by_tuple`, `ssl_ctx_by_tuple`
- `http_in_flight`, `http2_in_flight`
- `redis_in_flight`, `redis_key_in_flight`
- `postgres_in_flight`
- `http2_dynamic_table`, `http2_dynamic_counter_table`, `http2_incomplete_frames`
- `kafka_response`, `kafka_in_flight`
- `go_tls_conn_by_tuple`, `connection_protocol`, `tls_enhanced_tags`

### PID-Keyed Maps
For maps keyed by pid_tgid (8-byte uint64)
:
1. Extracts PID from the upper 32 bits of each key
2. Checks if the process still exists in /proc
3. Reports entries where the process no longer exists

Maps checked:
- `ssl_read_args`, `ssl_read_ex_args`, `ssl_write_args`, `ssl_write_ex_args`
- `bio_new_socket_args`, `ssl_ctx_by_pid_tgid`

## Requirements

- Python 3.6+ (stdlib only, no pip dependencies)
- Root privileges (for accessing eBPF maps and /proc)
- Internet access (only if bpftool not installed — used for auto-download)

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

### Minimal Container Environments (no unzip)

In minimal containers (e.g., Datadog Agent system-probe), `unzip` may not be available.
Use Python's built-in zipfile module instead:

```bash
# Copy zip to container
kubectl cp usm_leak_detector.zip <namespace>/<pod>:/tmp/ -c system-probe

# Extract using Python and run
kubectl exec -n <namespace> <pod> -c system-probe -- /bin/bash -c \
  "cd /tmp && rm -rf usm_leak_detector && python3 -m zipfile -e usm_leak_detector.zip . && python3 -m usm_leak_detector -v"
```

## Backend Selection

The script automatically selects the best available backend:
1. **bpftool** (preferred) - Outputs hex arrays, requires parsing
2. **system-probe** (fallback) - Uses `system-probe ebpf map` commands with BTF-formatted output

### Automatic bpftool Download

If `bpftool` is not installed, the script automatically downloads a static binary from
[libbpf/bpftool releases](https://github.com/libbpf/bpftool/releases) to `/tmp/bpftool`.

- Supports **amd64** and **arm64** architectures
- Works on Ubuntu, Debian, RHEL, Amazon Linux, and other common cloud images
- Cached in `/tmp/bpftool` - reused on subsequent runs until reboot
- Falls back to `system-probe` if download fails (e.g., no internet access)

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
├── logging_config.py     # Logging configuration
├── models.py             # Data classes (ConnTuple, MapLeakInfo, PIDLeakInfo, etc.)
├── map_discovery.py      # ConnTuple map discovery logic
├── network.py            # Network namespace and TCP parsing
├── parser.py             # ConnTuple byte parsing
├── validator.py          # Tuple validation against active connections
├── pid_validator.py      # PID-based leak detection
├── analyzer.py           # Map analysis logic
├── report.py             # Report generation
├── subprocess_utils.py   # Seccomp-safe subprocess timeout handling
└── backends/
    ├── __init__.py       # Backend exports
    ├── ebpf_backend.py   # Abstract backend interface
    ├── bpftool.py        # bpftool backend implementation
    ├── bpftool_downloader.py  # Auto-download static bpftool binary
    ├── system_probe.py   # system-probe backend implementation
    ├── backend_selector.py  # Backend selection logic
    └── streaming.py      # Streaming JSON parser for bpftool output
```
