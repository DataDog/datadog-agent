# macOS connection monitoring

The macOS connection monitor has an opt-in composite backend. NStat is the
authoritative, event-driven source for connection lifecycle, tuple, process
identity, and byte counters. Libpcap may enrich those connections with packet
evidence, and bounded libproc scans may resolve identities that NStat did not
provide. Neither sidecar creates a connection or replaces authoritative NStat
data.

The existing eBPF-less packet-owned tracer remains the default.

## Configuration

Set `network_config.darwin_connection_tracer_backend` in
`system-probe.yaml`:

- `ebpfless`: existing backend and default.
- `nstat`: NStat plus optional libproc reconciliation, without packet
  enrichment.
- `nstat-pcap`: complete NStat, libpcap, and libproc composite.
- `auto`: complete composite with startup and runtime fallback to `ebpfless`.

The `nstat`, `nstat-pcap`, and `auto` modes all fall back if the private NStat
control or revision-9 ABI is unavailable. A fatal runtime error closes the
current NStat generation and all sidecars before the eBPF-less generation
starts.

Independent kill switches:

```yaml
network_config:
  darwin_connection_tracer_backend: nstat-pcap
  darwin_connection_tracer_packet_enabled: true
  darwin_connection_tracer_libproc_enabled: true
```

The packet snap length, BPF ring size, reconciliation interval, process limit,
descriptor limit, and observation limit are bounded by the Agent even if an
invalid value is configured.

## Health and troubleshooting

The system-probe network tracer stats report:

- `active_backend`: `nstat`, `ebpfless`, or `unavailable`.
- `nstat_abi_revision`: currently `9`.
- `source_healthy` and `runtime_fallback`.
- `packet_enrichment` and `libproc_reconciler`: `healthy`, `disabled`, or
  `unavailable`.
- `last_error`: a single-line, bounded diagnostic.

If the active backend is `ebpfless` while NStat was requested, inspect
`last_error` and system-probe logs. An ABI error means the running macOS kernel
does not match the revision-9 decoder. Packet or libproc degradation does not
stop NStat collection; use the independent switches to isolate either sidecar.

The packet source intentionally excludes `lo0`, `bridge*`, `vlan*`, and
`vmenet*`. Connections on those interfaces remain visible through NStat but do
not receive packet-only retransmission, reset/refusal, or L7/TLS enrichment.
`utun*` is captured when BPF access is available. Physical interface names such
as `en*` are captured.

## Qualification

Run the live kernel-control suite on every candidate host:

```shell
RUN_NSTAT_FUNCTIONAL_TEST=1 \
  dda inv test \
  --targets=./pkg/network/filter,./pkg/network/tracer/connection,./pkg/network/tracer/connection/nstat,./pkg/network/tracer \
  --extra-args='-run (NStat|ControlFunctionalRevision9|DarwinCompositeQualification|DarwinPacketSourceInterfaceMatrix) -count=1' \
  --timeout=120
```

The core suite covers revision-9 parsing from the live control,
separate-process TCP and UDP over IPv4 and IPv6, exact payload counters,
one-way UDP while open, one-way UDP immediate close, graceful close,
abortive-close lifecycle, short-lived flows, and exactly-once removal. The
optional targets add packet-derived refusal evidence and the cardinality gate.
IPv6 is skipped only when the host has no IPv6 loopback.

Optional environmental gates:

- Set `NSTAT_REFUSAL_TARGET` to a closed TCP port reached through an interface
  captured by libpcap.
- Set `NSTAT_LOAD_CONNECTIONS` to the sustained cardinality target. Override
  the default 128 MiB heap-growth gate with `NSTAT_MAX_HEAP_DELTA_MB`.

### Cardinality gate

Ensure the shell's open-file limit exceeds twice the requested connection
count plus headroom. Start with 1,000 connections:

```shell
ulimit -n 4096
RUN_NSTAT_FUNCTIONAL_TEST=1 \
  NSTAT_LOAD_CONNECTIONS=1000 \
  dda inv test \
  --targets=./pkg/network/tracer/connection \
  --extra-args='-run ^TestNStatQualificationSustainedCardinality$ -count=1 -v' \
  --timeout=120
```

The gate requires every connection to become active, heap growth to stay below
128 MiB, and every connection to close. Increase
`NSTAT_LOAD_CONNECTIONS` toward the intended rollout load and record creation
time, close latency, and heap delta from the `connections=` entry in
`test_output.json`. Set
`NSTAT_MAX_HEAP_DELTA_MB` only when validating an explicitly approved
alternative memory budget.

For packaged validation, build and install the system-probe from the candidate
package, select each backend in turn, and query `/connections` from both the
core Agent and process-agent clients. Verify the same PID, tuple, byte
counters, failures, protocol stack, and TLS tags reach fakeintake. Then force
an NStat control failure and confirm:

1. every open NStat connection is emitted closed exactly once;
2. sidecars stop before `active_backend` changes to `ebpfless`;
3. no tuple is emitted concurrently by both generations;
4. restart and package upgrade preserve the configured opt-in mode; and
5. an injected ABI mismatch reports revision 9 and falls back; and
6. forced source-reference, tuple, and PID reuse creates a new identity and
   never attributes the new owner to the previous flow.

Record CPU, resident memory, packet capture drops, NStat parser/kernel errors,
close latency, and unresolved PID rate during the load gate. Compare idle,
steady-state, burst, and 24-hour sustained runs against `ebpfless`.

### Platform and interface matrix

Qualification is required on Apple Silicon and Intel for every supported macOS
version. Exercise:

- physical Ethernet and Wi-Fi (`en*`);
- loopback (`lo0`, NStat-only by design);
- VPN (`utun*`);
- bridge/VM (`bridge*` and `vmenet*`, NStat-only by design); and
- VLAN (`vlan*`, NStat-only by design).

Do not enable the backend by default until every supported platform row passes,
the packaged path reaches fakeintake, and no performance gate regresses.

## Windows semantic comparison

Externally visible fields use the existing cross-platform connection payload:

- tuple, PID, lifecycle, and cumulative payload bytes: **exact** when NStat
  supplies them;
- direction and client/server orientation: **equivalent**, normalized to the
  existing Agent tuple model;
- retransmissions and reset/refusal evidence: **conditional** on packet
  enrichment for the interface;
- RTT and RTT variance: **approximate**, because kernel sampling differs;
- HTTP, HTTP/2, TLS protocol stack, and TLS tags: **equivalent when observed**
  within bounded packet-prefix reassembly;
- container and network-namespace identity: **unavailable** on macOS; and
- interface index: **unavailable in the serialized connection payload**.

Any matrix result that differs from these classifications blocks default
enablement and must be documented before rollout.

## Rollout

1. Keep `ebpfless` as the default.
2. Enable `nstat` for internal canaries to validate authoritative lifecycle and
   PID accuracy.
3. Enable `nstat-pcap` for canaries after packet-drop and resource gates pass.
4. Expand by macOS version and architecture while monitoring fallback,
   unresolved identity, parser errors, capture drops, CPU, and memory.
5. Stop expansion on sustained fallback, duplicate lifecycle events, ABI
   mismatch, or a breached performance threshold.
