// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"slices"
)

// HardeningObservations is the whole-tree aggregate of the runtime behavior the workload
// hardening controls reason about. It unions over every process node in the tree, which is
// the scope the dump header describes; per-image-tag rollups are a separate concern handled
// at profile encoding time.
type HardeningObservations struct {
	// UIDs is the sorted union of every real, effective and filesystem UID observed on a
	// process node. The three are unioned rather than reported separately because the
	// question every hardening control asks is whether root was involved at all: a process
	// that execs a setuid-root binary reports uid 1000 with euid 0, so reading the real uid
	// alone would classify it as never-root.
	UIDs []uint32

	// CapabilitiesMask has bit i set when CAP_i was checked and held by some process. A
	// capability that was checked and denied is deliberately absent: the control reasons
	// about what the workload needs, not what it asked for.
	CapabilitiesMask uint64

	// CapabilitiesGrantedMask is the union of Credentials.CapEffective across processes,
	// i.e. what the runtime granted as opposed to what was used. No manifest field states
	// this, which is what makes the delta against CapabilitiesMask worth shipping.
	CapabilitiesGrantedMask uint64

	// RuntimeProcessNodes counts the process nodes added at runtime. Nodes from the procfs
	// snapshot only describe what happened to be running when the dump started, so a tree
	// holding none of these supports no claim about what the workload actually does.
	RuntimeProcessNodes int
}

// capabilityMaskBits bounds the shift used to build the capability masks. CAP_LAST_CAP is
// well below this today, but the masks are uint64 on the wire and a wider kernel constant
// must not silently wrap into an unrelated bit.
const capabilityMaskBits = 64

// HardeningObservations walks the tree once and aggregates the observations the hardening
// posture summary in the dump header is built from.
func (at *ActivityTree) HardeningObservations() HardeningObservations {
	var observations HardeningObservations
	uids := make(map[uint32]struct{})

	at.visit(func(processNode *ProcessNode) {
		if processNode.GenerationType == Runtime {
			observations.RuntimeProcessNodes++
		}

		credentials := &processNode.Process.Credentials
		uids[credentials.UID] = struct{}{}
		uids[credentials.EUID] = struct{}{}
		uids[credentials.FSUID] = struct{}{}
		observations.CapabilitiesGrantedMask |= credentials.CapEffective

		for _, capabilityNode := range processNode.Capabilities {
			if capabilityNode.Capable && capabilityNode.Capability < capabilityMaskBits {
				observations.CapabilitiesMask |= 1 << capabilityNode.Capability
			}
		}
	})

	if len(uids) > 0 {
		observations.UIDs = make([]uint32, 0, len(uids))
		for uid := range uids {
			observations.UIDs = append(observations.UIDs, uid)
		}
		slices.Sort(observations.UIDs)
	}

	return observations
}
