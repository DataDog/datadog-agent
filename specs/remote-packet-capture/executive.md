# Remote packet capture — executive status

## Requirements summary

Network operators can request a short host capture remotely, select an interface and traffic filter, and receive a standard artifact containing recognized protocol headers without application payloads. Collection is opt-in and bounded by time, packet count, and artifact size. The action delivers the artifact with a correlation identifier and reports counts that help operators assess missing or truncated evidence. Deployments can expose a capture-only local channel rather than giving the runner general system-probe access.

## Technical summary

Pinned PR head: `5190445c44de7544b67072705218f4af2dd4a1b5`, inspected 2026-10-22. The runner invokes a Linux/pcap/cgo system-probe module over a Unix socket, streams PCAP into a temporary file, reads final statistics from HTTP trailers, and uploads a replayable compressed multipart attachment through a new fileupload component. The component applies configured endpoints, credentials, retry classification, and failover routing. [ADR-001](../adrs/001_capture-limits-cannot-be-raised.md), [ADR-002](../adrs/002_keep-fragment-headers-without-reassembly.md), and [ADR-003](../adrs/003_bound-writes-before-waiting-for-teardown.md) reconstruct documented decisions. The [Allium model](remote-packet-capture.allium) specifies the ordered capture/stream/finalize/upload/cleanup lifecycle and three model invariants; it does not replace packet-decoder tests.

## Status summary

This is retrospective extraction from an existing PR, not a requirements-first implementation exercise. All requirement wording is reconstructed for this case study; [traceability](../traceability.md) distinguishes explicit intent from inferred contracts. Allium 3.5.2 checking passed and candidate test obligations were generated; no spec–code divergence check was run, and three model questions remain unresolved. No Agent tests were executed locally, and test-file inspection is not a passing test result. No claim of merge, deployment, production verification, or complete requirement coverage is made.

| Requirement | Status | Evidence and limits |
| --- | --- | --- |
| REQ-PCAP-001: Request targeted evidence | ⚠️ Static evidence | Action/trigger/filter paths and test definitions found; interface/BPF behavior needs the real capture environment. |
| REQ-PCAP-002: Enable capture deliberately | ⚠️ Static evidence | Opt-in config, build constraints, and unsupported stub found; supported build/config matrix not executed. |
| REQ-PCAP-003: Exclude payloads | ⚠️ Static evidence | Allowlisted header parsing and broad malformed/fragment fixtures found; no universal privacy proof. |
| REQ-PCAP-004: Bound collection | ⚠️ Static evidence | Both boundaries clamp inputs; byte-accounting/readback tests found. Packet-count stop and full timing behavior not exercised. |
| REQ-PCAP-005: Finish collection | ⚠️ Static evidence | Idle capture test and deadline setter test found. No full stalled-reader teardown test found. |
| REQ-PCAP-006: Surface failures | ⚠️ Static evidence | Read-error, error-trailer, HTTP status, and upload-failure tests found. Some startup comments describe an older order than the pinned source. |
| REQ-PCAP-007: Correlate and assess | ⚠️ Static evidence | Identifier, metadata, and count propagation tests found. Missing/malformed trailer counts become zero. |
| REQ-PCAP-008: Deliver with recovery | ⚠️ Static evidence | Multipart, retry, cancellation, and upload-error tests found; real-intake tests are opt-in and were not run. |
| REQ-PCAP-009: Honor destinations | ⚠️ Static evidence | Additional-endpoint and failover tests found; full endpoint/credential runtime behavior not exercised. |
| REQ-PCAP-010: Restrict access | ⚠️ Static evidence | Route-isolation, normalized-path escape, and channel-selection tests found; container mount and permission isolation not verified. |
| REQ-PCAP-011: Remove temp evidence | ⚠️ Static evidence | Upload success/failure removal assertions found. Removal errors are ignored; cleanup after process crash is not promised. |

**Verified completion:** 0 of 11 requirements independently validated by execution. Implementation and partial test definitions were found for all 11.

## Boundaries and validation gaps

- This PR supplies the host/action/delivery path. Backend retrieval UX, customer authorization policy, and Helm/Operator rollout are outside this extracted contract. Existing PAR authorization is a dependency, not a newly demonstrated guarantee.
- A byte cap below 24 cannot be a literal file-size bound: the PCAP header is always emitted. The requirements make that exception explicit instead of silently strengthening the code's contract.
- Header-only does not mean anonymous or free of sensitive information. Recognized headers still contain addresses, ports, and other header fields. No additional redaction requirement is inferred.
- The server sets HTTP 200 through the first successful output write and sets response headers before Start; old client/test comments still claim success is committed before Start. Startup output failures may already have written bytes, so do not infer universal HTTP error-status behavior from the startup branch alone.
- `trailerInt` maps missing, malformed, and negative values through integer parsing without validation. A missing capture-error trailer is treated as zero. The code therefore does not establish a strict “validated complete capture report” contract.
- Stop-time statistics errors are logged rather than propagated. Known positive capture-error trailers fail the real socket trigger, although a fake trigger can produce a successful action result containing a nonzero error count.
- No global capture concurrency/admission policy, crash-recovery spool, failed-upload retention, or duplicate-capture-ID policy was found. Per-capture ceilings do not establish a fleet-wide or process-wide resource bound.

## Open questions and future directions

- Should the product reject byte limits below the file-header floor instead of returning a header larger than the requested limit?
- Should absent or malformed final statistics fail the action rather than look like a clean zero-error capture?
- What concurrent-capture budget and failure-recovery retention policy should apply under production load?
- What integration evidence is required before calling the deployed runner/socket/module/intake path complete?
