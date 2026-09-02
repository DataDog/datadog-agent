// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func newPostureTestProfile(nodes ...*activity_tree.ProcessNode) *Profile {
	p := New()
	p.ActivityTree.ProcessNodes = nodes
	return p
}

func newPostureTestNode(generationType activity_tree.NodeGenerationType, creds model.Credentials) *activity_tree.ProcessNode {
	return &activity_tree.ProcessNode{
		NodeBase:       activity_tree.NewNodeBase(),
		GenerationType: generationType,
		Process:        activity_tree.ProcessInfo{Credentials: creds},
	}
}

func TestComputeHardeningPosture_NilWhenNoRuntimeNodes(t *testing.T) {
	// A snapshot-only dump describes what was already running when the dump started. It
	// cannot support an "this behavior was never observed" claim, so it ships no posture.
	p := newPostureTestProfile(
		newPostureTestNode(activity_tree.Snapshot, model.Credentials{UID: 1000}),
	)

	assert.Nil(t, p.ComputeHardeningPosture(true))
}

func TestComputeHardeningPosture_UIDsCappedRetainingRoot(t *testing.T) {
	// The header is indexed, so the uid list is bounded. Sorting ascending before the
	// truncation is what keeps uid 0 in: losing it would turn a root workload into a
	// never-root one, which is the one mistake this list must not make.
	var nodes []*activity_tree.ProcessNode
	nodes = append(nodes, newPostureTestNode(activity_tree.Runtime, model.Credentials{UID: 0, EUID: 0, FSUID: 0}))
	for uid := uint32(100); uid <= 140; uid++ {
		nodes = append(nodes, newPostureTestNode(activity_tree.Runtime, model.Credentials{UID: uid, EUID: uid, FSUID: uid}))
	}

	posture := newPostureTestProfile(nodes...).ComputeHardeningPosture(true)

	require.NotNil(t, posture)
	assert.Len(t, posture.Observed.UIDs, maxPostureUIDs)
	assert.Equal(t, uint32(0), posture.Observed.UIDs[0])
	assert.True(t, posture.Observed.UIDsCapped)
}

func TestComputeHardeningPosture_UIDsNotCappedUnderTheLimit(t *testing.T) {
	p := newPostureTestProfile(
		newPostureTestNode(activity_tree.Runtime, model.Credentials{UID: 1000, EUID: 1000, FSUID: 1000}),
	)

	posture := p.ComputeHardeningPosture(true)

	require.NotNil(t, posture)
	assert.Equal(t, []uint32{1000}, posture.Observed.UIDs)
	assert.False(t, posture.Observed.UIDsCapped)
}

func TestComputeHardeningPosture_CapabilitiesCollectedIsRecorded(t *testing.T) {
	// Capability collection is off by default and unsupported on older kernels. Without
	// this flag an empty capabilities_mask is indistinguishable from a workload that used
	// no capabilities, and every hardening control reading the mask would fail open.
	p := newPostureTestProfile(
		newPostureTestNode(activity_tree.Runtime, model.Credentials{UID: 1000, EUID: 1000, FSUID: 1000}),
	)

	posture := p.ComputeHardeningPosture(false)

	require.NotNil(t, posture)
	assert.False(t, posture.Observed.CapabilitiesCollected)
	assert.Zero(t, posture.Observed.CapabilitiesMask)
}

func TestComputeHardeningPosture_CarriesMasksAndArch(t *testing.T) {
	p := newPostureTestProfile(
		newPostureTestNode(activity_tree.Runtime, model.Credentials{UID: 1000, EUID: 1000, FSUID: 1000, CapEffective: 1 << 12}),
	)
	p.Metadata.Arch = "x86_64"

	posture := p.ComputeHardeningPosture(true)

	require.NotNil(t, posture)
	assert.Equal(t, uint64(1<<12), posture.Observed.CapabilitiesGrantedMask)
	assert.Equal(t, "x86_64", posture.Observed.Arch)
	assert.Equal(t, postureVersion, posture.PostureVersion)
}

func TestActivityDumpHeader_OmitsHardeningPostureWhenAbsent(t *testing.T) {
	// The block is absent on the majority of dumps — capability collection off, snapshot-only
	// trees, the feature disabled. Absent has to mean absent on the wire, not an empty object,
	// so the backend can tell "the Agent did not collect this" from "there was nothing to say".
	header := ActivityDumpHeader{DNSNames: utils.NewStringKeys(nil)}

	encoded, err := json.Marshal(header)

	require.NoError(t, err)
	assert.JSONEq(t, `{"dns_names": []}`, string(encoded))
}

func TestActivityDumpHeader_MarshalsHardeningPosture(t *testing.T) {
	header := ActivityDumpHeader{
		DNSNames: utils.NewStringKeys(nil),
		Hardening: &HardeningPosture{
			PostureVersion: postureVersion,
			Observed: ObservedBehavior{
				UIDs:                  []uint32{0, 1000},
				CapabilitiesCollected: true,
				CapabilitiesMask:      1 << 7,
				Arch:                  "x86_64",
			},
		},
	}

	encoded, err := json.Marshal(header)

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"dns_names": [],
		"hardening": {
			"posture_version": 1,
			"observed": {
				"uids": [0, 1000],
				"uids_capped": false,
				"capabilities_collected": true,
				"capabilities_mask": 128,
				"capabilities_granted_mask": 0,
				"arch": "x86_64"
			}
		}
	}`, string(encoded))
}
