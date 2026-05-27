// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

// IfType is the IF-MIB ifType (IANAifType) for an interface. We only
// reference the values relevant to the V1 tiebreaker; the full registry
// is large and lives in RFC 7224.
type IfType int32

// IF-MIB ifType values used by the resolver.
//
// Sourced from https://www.iana.org/assignments/ianaiftype-mib. Values
// not enumerated here are still valid IfType inputs — they simply do
// not get special treatment.
const (
	// IfTypeOther — "other". Vendors sometimes return this for
	// virtual or proprietary interfaces; treated as "unhelpful" by
	// the resolver.
	IfTypeOther IfType = 1
	// IfTypeEthernetCsmacd — a regular physical Ethernet port.
	IfTypeEthernetCsmacd IfType = 6
	// IfTypePropVirtual — a generic virtual interface (sometimes used
	// for VLAN sub-interfaces or bridge members; vendor-dependent).
	IfTypePropVirtual IfType = 53
	// IfTypeL2Vlan — an IEEE 802.1Q VLAN sub-interface.
	IfTypeL2Vlan IfType = 135
	// IfTypeL3Ipvlan — an IPv4/IPv6 sub-interface.
	IfTypeL3Ipvlan IfType = 136
	// IfTypeIeee8023adLag — a bonding/LACP aggregate interface.
	IfTypeIeee8023adLag IfType = 161
	// IfTypeBridge — a bridge interface (rarely surfaced via IF-MIB
	// but seen on some Linux-based vendors).
	IfTypeBridge IfType = 209
)

// IsPhysical reports whether the given ifType denotes a physical
// Ethernet-like port. Matches the predicate currently used in
// InterfaceMetadata.IsPhysical for ifType 6/62/69/117.
func (t IfType) IsPhysical() bool {
	switch t {
	case 6, 62, 69, 117:
		return true
	}
	return false
}

// IsVirtual reports whether the given ifType denotes a logical/virtual
// interface that commonly inherits its parent's MAC address.
//
// Note: ieee8023adLag (161) is intentionally NOT in this set. LAGs
// aggregate physical NICs and the collision shape they create is
// "multiple physicals share the LAG's MAC", not "sub-interfaces share
// a parent's MAC". Rule (b) of the resolver is about the latter; the
// LAG case is handled by the generalised "match ExpectedType exactly"
// rule.
func (t IfType) IsVirtual() bool {
	switch t {
	case IfTypePropVirtual, IfTypeL2Vlan, IfTypeL3Ipvlan, IfTypeBridge:
		return true
	}
	return false
}

// IsAmbiguousVendorValue reports whether the value is one of the
// vendor-noisy values (1 = other, 53 = propVirtual) that the resolver
// should treat as "no signal" when scoring candidates.
//
// Note: we keep 53 (propVirtual) in this set because at least one major
// vendor returns it generically for both VLAN and tunnel interfaces.
// Rule (a) of the tiebreaker still prefers ethernetCsmacd over
// propVirtual when context says "physical"; this predicate is used
// only by Auditor for "ifType not helpful" classification.
func (t IfType) IsAmbiguousVendorValue() bool {
	return t == IfTypeOther || t == IfTypePropVirtual
}

// Candidate represents one interface that may be the resolution target.
// The package keeps this struct minimal on purpose — heavy fields like
// admin/oper status live elsewhere and are not load-bearing for the V1
// tiebreaker.
type Candidate struct {
	// Index is the IF-MIB ifIndex (or vendor analogue). Must be > 0.
	Index int32
	// Name is the ifName (or ifDescr if ifName is empty). Used by
	// the longest-common-prefix tiebreaker.
	Name string
	// MAC is the lowercase, colon-separated MAC address. Empty MAC
	// candidates are still indexed by Name but ignored by the
	// MAC-keyed lookup path.
	MAC string
	// Type is the IF-MIB ifType. Zero means "unknown / not collected"
	// and disables the type-based tiebreaker rules for this
	// candidate (without failing resolution).
	Type IfType
}

// ContextHint is the optional context the caller passes to the
// resolver. Every field is optional; the more fields populated, the
// fewer ambiguous resolutions surface.
type ContextHint struct {
	// ExpectedType, if non-zero, indicates the type of interface the
	// caller is looking for (e.g. a NetFlow record that came from a
	// physical port supplies ExpectedType=IfTypeEthernetCsmacd).
	ExpectedType IfType
	// VLANID, if non-zero, indicates the caller is looking for a
	// VLAN/sub-interface tagged with this VLAN ID. Activates the
	// "prefer virtual" rule.
	VLANID int
	// NameHint, if non-empty, is the caller's best guess at the
	// interface name. Used by the longest-common-prefix tiebreaker
	// as the final disambiguation step.
	NameHint string
}

// Outcome categorises the result of one Resolve call. Stable string
// values — these are emitted as a telemetry label and must not be
// renamed without coordinating with downstream dashboards.
type Outcome string

// Outcome enum values.
const (
	// OutcomeMatchedUnique — exactly one candidate matched.
	OutcomeMatchedUnique Outcome = "matched_unique"
	// OutcomeMatchedAmbiguous — multiple candidates matched and the
	// tiebreaker picked one of them.
	OutcomeMatchedAmbiguous Outcome = "matched_ambiguous"
	// OutcomeUnresolvedAmbiguous — multiple candidates matched and
	// the tiebreaker could not narrow them down; the caller MUST NOT
	// guess.
	OutcomeUnresolvedAmbiguous Outcome = "unresolved_ambiguous"
	// OutcomeUnmatched — no candidate matched on MAC at all.
	OutcomeUnmatched Outcome = "unmatched"
)

// Result is what Resolve returns. Interface is nil iff the resolution
// failed (Unmatched or UnresolvedAmbiguous); inspect Outcome and
// CandidateCount to distinguish.
type Result struct {
	// Interface is the chosen candidate. Nil when Outcome is
	// OutcomeUnmatched or OutcomeUnresolvedAmbiguous.
	Interface *Candidate
	// Outcome categorises this resolution attempt.
	Outcome Outcome
	// CandidateCount is the number of candidates that matched the
	// primary key (MAC) before the tiebreaker ran. 0 for Unmatched.
	CandidateCount int
	// TiebreakerRule, when Outcome is MatchedAmbiguous, identifies
	// which tiebreaker rule fired. Empty otherwise. Stable string
	// values — used as a telemetry label.
	TiebreakerRule string
}
