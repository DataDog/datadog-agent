// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"cmp"
	"slices"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
)

// BlindReason is the kind of foreign program that runs before the classifiers of a hook
type BlindReason string

const (
	// BlindReasonCiliumTCX is a Cilium TCX program: it never lets the hook go on to the next program
	BlindReasonCiliumTCX BlindReason = "cilium_tcx"
	// BlindReasonCiliumTCFilter is a Cilium filter with a lower priority value: it ends the tc chain
	BlindReasonCiliumTCFilter BlindReason = "cilium_tc_filter"
	// BlindReasonUnknownTCX is a foreign TCX program, which may let the hook go on to the next program
	BlindReasonUnknownTCX BlindReason = "unknown_tcx"
	// BlindReasonUnknownTCFilter is a foreign direct-action filter with a lower priority value, which may
	// let the tc chain go on
	BlindReasonUnknownTCFilter BlindReason = "unknown_tc_filter"
)

// BlindReasons lists every blind reason
var BlindReasons = []BlindReason{
	BlindReasonCiliumTCX,
	BlindReasonCiliumTCFilter,
	BlindReasonUnknownTCX,
	BlindReasonUnknownTCFilter,
}

// EndsChain returns true if the foreign program is known to end the chain before the classifiers run
func (r BlindReason) EndsChain() bool {
	return r == BlindReasonCiliumTCX || r == BlindReasonCiliumTCFilter
}

// ciliumPrefix is the prefix Cilium itself relies on to recognize its programs and filters
const ciliumPrefix = "cil_"

// TCXProgram is a program attached to a TCX hook
type TCXProgram struct {
	ID     uint32
	LinkID uint32
	Name   string
}

// Filter is a bpf filter of a clsact qdisc
type Filter struct {
	Name         string
	ProgramID    uint32
	Priority     uint16
	Chain        uint32
	DirectAction bool
}

// HookPrograms holds the programs attached to a hook
type HookPrograms struct {
	// TCX lists the TCX programs in run order
	TCX     []TCXProgram
	Filters []Filter
}

// ProgramAhead is a foreign program that runs before the classifiers of a hook
type ProgramAhead struct {
	// Name is the name of the program, or of the filter
	Name   string
	Reason BlindReason
}

// FindProgramAhead returns the foreign program that runs before the classifiers of a hook, given how the
// classifiers are attached and the priority of their filters. A program known to end the chain is preferred
// over the unknown programs that run before it, since the classifiers are blind either way.
func FindProgramAhead(mode manager.TCAttachMode, hook HookPrograms, filterPriority uint16, isOwn func(programID uint32) bool) (ProgramAhead, bool) {
	var ahead []ProgramAhead

	switch mode {
	case manager.TCAttachModeLegacy:
		// the legacy tc chain only runs when every TCX program lets the hook go on
		for _, prog := range hook.TCX {
			if !isOwn(prog.ID) {
				ahead = append(ahead, newProgramAhead(prog.Name, BlindReasonCiliumTCX, BlindReasonUnknownTCX))
			}
		}

		filters := slices.Clone(hook.Filters)
		slices.SortStableFunc(filters, func(a, b Filter) int {
			return cmp.Compare(a.Priority, b.Priority)
		})
		for _, filter := range filters {
			// filters of other chains only run when a filter of chain 0 jumps to them
			if filter.DirectAction && filter.Chain == 0 && filter.Priority < filterPriority && !isOwn(filter.ProgramID) {
				ahead = append(ahead, newProgramAhead(filter.Name, BlindReasonCiliumTCFilter, BlindReasonUnknownTCFilter))
			}
		}
	}

	if len(ahead) == 0 {
		return ProgramAhead{}, false
	}
	if i := slices.IndexFunc(ahead, func(prog ProgramAhead) bool { return prog.Reason.EndsChain() }); i >= 0 {
		return ahead[i], true
	}
	return ahead[0], true
}

func newProgramAhead(name string, cilium BlindReason, unknown BlindReason) ProgramAhead {
	if strings.HasPrefix(name, ciliumPrefix) {
		return ProgramAhead{Name: name, Reason: cilium}
	}
	return ProgramAhead{Name: name, Reason: unknown}
}
