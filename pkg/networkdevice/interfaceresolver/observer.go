// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

// Observer is the telemetry surface for the resolver. The package
// deliberately defines its own interface instead of pulling in
// pkg/telemetry directly so that callers can wire up whichever
// telemetry backend the agent is using at that integration point
// (libtelemetry, expvar, Prometheus, or a unit-test fake).
//
// All methods MUST be safe to call from any goroutine. Implementations
// must not block — the resolver is on a hot path.
//
// Stable label semantics (do NOT rename without checking dashboards):
//
//   - vendor          : the device vendor as known to the caller
//     (e.g. "cisco", "juniper", "arista"). Empty
//     string is acceptable but discouraged.
//   - outcome         : one of OutcomeMatchedUnique,
//     OutcomeMatchedAmbiguous,
//     OutcomeUnresolvedAmbiguous, OutcomeUnmatched.
//   - rule            : one of the Rule* constants in resolver.go;
//     empty string when no tiebreaker was needed.
//   - candidate_count : the bucketed N from the resolution attempt
//     (see CandidateCountBucket). High-cardinality
//     integers are not safe as metric labels in
//     Datadog, so the resolver buckets first.
//   - if_type         : the IF-MIB ifType integer (used by
//     ObserveIfTypeDistribution only). Cardinality
//     is bounded by the IANAifType registry.
type Observer interface {
	// ObserveResolution is called exactly once per Resolve() call.
	ObserveResolution(vendor string, result Result)

	// ObserveIfTypeDistribution is called once per indexed candidate
	// at Resolver construction time. It enables the
	// "ifType-distribution-by-vendor" PRD metric used to decide
	// whether ifType alone is rich enough on a given vendor.
	ObserveIfTypeDistribution(vendor string, ifType IfType, count int)
}

// CandidateCountBucket bins the per-resolution candidate count into a
// low-cardinality label safe for metric tagging.
//
// Buckets: "0", "1", "2", "3-5", "6-10", "11+". Stable strings — do
// not rename.
func CandidateCountBucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n == 1:
		return "1"
	case n == 2:
		return "2"
	case n <= 5:
		return "3-5"
	case n <= 10:
		return "6-10"
	default:
		return "11+"
	}
}

// noopObserver implements Observer and discards every event. It is
// the default when callers do not supply one — the resolver is then
// silent but still correct.
type noopObserver struct{}

func (noopObserver) ObserveResolution(string, Result)              {}
func (noopObserver) ObserveIfTypeDistribution(string, IfType, int) {}

// NoopObserver returns an Observer that does nothing. Useful in unit
// tests and for the small number of call sites where telemetry is
// undesired (e.g. one-shot CLI scans).
func NoopObserver() Observer { return noopObserver{} }
