# pkg/network

Before testing a change here or claiming it works, read `pkg/ebpf/AGENTS.md` (testing system-probe and
eBPF changes).

- **Classification tests can pass on the wrong connection.** `waitForConnectionsWithProtocol` matches
  any connection between the given addresses, so cases that share a server port can pass on an earlier
  case's connection. Give each case its own port, and show a new test fails against a deliberately
  broken implementation before trusting it.

## Performance claims

The general rules are in `pkg/ebpf/AGENTS.md`. For network probes, also:

- **Normalize by work done.** On a saturated node, lower overhead shows up as more throughput, not less
  CPU; check that the achieved load matches between variants.
- **Loopback and cross-container traffic are valid modes.** Measure them as their own scenario, not as
  a stand-in for cross-host traffic.

A fuller guide to load-testing CNM probes (probe layers, the traffic that drives each one, and the
surrounding kernel's concurrency) is future work.
