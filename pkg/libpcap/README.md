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
| `grammar/` | Hand-written lexer + goyacc-generated parser | `scanner.l`, `grammar.y.in` |
| `codegen/` | BPF code generator (expression → block CFG → instructions) | `gencode.c` |
| `optimizer/` | BPF optimizer (constant folding, dead code elimination, jump threading, predicate assertion) | `optimize.c` |
| `nameresolver/` | Host, port, and protocol name resolution | `nametoaddr.c` |

## API

The top-level package exposes a drop-in replacement for `gopacket/pcap`:

```go
import "github.com/DataDog/datadog-agent/pkg/libpcap"

// Compile a filter to BPF instructions
insns, err := libpcap.CompileBPFFilter(libpcap.LinkTypeEthernet, 256, "tcp dst port 80")

// Compile and get a human-readable dump
dump, err := libpcap.DumpFilter(libpcap.LinkTypeEthernet, 256, "tcp dst port 80")
```

## Supported features

### Filter expressions — full support

The following filter constructs produce **instruction-for-instruction identical**
BPF output compared to C libpcap 1.10.6, both optimized (`filtertest` default)
and unoptimized (`filtertest -O`):

| Category | Examples | Status |
|----------|---------|--------|
| Protocol keywords | `ip`, `ip6`, `arp`, `rarp`, `tcp`, `udp`, `sctp`, `icmp`, `icmp6`, `igmp`, `igrp`, `pim`, `vrrp`, `carp`, `ah`, `esp` | Exact match |
| ISO/IS-IS protocols | `iso`, `esis`, `isis`, `clnp`, IS-IS levels/PDU types (`l1`, `l2`, `iih`, `lsp`, `snp`, `csnp`, `psnp`) | Exact match |
| LLC protocols | `stp`, `ipx`, `netbeui` | Exact match |
| Legacy protocols | `atalk`, `aarp`, `decnet`, `lat`, `sca`, `moprc`, `mopdl` | Exact match |
| Host matching | `host 192.168.1.1`, `src host 10.0.0.1`, `dst host`, `host` with name resolution | Exact match |
| IPv6 hosts | `ip6 src host ::1`, `ip6 dst host fe80::1`, `host` with IPv6 | Exact match |
| Ethernet hosts | `ether src 00:11:22:33:44:55`, `ether dst ff:ff:ff:ff:ff:ff`, `ether host` | Exact match |
| Network matching | `net 192.168.0.0/16`, `src net 10.0.0.0/8`, `net` with mask notation | Exact match |
| Port matching | `port 80`, `src port 443`, `dst port 53`, `tcp port 80`, `udp port 53` | Exact match |
| Port ranges | `portrange 8000-9000` | Exact match |
| Direction qualifiers | `src`, `dst`, `src or dst`, `src and dst` | Exact match |
| Boolean operators | `and` / `&&`, `or` / `||`, `not` / `!`, parentheses | Exact match |
| Broadcast/multicast | `ether broadcast`, `ip multicast`, `ip6 multicast`, `ether multicast` | Exact match |
| Packet length | `less 100`, `greater 1000`, `len >= 64`, `len <= 1500` | Exact match |
| Protocol numbers | `ip proto 6`, `ip proto 17` | Exact match |
| Named ports/protos | `port http`, `proto tcp` (via name resolver) | Exact match |
| Combined filters | `tcp port 80 and host 192.168.1.1`, `tcp or udp`, `not tcp`, `(tcp or udp) and port 53`, `src net 192.168.0.0/24 and dst port 80` | Exact match |

### Filter expressions — supported with unoptimized output differences

The following constructs compile correctly and produce **instruction-identical
optimized output** compared to C libpcap. However, their *unoptimized* instruction
sequences differ because the Go codegen uses the general-purpose register-based
form for byte-access and arithmetic expressions, while C libpcap has specialized
shortcuts. The optimizer eliminates these differences.

| Category | Examples | Unoptimized difference | Optimized |
|----------|---------|----------------------|-----------|
| Byte access | `tcp[13] & 0x02 != 0`, `tcp[tcpflags] & tcp-syn != 0`, `ip[8] < 64` | More register load/store instructions | Exact match |
| Bitwise in comparisons | `ether[0] & 1 != 0`, `ip[0] & 0xf != 5` | General-purpose arth path vs shortcut | Exact match |
| TCP flags | `tcp[tcpflags] == tcp-syn` | Same as above | Exact match |
| Arithmetic expressions | `tcp[tcpflags] & (tcp-syn\|tcp-ack) != 0` | Uses register ALU ops | Exact match |

### Not yet implemented

The following features return a `"not yet implemented"` error at compile time.
They are planned for future implementation.

| Feature | Filter examples | libpcap C source |
|---------|----------------|-----------------|
| VLAN | `vlan`, `vlan 100`, `vlan and tcp port 80` | `gen_vlan()` |
| MPLS | `mpls`, `mpls 100` | `gen_mpls()` |
| PPPoE | `pppoed`, `pppoes` | `gen_pppoed()`, `gen_pppoes()` |
| Geneve | `geneve`, `geneve 100` | `gen_geneve()` |
| LLC frames | `llc`, `llc i`, `llc s`, `llc u` | `gen_llc()` |
| ATM | `lane`, `metac`, `bcc`, `oam`, `vpi`, `vci` | `gen_atmtype_abbrev()` |
| MTP2/MTP3 (SS7) | `fisu`, `lssu`, `msu`, `sio`, `opc`, `dpc` | `gen_mtp2type_abbrev()` |
| PF (packet filter) | `on ifname`, `rset ruleset`, `reason`, `action` | `gen_pf_*()` |
| IEEE 802.11 | `type`, `subtype`, `dir`, `addr1`-`addr4`, `ra`, `ta` | `gen_p80211_type()` |
| Inbound/outbound | `inbound`, `outbound` | `gen_inbound()` |
| Interface index | `ifindex N` | `gen_ifindex()` |
| Gateway | `gateway hostname` | `gen_gateway()` |
| Protochain | `protochain N` | `gen_protochain()` |
| ARCnet addresses | `$XX` AID tokens | `gen_acode()` |

### Differences from C libpcap

| Aspect | C libpcap | Go implementation |
|--------|-----------|-------------------|
| **Build requirements** | C compiler, libpcap headers, `flex`, `bison` | Pure Go, no C toolchain |
| **Parser** | Flex-generated lexer + Bison-generated parser | Hand-written lexer + goyacc parser |
| **Inherited attributes** | Bison `$<type>0` stack access | Explicit qualifier stack on `CompilerState` |
| **Error handling** | `setjmp`/`longjmp` | Go error returns (`cs.Err`) |
| **Memory management** | Custom chunked allocator (`newchunk`) | Go garbage collector |
| **Name resolution** | `gethostbyname`, `getaddrinfo`, `/etc/protocols`, `/etc/ethers` | Go `net.LookupIP`, `net.LookupPort`, built-in protocol table |
| **Optimizer** | Full optimizer (dead code elimination, constant folding, jump optimization) | Full optimizer — 49/52 corpus filters produce instruction-identical optimized output |
| **Matching API** | `pcap_offline_filter()`, `BPF.Matches()` | Not yet implemented (Phase 4) — `bpf.Filter()` interpreter exists |
| **Link types** | ~60 DLT types | Ethernet (`DLT_EN10MB`), loopback, Linux cooked, raw IP |
| **Unoptimized output** | Compact shortcuts for common patterns | General-purpose register-based form for byte access/arithmetic (optimizer eliminates the differences) |
| **Thread safety** | Reentrant parser via Flex/Bison options | Naturally safe (no global state) |

### Grammar porting

The goyacc grammar was mechanically ported from `grammar.y.in`. See
[`grammar/PORTING.md`](grammar/PORTING.md) for the full list of modifications
and [`grammar/convert_grammar.sh`](grammar/convert_grammar.sh) for the
automated conversion script.

## Development

### Prerequisites

For running golden tests (comparison against C libpcap), you need the C
`filtertest` binary built from libpcap 1.10.6 sources:

```bash
./pkg/libpcap/testdata/build_filtertest.sh /path/to/libpcap-1.10.6 ./pkg/libpcap/testdata/filtertest
```

### Running tests

```bash
# Unit tests (no C toolchain needed)
go test ./pkg/libpcap/...

# Golden tests comparing Go vs C filtertest (requires filtertest binary)
go test -tags libpcap_test ./pkg/libpcap/ -run TestGoldenSimpleFilters
go test -tags libpcap_test ./pkg/libpcap/ -run TestGoldenFiltertestUnoptimized

# Verbose output showing BPF instruction dumps
go test -tags libpcap_test ./pkg/libpcap/ -v -run TestGoldenSimpleFilters
```

### Parser generation

The grammar is defined in `grammar/grammar.y` and processed by `goyacc`:

```bash
go generate ./pkg/libpcap/grammar/
```

## Migration status

| Phase | Status |
|-------|--------|
| 1. Foundation (bpf/, test harness) | Complete |
| 2. Compiler pipeline (scanner, grammar, codegen, linearizer) | Complete |
| 3. Optimizer | Complete |
| 4. Matching API (`NewBPF`, `Matches`) | Not started |
| 5. Integration (switch agent consumers) | Not started |
| 6. Cleanup (remove gopacket/pcap dep) | Not started |

## Reference

- [libpcap 1.10.6](https://www.tcpdump.org/) — the C library being reimplemented
- [Grammar porting guide](grammar/PORTING.md) — how `grammar.y.in` was converted to goyacc
- `pkg/security/ebpf/probes/rawpacket/pcap.go` — agent consumer (BPF compilation)
- `pkg/security/secl/model/oo_packet_filter_unix.go` — agent consumer (packet matching)
