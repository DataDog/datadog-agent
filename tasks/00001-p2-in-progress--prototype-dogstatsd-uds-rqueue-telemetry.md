# Prototype DogStatsD Unix-socket receive-queue telemetry

## Goal

Determine whether Linux `sock_diag` provides a useful, low-overhead signal that the DogStatsD Unix datagram socket is backing up, and build a narrowly scoped Agent prototype if it does.

The motivating Confluence note proposes `UDIAG_SHOW_RQLEN`. Kernel-source review establishes two important corrections/constraints:

- The receive-side field is `udiag_rqueue`; `udiag_wqueue` is write-memory allocation.
- For a non-listening `SOCK_DGRAM` Unix socket, `udiag_rqueue` is `unix_inq_len()`, which returns only the byte length of the **next datagram**, not total queued bytes or datagram count. Thus nonzero means at least one datagram is pending, but its magnitude is not queue depth.
- `UDIAG_SHOW_MEMINFO` additionally exposes `SK_MEMINFO_RMEM_ALLOC`, `SK_MEMINFO_RCVBUF`, and cumulative `SK_MEMINFO_DROPS`. These may provide the more actionable saturation signal.

## Prototype plan

1. **Validate semantics in Linux via Lima**
   - Start/reuse an arm64 Lima VM (Lima is installed locally; all currently listed VMs are stopped).
   - Build a small Linux-only test harness around an AF_UNIX datagram receiver that intentionally does not drain its socket.
   - Identify the receiver exactly by `fstat(fd).Ino`, then issue an exact `NETLINK_SOCK_DIAG` / `SOCK_DIAG_BY_FAMILY` request with `UDIAG_SHOW_RQLEN | UDIAG_SHOW_MEMINFO`; avoid a system-wide socket dump and avoid matching by path.
   - Send fixed- and mixed-size datagrams incrementally and record, at each known queue depth:
     - `udiag_rqueue` and `udiag_wqueue`
     - `SK_MEMINFO_RMEM_ALLOC` and `SK_MEMINFO_RCVBUF`
     - `SK_MEMINFO_DROPS`
     - sender success versus `EAGAIN`/blocking
   - Drain packets and verify values return to their idle state. Repeat with configured `SO_RCVBUF` and the default receive buffer.
   - Confirm the query works without `CAP_NET_ADMIN` when the process owns/hosts the socket, and document current-network-namespace behavior.

2. **Choose the useful signal before integrating**
   - Treat `udiag_rqueue > 0` only as a pending-datagram indicator.
   - Prefer `RMEM_ALLOC / RCVBUF` as queue utilization if the experiment demonstrates a stable relationship to saturation.
   - Prefer `DROPS` as a monotonic counter if it increments for Unix datagram receive-queue overflow on supported kernels.
   - Stop after documenting a negative result if none of these distinguishes healthy burst traffic from sustained receiver backpressure.

3. **Add a minimal Linux-only DogStatsD prototype when justified**
   - Put the query implementation behind Linux build constraints, with a no-op/non-Linux implementation so all existing platforms continue to compile.
   - Reuse the listener's `SyscallConn` to obtain the socket inode without duplicating or transferring ownership of the file descriptor.
   - Open one `NETLINK_SOCK_DIAG` socket and sample periodically in a lifecycle-bound goroutine; do not query per packet and do not place netlink work in the receive hot path.
   - Start sampling only for the Unix datagram listener, and stop/close it with the listener.
   - Fail open: inability to open/query/parse diagnostics must never prevent DogStatsD startup or packet intake; log failures with rate limiting and expose an error counter if useful.
   - Initially expose internal telemetry (exact names finalized from experimental semantics), likely including:
     - pending-next-datagram bytes or a boolean pending gauge
     - receive-memory allocated bytes
     - receive-buffer capacity bytes
     - receive-memory utilization ratio
     - socket drop count or delta, if validated
   - Avoid socket-path labels by default to prevent cardinality/path disclosure; use the existing bounded listener identity conventions only if a label is needed.
   - Keep the prototype opt-in unless measurements show negligible overhead and the selected metrics warrant default cross-org Agent telemetry.

4. **Test and measure**
   - Unit-test request encoding, netlink response/attribute parsing, malformed responses, and metric conversion without requiring Linux privileges.
   - Add a Linux integration test using a real Unix datagram socket that verifies idle, queued, drained, and overflow behavior; skip only for a specifically detected unsupported kernel capability.
   - Verify non-Linux compilation through existing build-tag/Bazel targets.
   - Run focused tests with `dda inv test --targets=./comp/dogstatsd/listeners` (never raw `go test`) and relevant Bazel tests if needed.
   - In Lima, compare CPU/syscall overhead at representative polling intervals and confirm the sampler does not measurably affect DogStatsD intake.

5. **Record the result**
   - Document exact kernel semantics and supported kernel behavior in code comments/tests.
   - Summarize whether this should advance from prototype to production telemetry, including metric semantics, polling interval, privilege/namespace limitations, and observed overhead.

## Acceptance criteria

- A repeatable Lima experiment maps known queue states to `RQLEN` and `MEMINFO` values.
- The result explicitly answers whether `rqueue` alone is useful (expected: only as a nonempty indicator for Unix datagrams).
- If a useful signal exists, a Linux-only, fail-open Agent prototype reports it without netlink calls in the receive hot path and has focused unit/integration coverage.
- Existing non-Linux builds remain unaffected.
- Test and overhead results are captured for a production go/no-go decision.
