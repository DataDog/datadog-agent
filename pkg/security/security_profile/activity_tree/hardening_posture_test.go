// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// newPostureTestNode builds a process node carrying the given credentials, so the posture
// tests can state exactly which identity a process ran under.
func newPostureTestNode(path string, generationType NodeGenerationType, creds model.Credentials) *ProcessNode {
	return &ProcessNode{
		NodeBase:       NewNodeBase(),
		GenerationType: generationType,
		Process: ProcessInfo{
			FileEvent:   model.FileEvent{PathnameStr: path},
			Credentials: creds,
		},
	}
}

func TestHardeningObservations_UIDsUnionRealEffectiveAndFilesystem(t *testing.T) {
	// A process that execs a setuid-root binary reports uid 1000 with euid 0. Both values
	// have to land in the union, otherwise the backend reads the workload as never-root.
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}
	tree.ProcessNodes = []*ProcessNode{
		newPostureTestNode("/usr/bin/app", Runtime, model.Credentials{UID: 1000, EUID: 0, FSUID: 1000}),
	}

	observations := tree.HardeningObservations()

	assert.Equal(t, []uint32{0, 1000}, observations.UIDs)
}

func TestHardeningObservations_UIDsAreDeduplicatedAndSorted(t *testing.T) {
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}
	parent := newPostureTestNode("/usr/bin/entrypoint", Runtime, model.Credentials{UID: 0, EUID: 0, FSUID: 0})
	child := newPostureTestNode("/usr/bin/app", Runtime, model.Credentials{UID: 1000, EUID: 1000, FSUID: 1000})
	sibling := newPostureTestNode("/usr/bin/sidecar", Runtime, model.Credentials{UID: 65534, EUID: 1000, FSUID: 65534})
	parent.Children = []*ProcessNode{child, sibling}
	tree.ProcessNodes = []*ProcessNode{parent}

	observations := tree.HardeningObservations()

	assert.Equal(t, []uint32{0, 1000, 65534}, observations.UIDs)
}

func TestHardeningObservations_CapabilitiesMaskOnlyCoversHeldCapabilities(t *testing.T) {
	// CapabilityNode records the check; Capable says whether the process actually held it.
	// A capability that was checked and denied must not appear in the used mask.
	now := time.Now()
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}
	tagID := tree.GetOrInsertImageTag("v1")

	node := newPostureTestNode("/usr/bin/app", Runtime, model.Credentials{})
	node.Capabilities = []*CapabilityNode{
		NewCapabilityNode(7, true, now, tagID, Runtime),   // CAP_SETUID, held
		NewCapabilityNode(12, false, now, tagID, Runtime), // CAP_NET_ADMIN, denied
	}
	tree.ProcessNodes = []*ProcessNode{node}

	observations := tree.HardeningObservations()

	assert.Equal(t, uint64(1<<7), observations.CapabilitiesMask)
}

func TestHardeningObservations_GrantedMaskUnionsCapEffective(t *testing.T) {
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}
	tree.ProcessNodes = []*ProcessNode{
		newPostureTestNode("/usr/bin/a", Runtime, model.Credentials{CapEffective: 1 << 7}),
		newPostureTestNode("/usr/bin/b", Runtime, model.Credentials{CapEffective: 1 << 12}),
	}

	observations := tree.HardeningObservations()

	assert.Equal(t, uint64(1<<7|1<<12), observations.CapabilitiesGrantedMask)
}

func TestHardeningObservations_OnlyRuntimeNodesAreCounted(t *testing.T) {
	// A dump made entirely of procfs snapshot nodes describes what was running when the
	// dump started, not what the workload does. The count is what lets the caller drop it.
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}
	tree.ProcessNodes = []*ProcessNode{
		newPostureTestNode("/usr/bin/snapshotted", Snapshot, model.Credentials{}),
		newPostureTestNode("/usr/bin/observed", Runtime, model.Credentials{}),
		newPostureTestNode("/usr/bin/drifted", ProfileDrift, model.Credentials{}),
	}

	observations := tree.HardeningObservations()

	assert.Equal(t, 1, observations.RuntimeProcessNodes)
}

func TestHardeningObservations_EmptyTree(t *testing.T) {
	tree := &ActivityTree{Stats: NewActivityTreeNodeStats()}

	observations := tree.HardeningObservations()

	assert.Empty(t, observations.UIDs)
	assert.Zero(t, observations.CapabilitiesMask)
	assert.Zero(t, observations.CapabilitiesGrantedMask)
	assert.Zero(t, observations.RuntimeProcessNodes)
}
