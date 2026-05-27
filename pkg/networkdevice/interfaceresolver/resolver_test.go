// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data: a typical Cisco-ish device with one physical port and two
// VLAN sub-interfaces, all sharing the parent MAC. This is the canonical
// shape the PRD calls out as the riskiest collision.
func vlanColocatedCandidates() []Candidate {
	mac := "aa:bb:cc:dd:ee:01"
	return []Candidate{
		{Index: 1, Name: "Gi0/1", MAC: mac, Type: IfTypeEthernetCsmacd},
		{Index: 101, Name: "Gi0/1.100", MAC: mac, Type: IfTypeL2Vlan},
		{Index: 102, Name: "Gi0/1.200", MAC: mac, Type: IfTypeL2Vlan},
	}
}

func TestNewResolver_SkipsInvalidCandidates(t *testing.T) {
	r := NewResolver([]Candidate{
		{Index: 0, Name: "bogus", MAC: "aa:bb:cc:dd:ee:00"},  // ifIndex 0
		{Index: -1, Name: "bogus", MAC: "aa:bb:cc:dd:ee:00"}, // negative ifIndex
		{Index: 1, Name: "Gi0/1", MAC: ""},                   // empty MAC
		{Index: 2, Name: "Gi0/2", MAC: "aa:bb:cc:dd:ee:02"},
		{Index: 2, Name: "Gi0/2-dup", MAC: "aa:bb:cc:dd:ee:02"}, // dup ifIndex
	})

	assert.Len(t, r.CandidatesForMAC("aa:bb:cc:dd:ee:02"), 1)
	assert.Empty(t, r.CandidatesForMAC("aa:bb:cc:dd:ee:00"))
}

func TestResolve_UniqueMatch(t *testing.T) {
	r := NewResolver([]Candidate{
		{Index: 5, Name: "Gi0/5", MAC: "aa:bb:cc:dd:ee:05", Type: IfTypeEthernetCsmacd},
	})

	res := r.Resolve("aa:bb:cc:dd:ee:05", ContextHint{})

	assert.Equal(t, OutcomeMatchedUnique, res.Outcome)
	require.NotNil(t, res.Interface)
	assert.Equal(t, int32(5), res.Interface.Index)
	assert.Equal(t, 1, res.CandidateCount)
	assert.Equal(t, RuleNone, res.TiebreakerRule)
}

func TestResolve_Unmatched(t *testing.T) {
	r := NewResolver([]Candidate{
		{Index: 5, Name: "Gi0/5", MAC: "aa:bb:cc:dd:ee:05", Type: IfTypeEthernetCsmacd},
	})

	res := r.Resolve("00:00:00:00:00:00", ContextHint{})

	assert.Equal(t, OutcomeUnmatched, res.Outcome)
	assert.Nil(t, res.Interface)
	assert.Equal(t, 0, res.CandidateCount)
}

func TestResolve_MACCaseInsensitive(t *testing.T) {
	r := NewResolver([]Candidate{
		{Index: 5, Name: "Gi0/5", MAC: "AA:BB:CC:DD:EE:05", Type: IfTypeEthernetCsmacd},
	})

	res := r.Resolve("aa:bb:cc:dd:ee:05", ContextHint{})

	assert.Equal(t, OutcomeMatchedUnique, res.Outcome)
	require.NotNil(t, res.Interface)
	// Stored MAC is normalised lowercase.
	assert.Equal(t, "aa:bb:cc:dd:ee:05", res.Interface.MAC)
}

func TestResolve_AmbiguousNoHint_Unresolved(t *testing.T) {
	r := NewResolver(vlanColocatedCandidates())

	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{})

	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)
	assert.Nil(t, res.Interface, "must NOT silently guess one of the candidates")
	assert.Equal(t, 3, res.CandidateCount)
}

// Rule (a): caller wants the physical port.
func TestResolve_PreferPhysical(t *testing.T) {
	r := NewResolver(vlanColocatedCandidates())

	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{
		ExpectedType: IfTypeEthernetCsmacd,
	})

	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
	assert.Equal(t, RulePreferPhysical, res.TiebreakerRule)
	require.NotNil(t, res.Interface)
	assert.Equal(t, int32(1), res.Interface.Index, "must pick the ethernetCsmacd candidate")
}

// Rule (b): caller has a VLAN ID, wants a sub-interface, but we have
// two sub-interfaces in the candidate set. Rule (b) requires a UNIQUE
// virtual candidate — with two, it falls through to rule (c)/(d).
func TestResolve_PreferVirtual_ButAmbiguousAmongVirtuals(t *testing.T) {
	r := NewResolver(vlanColocatedCandidates())

	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{VLANID: 100})

	// Two l2vlan candidates — rule (b) cannot pick a unique one, and
	// no NameHint was given, so we end up unresolved.
	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)
	assert.Equal(t, 3, res.CandidateCount)
}

// Rule (b) + (c): VLAN ID and a NameHint together pick the right
// sub-interface.
func TestResolve_PreferVirtual_PlusNameHint_Picks(t *testing.T) {
	r := NewResolver(vlanColocatedCandidates())

	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{
		VLANID:   200,
		NameHint: "Gi0/1.200",
	})

	// Rule (b) cannot pick uniquely among two l2vlans, but rule (c)
	// (longest common prefix) does pick Gi0/1.200.
	require.NotNil(t, res.Interface)
	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
	assert.Equal(t, RuleLongestCommonPrefix, res.TiebreakerRule)
	assert.Equal(t, int32(102), res.Interface.Index)
}

// Rule (b) DOES pick uniquely when only one virtual is in the set.
func TestResolve_PreferVirtual_UniqueVirtual(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Gi0/1", MAC: mac, Type: IfTypeEthernetCsmacd},
		{Index: 101, Name: "Gi0/1.100", MAC: mac, Type: IfTypeL2Vlan},
	})

	res := r.Resolve(mac, ContextHint{VLANID: 100})

	require.NotNil(t, res.Interface)
	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
	assert.Equal(t, RulePreferVirtual, res.TiebreakerRule)
	assert.Equal(t, int32(101), res.Interface.Index)
}

// Rule (c) only: NameHint with no type signal.
func TestResolve_LongestCommonPrefix_Picks(t *testing.T) {
	r := NewResolver(vlanColocatedCandidates())

	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{NameHint: "Gi0/1.100"})

	require.NotNil(t, res.Interface)
	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
	assert.Equal(t, RuleLongestCommonPrefix, res.TiebreakerRule)
	assert.Equal(t, int32(101), res.Interface.Index)
}

// Rule (c) refuses to pick on a zero-length common prefix — that would
// be the same as "first candidate wins", which is exactly the silent
// guess the PRD forbids.
func TestResolve_LongestCommonPrefix_RefusesZeroLenPrefix(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Gi0/1", MAC: mac, Type: IfTypeL2Vlan},
		{Index: 2, Name: "Gi0/2", MAC: mac, Type: IfTypeL2Vlan},
	})

	res := r.Resolve(mac, ContextHint{NameHint: "WhollyDifferentName"})

	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)
	assert.Nil(t, res.Interface)
}

// Rule (c) refuses to pick when two candidates tie on prefix length.
func TestResolve_LongestCommonPrefix_RefusesTie(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Et0/1", MAC: mac, Type: IfTypeL2Vlan},
		{Index: 2, Name: "Et0/2", MAC: mac, Type: IfTypeL2Vlan},
	})

	res := r.Resolve(mac, ContextHint{NameHint: "Et0/3"}) // ties at "Et0/" for both

	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)
}

// Rule generalisation: ExpectedType that is neither physical nor
// virtual (e.g. LAG) should still pick uniquely when one candidate
// matches exactly.
func TestResolve_MatchExpectedType_LAG(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Gi0/1", MAC: mac, Type: IfTypeEthernetCsmacd},
		{Index: 99, Name: "Po1", MAC: mac, Type: IfTypeIeee8023adLag},
	})

	res := r.Resolve(mac, ContextHint{ExpectedType: IfTypeIeee8023adLag})

	require.NotNil(t, res.Interface)
	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
	assert.Equal(t, RuleMatchExpectedType, res.TiebreakerRule)
	assert.Equal(t, int32(99), res.Interface.Index)
}

// Defensive: when every candidate has ifType=0 (unknown), the
// type-based rules silently no-op and we fall through.
func TestResolve_UnknownIfType_FallsThrough(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Gi0/1", MAC: mac, Type: 0},
		{Index: 2, Name: "Gi0/2", MAC: mac, Type: 0},
	})

	// With no NameHint and no useful Type, we must NOT pick.
	res := r.Resolve(mac, ContextHint{ExpectedType: IfTypeEthernetCsmacd})
	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)

	// With a NameHint that disambiguates, rule (c) picks.
	res = r.Resolve(mac, ContextHint{NameHint: "Gi0/2"})
	require.NotNil(t, res.Interface)
	assert.Equal(t, int32(2), res.Interface.Index)
}

func TestAmbiguousMACs(t *testing.T) {
	r := NewResolver([]Candidate{
		{Index: 1, Name: "Gi0/1", MAC: "aa:bb:cc:dd:ee:01", Type: IfTypeEthernetCsmacd},
		{Index: 2, Name: "Gi0/2", MAC: "aa:bb:cc:dd:ee:02", Type: IfTypeEthernetCsmacd},
		{Index: 101, Name: "Gi0/1.100", MAC: "aa:bb:cc:dd:ee:01", Type: IfTypeL2Vlan},
	})

	got := r.AmbiguousMACs()
	sort.Strings(got)
	assert.Equal(t, []string{"aa:bb:cc:dd:ee:01"}, got)
}

func TestIfType_Predicates(t *testing.T) {
	cases := []struct {
		t        IfType
		physical bool
		virtual  bool
		vendor   bool
	}{
		{IfTypeOther, false, false, true},
		{IfTypeEthernetCsmacd, true, false, false},
		{IfType(62), true, false, false},
		{IfType(69), true, false, false},
		{IfType(117), true, false, false},
		{IfTypeL2Vlan, false, true, false},
		{IfTypePropVirtual, false, true, true},
		{IfTypeL3Ipvlan, false, true, false},
		{IfTypeBridge, false, true, false},
		{IfTypeIeee8023adLag, false, false, false}, // LAG: not virtual for V1 (see types.go)
		{IfType(999), false, false, false},
	}
	for _, c := range cases {
		c := c
		t.Run("", func(t *testing.T) {
			assert.Equal(t, c.physical, c.t.IsPhysical())
			assert.Equal(t, c.virtual, c.t.IsVirtual())
			assert.Equal(t, c.vendor, c.t.IsAmbiguousVendorValue())
		})
	}
}

func TestCommonPrefixLen(t *testing.T) {
	assert.Equal(t, 0, commonPrefixLen("", "x"))
	assert.Equal(t, 0, commonPrefixLen("x", ""))
	assert.Equal(t, 0, commonPrefixLen("ab", "cd"))
	assert.Equal(t, 3, commonPrefixLen("Gi0/1.100", "Gi0"))
	assert.Equal(t, 8, commonPrefixLen("Gi0/1.10", "Gi0/1.100"))
	assert.Equal(t, 4, commonPrefixLen("Et0/1", "Et0/2"))
}
