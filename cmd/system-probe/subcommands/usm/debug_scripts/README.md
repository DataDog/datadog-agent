# USM Debug Scripts

This folder contains Python scripts for debugging USM (Universal Service Monitoring) in staging, production, and customer environments.

## Why Python?

We use Python scripts because:
- The system-probe container already includes Python
- Scripts can be quickly copied and executed without recompilation
- Much faster iteration compared to writing debug tools in Go, compiling, and deploying a new version of system-probe

## Available Scripts

### usm_leak_detector.py

Detects leaked entries in USM eBPF maps by comparing map contents against active TCP connections.

**Requirements:**
- Python 3.6+ (stdlib only, no pip dependencies)
- `bpftool` in PATH **OR** `system-probe` binary accessible
- Root privileges (for accessing eBPF maps and /proc)

**Usage:**
```bash
# Using bpftool (preferred if available)
sudo python3 usm_leak_detector.py

# Using system-probe as fallback (auto-detected)
sudo python3 usm_leak_detector.py

# Explicitly specify system-probe path
sudo python3 usm_leak_detector.py --system-probe /path/to/system-probe

# Verbose output
sudo python3 usm_leak_detector.py -v

# Check specific maps only
sudo python3 usm_leak_detector.py --maps pid_fd_by_tuple,http_in_flight
```

**Backend Selection:**

The script automatically selects the best available backend:
1. **bpftool** (preferred) - Outputs hex arrays, requires parsing
2. **system-probe** (fallback) - Uses `system-probe ebpf map` commands with BTF-formatted output

If neither is available, the script will exit with an error.

**What it does:**
1. Discovers all USM-related eBPF maps (connection tuple-keyed maps)
2. Parses ConnTuple structures from map entries
3. Validates entries against active TCP connections in /proc/net/tcp[6]
4. Reports any leaked entries that don't correspond to active connections
