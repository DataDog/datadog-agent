---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Existing Antithesis SDK Assertions

## Summary

**No Antithesis SDK instrumentation exists in the codebase.**

A scan for the Antithesis Go SDK (`github.com/antithesishq/antithesis-sdk-go`)
and its assertion functions (`assert.Always`, `assert.Sometimes`,
`assert.Reachable`, `assert.Unreachable`, `assert.AlwaysOrUnreachable`) found no
usages anywhere in `pkg/`, `comp/`, or `cmd/`.

## Scan method

```
grep -rnil "antithesis" --include="*.go" pkg comp cmd   # → no matches
grep -rn  "antithesissdk|antithesis/sdk" --include="*.go"  # → no matches
```

The only textual matches for `Reachable` / `Sometimes` / `Unreachable` in the
tree are unrelated:

- `test/fakeintake/aggregator/netpathAggregator_test.go` — a `Reachable bool`
  field on a network-path hop model.
- `comp/forwarder/defaultforwarder/impl/forwarder_health.go` — an
  `apiKeyEndpointUnreachable` status constant.
- Various comments containing the English word "Sometimes."

None of these are Antithesis SDK calls.

## Implication

Every property in the catalog that needs SUT-side branch guidance or a replay
anchor represents **net-new instrumentation**. No evidence file should describe
an assertion as "already present" or "partially present" — the baseline is zero.
The Antithesis SDK dependency itself is not yet in `go.mod` for any module and
would need to be added to whichever module hosts the instrumented code (most
cluster-agent logic lives in the root `github.com/DataDog/datadog-agent`
module).
