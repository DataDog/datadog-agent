# ADR-001: Callers may lower capture limits but cannot raise them

- **Status:** Accepted (reconstructed from author-confirmed PR decisions; not a new
  project approval)
- **Date:** 2026-10-19 (formalized from existing design on 2026-10-22)
- **Affects:** REQ-PCAP-004, REQ-PCAP-008

## Context

A duration and a short per-packet read do not adequately constrain capture-file size on
a busy host. The initial positive caller-supplied packet and byte limits can exceed the
defaults. The initial upload path also reads the whole file into memory and retains it
through retries. A reviewer identifies the combination as a potentially large
resident-memory cost in the action runner.

## Options considered

These alternatives are evidenced in the review exchange, rather than inferred as a
hypothetical historical discussion.

1. **Default only non-positive limits and upload from a complete in-memory buffer.**
   Preserves arbitrary positive caller choices and supplies reusable upload bytes, but
   permits unexpectedly large captures and resident buffers.
2. **Clamp at fixed ceilings and reopen a streamed file for each upload attempt.**
   Bounds per-capture output while avoiding a second complete artifact in memory.
   Introduces replayable attachment readers and more file/body lifecycle handling.

## Decision

The author confirms both changes: positive caller limits can only lower the ceilings,
and uploads reopen the capture file for each attempt instead of using a complete
in-memory buffer.
Enforce duration and output limits where capture executes as well as at
the runner boundary.
The reviewer explicitly identifies direct callers of the privileged endpoint as a reason
not to trust runner-only validation.

This changes the meaning of the contract: “defaults” are enforceable ceilings, not
suggestions a caller can override.
Streaming is the chosen implementation for controlling delivery memory; it is not a
requirement that every future implementation use the same Go helpers.

## Consequences

- **Positive:** A single caller cannot request effectively unlimited output by supplying
  enormous positive limits; retries need not retain the entire file in runner memory.
- **Negative:** Operators cannot raise the ceilings for unusually large investigations.
  Delivery now depends on a readable local artifact across attempts.
- **Neutral:** The file-size bound counts PCAP framing as well as packet bytes; a file
  header is emitted even for a lower requested cap.

## References

- [Reviewer identifies unbounded limits and whole-file buffering](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4275837111).
- [Author confirms clamping and replayable streaming](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4318738920).
- [Reviewer requests enforcement at the execution boundary](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4275837797).
- `RunCaptureHandler.Run`, `boundRequest`, `capturer.drainLoop`, `uploader.send`,
  `newMultipartBody`; [requirements](../remote-packet-capture/requirements.md).
