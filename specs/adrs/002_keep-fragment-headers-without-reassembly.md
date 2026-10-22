# ADR-002: Preserve useful fragment headers without reassembling payloads

- **Status:** Accepted (reconstructed from author-confirmed PR decisions; not a new
  project approval)
- **Date:** 2026-10-22 (formalized from existing design)
- **Affects:** REQ-PCAP-003, REQ-PCAP-007

## Context

A header-only capture must exclude payload but preserve enough information to attribute
traffic to flows. Treating every fragment as opaque discards the transport header of
first fragments, including ports.
A reviewer proposes either keeping the fragment-aware logic or simplifying it through
packet reassembly, possibly as a configurable choice.
The capture reads at most 128 bytes from each packet, so reassembly would not have the
original packet contents.

## Options considered

1. **Stop all fragments at their IP header.** Avoids special transport decoding but
   loses ports from first fragments.
   This is the behavior identified by an earlier review.
2. **Decode complete transport headers in first fragments without reassembly.** Retains
   useful ports while leaving later fragment payload unrecorded; requires explicit
   malformed and truncated-header handling.
3. **Reassemble fragments before producing the capture, optionally by configuration.**
   Could simplify downstream inspection of complete datagrams, but clipped input
   prevents correct reconstruction and retaining payload for reassembly conflicts with
   the stated security concern.

## Decision

Keep first-fragment transport-header decoding without reassembly.
The author confirms first-fragment TCP/UDP/ICMP handling and tests, then explicitly
rejects reassembly because 128-byte clipping prevents correct reconstruction and holding
payload-bearing packets in memory is undesirable for security.

This is a refinement of the required evidence: “header only” must not accidentally mean
“omit useful transport headers whenever an IP packet is fragmented.”
It does not establish that customers requested a general fragmentation-diagnostics
product.

## Consequences

- **Positive:** First fragments retain complete recognized transport headers without
  reconstructing or recording the datagram payload.
- **Negative:** Fragment parsing needs dedicated edge-case tests; captures do not
  provide reassembled application datagrams.
- **Neutral:** Later fragments stop at the recognized network/fragment headers because
  their remaining bytes are payload.

## References

- [Review identifies lost first-fragment transport headers](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4275837585).
- [Author confirms first-fragment handling and tests](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4318812672).
- [Reviewer proposes reassembly and questions customer need](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4338172848).
- [Author explains clipping and payload-memory constraints](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4352736890).
- `headerLen`, `fragmentHeaderLen`, `completeHeader`, `TestHeaderLen`;
  [requirements](../remote-packet-capture/requirements.md).
