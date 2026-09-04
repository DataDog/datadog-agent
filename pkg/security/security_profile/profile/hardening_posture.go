// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

const (
	// postureVersion versions the shape of the hardening posture block, independently of the
	// Agent build (@agent_version already carries that). Bump it on breaking changes only: a
	// field removed, renamed, or the same field coming to mean something different. Adding a
	// field is not breaking.
	postureVersion = 1

	// maxPostureUIDs bounds the uid list. The dump header is indexed on every dump from every
	// container, so its width is a per-dump cost rather than a per-finding one.
	maxPostureUIDs = 32
)

// HardeningPosture is a bounded summary of a workload's observed behaviour, hoisted into the
// JSON dump header so the backend can evaluate hardening controls without opening the protobuf
// attachment. Same rationale as ActivityDumpHeader.DNSNames.
//
// Everything here has to survive a union across dumps, because one dump is one container over
// one interval and no control can be judged on that alone. Sets and masks merge; counts and
// cardinalities do not, so they are derived backend-side and deliberately absent.
type HardeningPosture struct {
	PostureVersion int              `json:"posture_version"`
	Observed       ObservedBehavior `json:"observed"`
}

// ObservedBehavior is what the workload was seen doing, as opposed to what its manifest
// declares. Fields the backend can derive from these are not repeated here: runs_as_root is
// exactly 0 ∈ uids, and which capabilities count as privilege-bearing is a judgement that
// belongs with the rule, not the Agent.
type ObservedBehavior struct {
	// UIDs is the union of the real, effective and filesystem UIDs observed across the
	// profile, sorted ascending and truncated to maxPostureUIDs. Sorting before truncating
	// is load-bearing: it keeps uid 0 in the list, and dropping it would turn a root
	// workload into a never-root one.
	UIDs       []uint32 `json:"uids"`
	UIDsCapped bool     `json:"uids_capped"`

	// CapabilitiesCollected reports whether capability observation was actually running.
	// It is off by default and unsupported below kernel 5.13 (5.17 on arm64), and without
	// this flag an empty CapabilitiesMask reads identically to a workload that used no
	// capabilities — which is the permissive answer for every control that reads it.
	CapabilitiesCollected bool `json:"capabilities_collected"`

	// CapabilitiesMask is what a process was observed holding when checked; bit i is CAP_i.
	CapabilitiesMask uint64 `json:"capabilities_mask"`
	// CapabilitiesGrantedMask is what the runtime granted. No manifest field states this: a
	// container with no securityContext still receives the runtime's default set.
	CapabilitiesGrantedMask uint64 `json:"capabilities_granted_mask"`

	// Arch is the profile's architecture. Capability numbering is stable across the
	// architectures we support, but the summary is only interpretable against a known one.
	Arch string `json:"arch"`
}

// ComputeHardeningPosture builds the posture block for this profile, or returns nil when the
// profile cannot support one. capabilitiesCollected reports whether capability observation was
// enabled for this Agent, which the caller reads from the probe configuration.
func (p *Profile) ComputeHardeningPosture(capabilitiesCollected bool) *HardeningPosture {
	observations := p.ActivityTree.HardeningObservations()

	// A tree with no runtime nodes holds only the procfs snapshot taken when the dump
	// started. It says what was running, never what the workload does, so it cannot back an
	// absence claim and ships nothing rather than an empty-looking one.
	if observations.RuntimeProcessNodes == 0 {
		return nil
	}

	uids := observations.UIDs
	capped := len(uids) > maxPostureUIDs
	if capped {
		uids = uids[:maxPostureUIDs]
	}

	return &HardeningPosture{
		PostureVersion: postureVersion,
		Observed: ObservedBehavior{
			UIDs:                    uids,
			UIDsCapped:              capped,
			CapabilitiesCollected:   capabilitiesCollected,
			CapabilitiesMask:        observations.CapabilitiesMask,
			CapabilitiesGrantedMask: observations.CapabilitiesGrantedMask,
			Arch:                    p.Metadata.Arch,
		},
	}
}
