# pkg/network

Before testing a change here or claiming it works, read `pkg/ebpf/AGENTS.md` (testing system-probe and
eBPF changes).

- **Classification tests can pass on the wrong connection.** `waitForConnectionsWithProtocol` matches
  any connection between the given addresses, so cases that share a server port can pass on an earlier
  case's connection. Give each case its own port, and show a new test fails against a deliberately
  broken implementation before trusting it.
