# eBPF Subcommands

This package provides debugging and diagnostic commands for inspecting eBPF objects system-wide.

## Commands

### `ebpf map list`

Lists all eBPF maps loaded on the system.

**Usage:**
```bash
sudo ./system-probe ebpf map list
sudo ./system-probe ebpf map list | grep http  # Filter for specific maps
```

**Output:**
- Lists all eBPF maps system-wide (not limited to system-probe maps)
- Shows map ID, type, name, flags, key size, value size, and max entries
- Output format matches bpftool

**Example:**
```
28677: Hash  name helper_err_tele  flags 0x0
    key 8B  value 3072B  max_entries 63
28678: Hash  name tcp_sendmsg_arg  flags 0x0
    key 8B  value 8B  max_entries 1024
28682: Hash  name conn_stats  flags 0x0
    key 48B  value 56B  max_entries 65536
```

### `ebpf map dump`

Dumps the contents of a specific eBPF map.

**Usage:**
```bash
sudo ./system-probe ebpf map dump id <map-id>       # Dump by map ID
sudo ./system-probe ebpf map dump name <map-name>   # Dump by map name
sudo ./system-probe ebpf map dump name conn_stats | jq  # Use with jq
sudo ./system-probe ebpf map dump --pretty name conn_stats  # Pretty-print output
```

**Flags:**
- `--pretty` - Pretty-print JSON output with proper indentation (default: false)

**Output:**
- JSON format with compact byte arrays (default) or pretty-printed with `--pretty`
- Each entry shows key and value as arrays of hex-encoded bytes
- Default output format matches bpftool for backward compatibility
- Compatible with jq and other JSON tools
- Use `--pretty` for human-readable output without requiring jq

**Example (default compact format):**
```json
[{
	"key": ["0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x00","0xac","0x12","0x00","0x02"],
	"value": ["0x38","0x04","0x00","0x00","0x00","0x00","0x00","0x00","0x88","0x02","0x00","0x00"]
},{
	"key": ["0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x7f","0x00","0x00","0x01"],
	"value": ["0xa6","0x7a","0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x00","0x00"]
}]
```

**Example (with `--pretty` flag):**
```json
[
  {
    "key": [
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0xac",
      "0x12",
      "0x00",
      "0x02"
    ],
    "value": [
      "0x38",
      "0x04",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x00",
      "0x88",
      "0x02",
      "0x00",
      "0x00"
    ]
  }
]
```

## Use Cases

### Inspecting eBPF Maps

Use `ebpf map list` and `ebpf map dump` to debug eBPF map contents:
- Verify maps are loaded correctly
- Check map sizes and configurations
- Inspect map contents for debugging
- Extract data for offline analysis with jq

**Example workflow:**
```bash
# List all maps and find the one you need
sudo ./system-probe ebpf map list | grep conn

# Dump the map contents
sudo ./system-probe ebpf map dump name conn_stats > conn_stats.json

# Analyze with jq
jq 'length' conn_stats.json  # Count entries
jq '.[0]' conn_stats.json    # See first entry
```

### Debugging Multiple eBPF Modules

These commands work with all eBPF maps from any module:
- USM (Universal Service Monitoring)
- Network Tracer
- OOM Kill Probe
- TCP Queue Tracer
- Security Monitoring
- Any other eBPF-based system-probe module

### Cross-Module Analysis

Compare maps across different modules:
```bash
# Find all connection tracking maps
sudo ./system-probe ebpf map list | grep -E 'conn|tcp'

# Compare USM and network tracer state
sudo ./system-probe ebpf map dump name usm_conn_stats > usm.json
sudo ./system-probe ebpf map dump name network_conn_stats > network.json
diff <(jq -S . usm.json) <(jq -S . network.json)
```

## Implementation Notes

### eBPF Commands
- Direct eBPF map inspection using cilium/ebpf library
- No dependencies on running system-probe instance
- Uses kernel APIs (`MapGetNextID`, `NewMapFromID`) to enumerate all maps system-wide
- Generic map iteration via `Map.Iterate()` - no custom per-map handlers needed
- Output format matches bpftool for compatibility
- List command outputs text, dump command outputs JSON
- Works with all eBPF map types (Hash, Array, PerCPU, etc.)

### Platform Support
- Linux only (build tag: `linux_bpf`)
- Requires kernel support for BPF syscalls
- No special permissions beyond CAP_BPF or CAP_SYS_ADMIN