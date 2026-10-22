# Decision index

These are retrospective records reconstructed from review comments, author replies, and pinned source. “Accepted” means the source shows the author chose and implemented the option; it does not claim that this local case study or the PR received project approval. The chain captures three documented refinements, not an invented complete history of the feature. Dates are the author-confirmed decision dates; formalization happened locally on 2026-10-22.

| ADR | Decision | Lifecycle | Affects |
| --- | --- | --- | --- |
| [001](001_capture-limits-cannot-be-raised.md) | Callers may lower capture limits but cannot raise them | Accepted, reconstructed | REQ-PCAP-004, REQ-PCAP-008 |
| [002](002_keep-fragment-headers-without-reassembly.md) | Preserve useful fragment headers without reassembling payloads | Accepted, reconstructed | REQ-PCAP-003, REQ-PCAP-007 |
| [003](003_bound-writes-before-waiting-for-teardown.md) | Bound response writes before waiting for teardown | Accepted, reconstructed | REQ-PCAP-004, REQ-PCAP-005, REQ-PCAP-006 |

## Task routing

| Work | Read |
| --- | --- |
| Change capture limits or delivery buffering | ADR-001 |
| Change fragment parsing, payload exclusion, or reassembly | ADR-002 |
| Change streaming, cancellation, or teardown | ADR-003 and ADR-001 |

## Decision dependencies

ADR-001 establishes hard bounds on one capture's output. ADR-003 complements it by bounding an independent source of resource retention: a blocked response writer. ADR-002 is independent and refines which evidence belongs within the capture's privacy boundary.
