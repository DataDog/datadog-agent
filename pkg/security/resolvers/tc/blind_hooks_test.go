// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
)

func TestFindProgramAhead(t *testing.T) {
	const filterPriority = 10
	const ownProgramID = 42
	isOwn := func(programID uint32) bool {
		return programID == ownProgramID
	}
	ownFilter := Filter{Name: "classifier_ingress_security", ProgramID: ownProgramID, Priority: filterPriority, DirectAction: true}

	for _, test := range []struct {
		name     string
		hook     HookPrograms
		expected ProgramAhead
		found    bool
	}{
		{
			name: "no foreign program",
			hook: HookPrograms{Filters: []Filter{ownFilter}},
		},
		{
			name:     "cilium tcx program",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, Name: "cil_from_netdev"}}, Filters: []Filter{ownFilter}},
			expected: ProgramAhead{Name: "cil_from_netdev", Reason: BlindReasonCiliumTCX},
			found:    true,
		},
		{
			name:     "cilium tcx program after cilium deleted every filter",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, Name: "cil_to_netdev"}}},
			expected: ProgramAhead{Name: "cil_to_netdev", Reason: BlindReasonCiliumTCX},
			found:    true,
		},
		{
			name:     "unknown tcx program",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, Name: "foreign_prog"}}, Filters: []Filter{ownFilter}},
			expected: ProgramAhead{Name: "foreign_prog", Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name:     "tcx program with a name that couldn't be read",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7}}},
			expected: ProgramAhead{Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name: "own tcx program",
			hook: HookPrograms{TCX: []TCXProgram{{ID: ownProgramID, Name: "classifier_egr"}}},
		},
		{
			name:     "cilium filter with a lower priority value",
			hook:     HookPrograms{Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, DirectAction: true}}},
			expected: ProgramAhead{Name: "cil_from_netdev-eth0", Reason: BlindReasonCiliumTCFilter},
			found:    true,
		},
		{
			name:     "unknown filter with a lower priority value",
			hook:     HookPrograms{Filters: []Filter{ownFilter, {Name: "foreign_filter", ProgramID: 7, Priority: 1, DirectAction: true}}},
			expected: ProgramAhead{Name: "foreign_filter", Reason: BlindReasonUnknownTCFilter},
			found:    true,
		},
		{
			name: "cilium filter with a higher priority value",
			hook: HookPrograms{Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 20, DirectAction: true}}},
		},
		{
			name: "cilium filter with the same priority value",
			hook: HookPrograms{Filters: []Filter{{Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: filterPriority, DirectAction: true}, ownFilter}},
		},
		{
			name: "filter without direct action",
			hook: HookPrograms{Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1}}},
		},
		{
			name: "filter of another chain",
			hook: HookPrograms{Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, Chain: 3, DirectAction: true}}},
		},
		{
			name: "own filter with a lower priority value",
			hook: HookPrograms{Filters: []Filter{{Name: "classifier_ingress_security", ProgramID: ownProgramID, Priority: 1, DirectAction: true}}},
		},
		{
			name: "cilium filter behind an unknown tcx program",
			hook: HookPrograms{
				TCX:     []TCXProgram{{ID: 8, Name: "foreign_prog"}},
				Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, DirectAction: true}},
			},
			expected: ProgramAhead{Name: "cil_from_netdev-eth0", Reason: BlindReasonCiliumTCFilter},
			found:    true,
		},
		{
			name: "cilium tcx program ahead of a cilium filter",
			hook: HookPrograms{
				TCX:     []TCXProgram{{ID: 8, Name: "cil_from_netdev"}},
				Filters: []Filter{ownFilter, {Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, DirectAction: true}},
			},
			expected: ProgramAhead{Name: "cil_from_netdev", Reason: BlindReasonCiliumTCX},
			found:    true,
		},
		{
			name: "unknown tcx program ahead of an unknown filter",
			hook: HookPrograms{
				TCX:     []TCXProgram{{ID: 8, Name: "foreign_prog"}},
				Filters: []Filter{ownFilter, {Name: "foreign_filter", ProgramID: 7, Priority: 1, DirectAction: true}},
			},
			expected: ProgramAhead{Name: "foreign_prog", Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name: "filters run by priority, not in dump order",
			hook: HookPrograms{Filters: []Filter{
				ownFilter,
				{Name: "foreign_filter_5", ProgramID: 8, Priority: 5, DirectAction: true},
				{Name: "foreign_filter_2", ProgramID: 7, Priority: 2, DirectAction: true},
			}},
			expected: ProgramAhead{Name: "foreign_filter_2", Reason: BlindReasonUnknownTCFilter},
			found:    true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, found := FindProgramAhead(manager.TCAttachModeLegacy, test.hook, filterPriority, isOwn)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.expected, prog)
		})
	}
}

func TestFindProgramAheadTCX(t *testing.T) {
	const filterPriority = 10
	own := map[uint32]bool{41: true, 42: true}
	isOwn := func(programID uint32) bool {
		return own[programID]
	}
	rawPacket := TCXProgram{ID: 41, LinkID: 141}
	classifier := TCXProgram{ID: 42, LinkID: 142}

	for _, test := range []struct {
		name     string
		hook     HookPrograms
		expected ProgramAhead
		found    bool
	}{
		{
			name: "no foreign program",
			hook: HookPrograms{TCX: []TCXProgram{rawPacket, classifier}},
		},
		{
			name: "cilium program at the tail",
			hook: HookPrograms{TCX: []TCXProgram{rawPacket, classifier, {ID: 7, LinkID: 107, Name: "cil_from_netdev"}}},
		},
		{
			name: "cilium filter with a lower priority value",
			hook: HookPrograms{
				TCX:     []TCXProgram{rawPacket, classifier},
				Filters: []Filter{{Name: "cil_from_netdev-eth0", ProgramID: 7, Priority: 1, DirectAction: true}},
			},
		},
		{
			name: "cilium program and no classifier",
			hook: HookPrograms{TCX: []TCXProgram{{ID: 7, LinkID: 107, Name: "cil_from_netdev"}}},
		},
		{
			name:     "cilium program at the head",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, LinkID: 107, Name: "cil_from_netdev"}, rawPacket, classifier}},
			expected: ProgramAhead{Name: "cil_from_netdev", Reason: BlindReasonCiliumTCX},
			found:    true,
		},
		{
			name:     "unknown program at the head",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, LinkID: 107, Name: "foreign_prog"}, rawPacket, classifier}},
			expected: ProgramAhead{Name: "foreign_prog", Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name:     "program attached without a link at the head",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, Name: "foreign_prog"}, rawPacket, classifier}},
			expected: ProgramAhead{Name: "foreign_prog", Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name:     "unknown program between the classifiers",
			hook:     HookPrograms{TCX: []TCXProgram{rawPacket, {ID: 7, LinkID: 107, Name: "foreign_prog"}, classifier}},
			expected: ProgramAhead{Name: "foreign_prog", Reason: BlindReasonUnknownTCX},
			found:    true,
		},
		{
			name: "cilium program behind an unknown one",
			hook: HookPrograms{TCX: []TCXProgram{
				{ID: 8, LinkID: 108, Name: "foreign_prog"},
				{ID: 7, LinkID: 107, Name: "cil_from_netdev"},
				rawPacket,
				classifier,
			}},
			expected: ProgramAhead{Name: "cil_from_netdev", Reason: BlindReasonCiliumTCX},
			found:    true,
		},
		{
			name:     "program with a name that couldn't be read",
			hook:     HookPrograms{TCX: []TCXProgram{{ID: 7, LinkID: 107}, rawPacket, classifier}},
			expected: ProgramAhead{Reason: BlindReasonUnknownTCX},
			found:    true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, found := FindProgramAhead(manager.TCAttachModeTCX, test.hook, filterPriority, isOwn)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.expected, prog)
		})
	}
}

func TestBlindReasonEndsChain(t *testing.T) {
	var endsChain []BlindReason
	for _, reason := range BlindReasons {
		if reason.EndsChain() {
			endsChain = append(endsChain, reason)
		}
	}
	assert.ElementsMatch(t, []BlindReason{BlindReasonCiliumTCX, BlindReasonCiliumTCFilter}, endsChain)
}
