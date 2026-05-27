// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

import (
	"strings"
)

// Tiebreaker rule names emitted on Result.TiebreakerRule. These are
// stable strings — see the design note in types.go about Outcome
// labels.
const (
	RuleNone                = ""
	RulePreferPhysical      = "prefer_physical"
	RulePreferVirtual       = "prefer_virtual"
	RuleMatchExpectedType   = "match_expected_type"
	RuleLongestCommonPrefix = "longest_common_prefix"
)

// Resolver maps MAC addresses to interface candidates. One Resolver
// instance corresponds to one device's interface table. The Resolver
// is read-only after construction (NewResolver builds the index in one
// pass) and therefore safe to share across goroutines.
type Resolver struct {
	// vendor is the device vendor as known to the caller. Used as
	// a telemetry label only. Empty string is acceptable but
	// discouraged.
	vendor string
	// observer receives instrumentation events. Never nil — defaults
	// to NoopObserver.
	observer Observer
	// byMAC maps lowercase MAC address → all candidates carrying it.
	byMAC map[string][]Candidate
}

// Option configures a Resolver at construction time.
type Option func(*Resolver)

// WithVendor sets the device vendor label used by the Observer.
// Vendor strings should be lowercase and short (e.g. "cisco",
// "juniper", "arista"). Cardinality is bounded by the number of
// supported vendors.
func WithVendor(vendor string) Option {
	return func(r *Resolver) { r.vendor = vendor }
}

// WithObserver attaches an Observer for instrumentation. Nil is
// accepted and treated as NoopObserver.
func WithObserver(o Observer) Option {
	return func(r *Resolver) {
		if o == nil {
			o = NoopObserver()
		}
		r.observer = o
	}
}

// NewResolver builds a Resolver over the given candidates. Duplicate
// candidates (same ifIndex) are de-duplicated keeping the first
// occurrence. Candidates with empty MAC are tolerated but not indexed.
// MAC normalisation is canonical-lowercase, colon-separated; callers
// are expected to already use formatColonSepBytes from internal/report.
//
// Instrumentation: during construction, the resolver emits one
// ObserveIfTypeDistribution event per (ifType, count) pair seen.
// This is the per-device snapshot the PRD calls for in step 1 of
// scope ("instrument ifType value distribution per vendor").
func NewResolver(candidates []Candidate, opts ...Option) *Resolver {
	r := &Resolver{
		observer: NoopObserver(),
		byMAC:    make(map[string][]Candidate, len(candidates)),
	}
	for _, opt := range opts {
		opt(r)
	}
	seenIdx := make(map[int32]struct{}, len(candidates))
	typeHist := make(map[IfType]int)
	for _, c := range candidates {
		if c.Index <= 0 {
			continue
		}
		if _, dup := seenIdx[c.Index]; dup {
			continue
		}
		seenIdx[c.Index] = struct{}{}
		typeHist[c.Type]++
		mac := normalizeMAC(c.MAC)
		if mac == "" {
			continue
		}
		c.MAC = mac
		r.byMAC[mac] = append(r.byMAC[mac], c)
	}
	for t, n := range typeHist {
		r.observer.ObserveIfTypeDistribution(r.vendor, t, n)
	}
	return r
}

// Resolve returns the canonical interface for the given MAC plus
// optional context hint.
//
// Tiebreaker priority (V1, matches PRD scope item #3):
//
//	(a) prefer ethernetCsmacd (6) over l2vlan/propVirtual when context
//	    indicates a physical port (ExpectedType.IsPhysical()).
//	(b) prefer the virtual candidate (l2vlan / propVirtual / l3ipvlan /
//	    bridge) when context carries a VLAN ID or names a virtual
//	    ExpectedType.
//	(c) prefer the candidate whose ifName is the longest common prefix
//	    of the context's NameHint.
//	(d) if still ambiguous, return OutcomeUnresolvedAmbiguous and let
//	    the caller decide — DO NOT guess silently.
//
// Rules (a) and (b) are mutually exclusive on a given call (they read
// different fields of ContextHint). If neither rule (a/b) nor rule (c)
// can break the tie, the result is unresolved.
func (r *Resolver) Resolve(mac string, hint ContextHint) Result {
	res := r.resolve(mac, hint)
	r.observer.ObserveResolution(r.vendor, res)
	return res
}

// resolve is the pure tiebreaker logic — no instrumentation. Exposed
// privately so tests can exercise it without a fake Observer.
func (r *Resolver) resolve(mac string, hint ContextHint) Result {
	mac = normalizeMAC(mac)
	candidates := r.byMAC[mac]
	switch len(candidates) {
	case 0:
		return Result{Outcome: OutcomeUnmatched, CandidateCount: 0}
	case 1:
		c := candidates[0]
		return Result{Interface: &c, Outcome: OutcomeMatchedUnique, CandidateCount: 1}
	}

	// Rule (a): caller asked for a physical port.
	if hint.ExpectedType.IsPhysical() {
		if pick, ok := pickUnique(candidates, func(c Candidate) bool {
			return c.Type.IsPhysical()
		}); ok {
			return Result{
				Interface:      &pick,
				Outcome:        OutcomeMatchedAmbiguous,
				CandidateCount: len(candidates),
				TiebreakerRule: RulePreferPhysical,
			}
		}
	}

	// Rule (b): caller asked for a virtual / sub-interface.
	if hint.VLANID > 0 || hint.ExpectedType.IsVirtual() {
		if pick, ok := pickUnique(candidates, func(c Candidate) bool {
			return c.Type.IsVirtual()
		}); ok {
			return Result{
				Interface:      &pick,
				Outcome:        OutcomeMatchedAmbiguous,
				CandidateCount: len(candidates),
				TiebreakerRule: RulePreferVirtual,
			}
		}
	}

	// Rule (a/b) generalised: prefer the candidate whose Type
	// equals ExpectedType exactly. This handles non-physical /
	// non-vlan ExpectedType values (e.g. LAG).
	if hint.ExpectedType != 0 {
		if pick, ok := pickUnique(candidates, func(c Candidate) bool {
			return c.Type == hint.ExpectedType
		}); ok {
			return Result{
				Interface:      &pick,
				Outcome:        OutcomeMatchedAmbiguous,
				CandidateCount: len(candidates),
				TiebreakerRule: RuleMatchExpectedType,
			}
		}
	}

	// Rule (c): longest common prefix vs NameHint. We require a
	// strict winner — if two candidates tie on prefix length, we do
	// NOT pick.
	if hint.NameHint != "" {
		if pick, ok := pickByLongestCommonPrefix(candidates, hint.NameHint); ok {
			return Result{
				Interface:      &pick,
				Outcome:        OutcomeMatchedAmbiguous,
				CandidateCount: len(candidates),
				TiebreakerRule: RuleLongestCommonPrefix,
			}
		}
	}

	// Rule (d): give up safely. The caller can log + skip; we never
	// silently pick one of the candidates.
	return Result{
		Outcome:        OutcomeUnresolvedAmbiguous,
		CandidateCount: len(candidates),
	}
}

// CandidatesForMAC exposes the raw candidate list for a MAC. Returns
// the empty slice when MAC is unknown. Callers should not mutate the
// returned slice. Intended for use by the Auditor and by debug tooling.
func (r *Resolver) CandidatesForMAC(mac string) []Candidate {
	return r.byMAC[normalizeMAC(mac)]
}

// AmbiguousMACs returns every MAC for which the resolver has more than
// one candidate. Useful for the Auditor's mis-resolution sampling and
// for the per-device "MAC-collision rate" telemetry counter.
func (r *Resolver) AmbiguousMACs() []string {
	var out []string
	for mac, cs := range r.byMAC {
		if len(cs) > 1 {
			out = append(out, mac)
		}
	}
	return out
}

// pickUnique returns the only candidate matching pred. If zero or >1
// match, ok is false.
func pickUnique(cs []Candidate, pred func(Candidate) bool) (Candidate, bool) {
	var pick Candidate
	count := 0
	for _, c := range cs {
		if pred(c) {
			pick = c
			count++
		}
	}
	if count == 1 {
		return pick, true
	}
	return Candidate{}, false
}

// pickByLongestCommonPrefix returns the candidate whose Name shares
// the longest common prefix with hint. Requires a strict winner — if
// multiple candidates tie on prefix length, ok is false.
//
// A zero-length common prefix is treated as "no match" — otherwise
// every candidate would tie at length 0 and we'd silently pick the
// first.
func pickByLongestCommonPrefix(cs []Candidate, hint string) (Candidate, bool) {
	bestLen := 0
	tie := false
	var pick Candidate
	for _, c := range cs {
		n := commonPrefixLen(c.Name, hint)
		switch {
		case n > bestLen:
			bestLen = n
			pick = c
			tie = false
		case n == bestLen && bestLen > 0:
			tie = true
		}
	}
	if bestLen == 0 || tie {
		return Candidate{}, false
	}
	return pick, true
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// normalizeMAC lowercases a MAC and trims surrounding whitespace.
// We do NOT do format conversion (e.g. dashes → colons) here — callers
// upstream are expected to use formatColonSepBytes so all stored MACs
// are already canonical. This function exists only to defend against
// case mismatches.
func normalizeMAC(mac string) string {
	if mac == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(mac))
}
