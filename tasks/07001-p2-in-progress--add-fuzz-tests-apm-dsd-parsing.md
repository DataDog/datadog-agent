---
created: 2026-04-09
priority: p2
status: in-progress
artifact: pending
---

# add-fuzz-tests-apm-dsd-parsing

## Plan


# Add Fuzz Tests for Uncovered DogStatsD/APM Parsing Codepaths

## Summary
Add fuzz tests targeting the critical gaps in DogStatsD and APM trace parsing — the primary attack surface where untrusted application data enters the agent. The existing fuzz coverage is good at the API handler level but misses foundational primitives and the OTLP ingestion path.

## Context
DogStatsD (UDP/UDS 8125) and APM (TCP/UDS 8126) are the agent's most attacker-reachable parsing surfaces. Every instrumented application on every customer host sends data to these endpoints. A bug in deserialization could escalate from a compromised app dependency to agent compromise. The existing fuzz tests cover API-level handlers (v02-v07) but miss:

1. **The low-level msgpack primitives** that every span field passes through (decoder_bytes.go) — 0 fuzz tests
2. **The OTLP ingestion path** — 0 fuzz tests despite growing adoption
3. **The V10 trace endpoint** — new format, 0 fuzz tests
4. **Origin detection parsing** — untrusted container ID/inode parsing, 0 fuzz tests

## What To Do

### 1. Fuzz `decoder_bytes.go` primitives (pkg/proto/pbgo/trace/)
Create `decoder_bytes_fuzz_test.go` with:

- **`FuzzRepairUTF8`** — Fuzz `repairUTF8(string)` with random byte strings. Assert: output is always valid UTF-8, never panics. This function has unbounded CPU cost on adversarial input.
- **`FuzzParseStringBytes`** — Fuzz `parseStringBytes([]byte)` with random msgpack-encoded payloads. Assert: never panics, returned string is valid UTF-8, remaining bytes + consumed bytes = input length.
- **`FuzzParseInt64Bytes`** — Fuzz `parseInt64Bytes([]byte)` with msgpack int/uint payloads. Assert: never panics, overflow cases return error.
- **`FuzzParseUint64Bytes`** — Fuzz `parseUint64Bytes([]byte)` similarly.
- **`FuzzParseFloat64Bytes`** — Fuzz `parseFloat64Bytes([]byte)` with all msgpack numeric types.

Seed corpus: use `msgp.AppendString`, `msgp.AppendInt64`, `msgp.AppendUint64`, `msgp.AppendFloat64` to generate valid seeds, plus hand-crafted edge cases (MaxInt64, MaxUint64, empty, nil bytes, invalid type headers).

### 2. Fuzz OTLP trace ingestion (pkg/trace/api/)
Create `otlp_fuzz_test.go` with:

- **`FuzzOTLPReceiveResourceSpans`** — Construct randomized `ptrace.ResourceSpans` using the existing `testutil.NewOTLPTracesRequest()` helpers. Vary: number of spans, attribute types (string/int/double/bool/map/slice), trace IDs, span kinds, timestamps, events, links. Assert: never panics, output payloads have valid structure.

Use `testing.F` with seed corpus from existing test cases in `otlp_test.go`. The fuzz target should:
1. Build an `OTLPReceiver` with test config (use patterns from existing tests)
2. Feed fuzzed `ptrace.ResourceSpans` to `ReceiveResourceSpans()`
3. Assert no panic, check output channel for well-formed payloads

Note: Since OTLP input is structured protobuf (not raw bytes), this fuzz test will need to use the structured fuzzing approach — fuzz the byte-level protobuf encoding that gets deserialized into `ptrace.ResourceSpans`, OR fuzz the parameters used to construct spans via `testutil`.

### 3. Fuzz V10 trace endpoint (pkg/trace/api/)
Add to existing `fuzz_test.go`:

- **`FuzzHandleTracesV10`** — Follow the exact pattern of `FuzzHandleTracesV07` but target the V10 endpoint (`/v1.0/traces`). Use `idx.InternalTracerPayload` msgpack encoding for seed corpus. The V10 path uses `decodeConvertedTracerPayload` which calls `InternalTracerPayload.UnmarshalMsg()`.

### 4. Fuzz origin detection parsing (comp/core/tagger/origindetection/)
Create `origindetection_fuzz_test.go` with:

- **`FuzzParseLocalData`** — Fuzz with random strings. Assert: never panics, parsed container IDs and inodes are consistent with input prefixes.
- **`FuzzParseExternalData`** — Fuzz with random comma-separated strings. Assert: never panics, parsed fields match expected prefix structure.

Seed corpus: `"ci-abc123"`, `"cid-legacy"`, `"in-12345"`, `"ci-abc,in-999"`, `"it-true,cn-mycontainer,pu-uid-here"`, empty string, very long strings.

## Acceptance Criteria
- [ ] All new fuzz tests compile and pass with `go test -run=... -count=1`
- [ ] Each fuzz test runs for 10 seconds without finding issues: `go test -fuzz=FuzzXxx -fuzztime=10s`
- [ ] Each fuzz test has meaningful seed corpus (at least 3 seeds per test: valid input, edge case, adversarial)
- [ ] Property assertions beyond "no panic": UTF-8 validity, idempotency where applicable, byte accounting
- [ ] No modifications to production code — fuzz tests only
- [ ] Tests follow existing patterns (see `pkg/trace/api/fuzz_test.go` and `comp/dogstatsd/server/parse_metrics_fuzz_test.go` for style)


## Progress

