# pkg/libpcap — Pure Go BPF Packet Filter Compiler

A pure Go reimplementation of libpcap's BPF filter compiler, targeting full
feature parity with libpcap 1.10.6 for `LinkTypeEthernet`.

## Why

The Datadog Agent uses libpcap to compile BPF packet filter expressions (e.g.,
`"tcp dst port 80"`) into classic BPF instructions. The current implementation
depends on `gopacket/pcap`, which wraps the C libpcap library via cgo. This
introduces a C toolchain requirement into what is otherwise a pure Go build,
complicates cross-compilation, and requires special `pcap` + `cgo` build tags
with unsupported-platform stubs.

This package replaces the cgo dependency with a native Go implementation.

## Packages

| Package | Description | Ports |
|---------|-------------|-------|
| `bpf/` | BPF instruction types, interpreter, validator, formatter | `pcap/bpf.h`, `bpf_filter.c`, `bpf_image.c`, `bpf_dump.c` |
| `scanner/` | Hand-written lexer for filter expressions | `scanner.l` |
| `grammar/` | goyacc-generated parser | `grammar.y.in` |
| `codegen/` | BPF code generator (expression → block CFG → instructions) | `gencode.c` |
| `optimizer/` | BPF optimizer (dead code elimination, constant folding, etc.) | `optimize.c` |
| `nameresolver/` | Host, port, and protocol name resolution | `nametoaddr.c` |

## API

The top-level package exposes a drop-in replacement for `gopacket/pcap`:

```go
import "github.com/DataDog/datadog-agent/pkg/libpcap"

// Compile a filter to BPF instructions
insns, err := libpcap.CompileBPFFilter(layers.LinkTypeEthernet, 256, "tcp dst port 80")

// Compile and match packets
filter, err := libpcap.NewBPF(layers.LinkTypeEthernet, 256, "tcp dst port 80")
ok := filter.Matches(captureInfo, packetData)
```

## Development

### Prerequisites

For running golden tests (comparison against C libpcap), you need the C
`filtertest` binary built from libpcap 1.10.6 sources:

```bash
cd /path/to/libpcap-1.10.6
./configure
make testprogs
# produces testprogs/filtertest
```

### Running tests

```bash
# Unit tests (no C toolchain needed)
go test ./pkg/libpcap/...

# Golden tests (requires filtertest binary)
go test -tags libpcap_test ./pkg/libpcap/...

# Fuzz testing
go test -fuzz=FuzzCompile ./pkg/libpcap/
```

### Parser generation

The grammar is defined in `grammar/grammar.y` and processed by `goyacc`:

```bash
go generate ./pkg/libpcap/grammar/
```

## Migration status

This package is being developed incrementally. See the
for the full migration plan.

| Phase | Status |
|-------|--------|
| 1. Foundation (bpf/, test harness) | Not started |
| 2. Compiler pipeline (scanner, grammar, codegen) | Not started |
| 3. Optimizer | Not started |
| 4. Matching API | Not started |
| 5. Integration (switch agent consumers) | Not started |
| 6. Cleanup (remove gopacket/pcap dep) | Not started |

## Reference

- [libpcap 1.10.6](https://www.tcpdump.org/) — the C library being reimplemented
- `pkg/security/ebpf/probes/rawpacket/pcap.go` — agent consumer (BPF compilation)
- `pkg/security/secl/model/oo_packet_filter_unix.go` — agent consumer (packet matching)
