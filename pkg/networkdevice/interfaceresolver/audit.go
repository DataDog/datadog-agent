// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

// OracleEntry is one ground-truth resolution sample. In production the
// oracle is constructed from LLDP-MIB plus ifName exact-match on a
// subset of devices (PRD scope item #4); in tests it is constructed
// from a known-good mapping.
//
// IfIndex is the correct interface for the (MAC, Hint) pair. Setting
// IfIndex to 0 means "the oracle says this MAC has no canonical
// interface" — the resolver is expected to return OutcomeUnmatched.
type OracleEntry struct {
	MAC      string
	Hint     ContextHint
	IfIndex  int32
}

// AuditReport summarises the audit job's findings. Counts are
// per-device — caller aggregates across devices as needed.
//
// MisResolutionRate is the PRD's correctness guardrail (target <0.5%).
// Coverage without correctness is a regression — so this report makes
// the trade-off explicit.
type AuditReport struct {
	// Sample is the number of oracle entries evaluated.
	Sample int
	// MatchedCorrect counts attempts where the resolver returned the
	// same IfIndex the oracle said was correct.
	MatchedCorrect int
	// MatchedIncorrect counts attempts where the resolver returned an
	// IfIndex but it disagreed with the oracle. This is the
	// "mis-resolution" bucket.
	MatchedIncorrect int
	// UnresolvedAmbiguous counts attempts where the resolver
	// surfaced ambiguity rather than guessing. NOT a correctness
	// failure — it's the safe path.
	UnresolvedAmbiguous int
	// Unmatched counts attempts where the resolver returned no
	// candidate. Whether this is a failure depends on the oracle:
	// if the oracle said "no interface" (IfIndex==0) too, this is
	// correct. The audit folds the "correct unmatched" into
	// MatchedCorrect.
	Unmatched int
}

// MisResolutionRate returns MatchedIncorrect / (MatchedCorrect +
// MatchedIncorrect). Returns 0 when the denominator is 0 (no resolved
// samples at all).
//
// We exclude unresolved/unmatched from the denominator on purpose —
// they are coverage events, not correctness events, and conflating them
// would let coverage drops mask mis-resolution regressions.
func (r AuditReport) MisResolutionRate() float64 {
	denom := r.MatchedCorrect + r.MatchedIncorrect
	if denom == 0 {
		return 0
	}
	return float64(r.MatchedIncorrect) / float64(denom)
}

// CoverageRate returns 1 - (Unmatched + UnresolvedAmbiguous) / Sample.
// Returns 0 when Sample is 0.
//
// Note: a correct "unmatched" (oracle also said no interface) still
// counts toward MatchedCorrect, NOT toward Unmatched. So this rate is
// the fraction of attempts where the resolver returned a usable
// answer.
func (r AuditReport) CoverageRate() float64 {
	if r.Sample == 0 {
		return 0
	}
	resolved := r.MatchedCorrect + r.MatchedIncorrect
	return float64(resolved) / float64(r.Sample)
}

// Auditor runs the audit job for one device's Resolver against a set
// of oracle entries.
type Auditor struct {
	resolver *Resolver
}

// NewAuditor binds an auditor to a built Resolver.
func NewAuditor(r *Resolver) *Auditor {
	return &Auditor{resolver: r}
}

// Run evaluates the resolver against every entry in `oracle` and
// returns an AuditReport.
//
// The audit deliberately reuses the public Resolve path so the
// resolver's instrumentation (per-resolution Observer events) fires
// during the audit. Callers who want to keep audit events out of
// their production resolution metrics should construct a dedicated
// Resolver for the audit with a separate Observer (e.g. a
// per-audit fakeObserver).
func (a *Auditor) Run(oracle []OracleEntry) AuditReport {
	rep := AuditReport{Sample: len(oracle)}
	for _, e := range oracle {
		res := a.resolver.Resolve(e.MAC, e.Hint)
		switch res.Outcome {
		case OutcomeMatchedUnique, OutcomeMatchedAmbiguous:
			// resolver returned an interface; check against oracle.
			if e.IfIndex == 0 {
				// Oracle says "no interface for this MAC" but
				// resolver found one. That's a mis-resolution.
				rep.MatchedIncorrect++
				continue
			}
			if res.Interface != nil && res.Interface.Index == e.IfIndex {
				rep.MatchedCorrect++
			} else {
				rep.MatchedIncorrect++
			}
		case OutcomeUnresolvedAmbiguous:
			rep.UnresolvedAmbiguous++
		case OutcomeUnmatched:
			// Oracle agrees "no interface" → correct unmatched.
			// Oracle disagrees → coverage miss, NOT correctness.
			if e.IfIndex == 0 {
				rep.MatchedCorrect++
			} else {
				rep.Unmatched++
			}
		}
	}
	return rep
}
