// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"slices"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

func testConfig(tcxEnabled bool) *config.Config {
	return &config.Config{
		NetworkIngressEnabled:      true,
		NetworkRawPacketEnabled:    true,
		NetworkClassifierPriority:  10,
		NetworkClassifierHandle:    2,
		RawNetworkClassifierHandle: 3,
		NetworkTCXEnabled:          tcxEnabled,
	}
}

func TestSetAttachPoint(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		tcr := NewResolver(testConfig(false))
		require.Equal(t, manager.TCAttachModeLegacy, tcr.AttachMode())

		for _, tcProbe := range probes.GetTCProbes(true, true) {
			clone := tcProbe.Copy()
			tcr.setAttachPoint(clone)

			expectedHandle := netlink.MakeHandle(0, 2)
			if slices.Contains(probes.RawPacketTCProgram, tcProbe.EBPFFuncName) {
				expectedHandle = netlink.MakeHandle(0, 3)
			}
			assert.Equal(t, manager.TCAttachModeLegacy, clone.TCAttachMode, tcProbe.EBPFFuncName)
			assert.Equal(t, uint16(10), clone.TCFilterPrio, tcProbe.EBPFFuncName)
			assert.Equal(t, expectedHandle, clone.TCFilterHandle, tcProbe.EBPFFuncName)
		}
	})

	t.Run("tcx", func(t *testing.T) {
		tcr := NewResolver(testConfig(true))
		require.Equal(t, manager.TCAttachModeTCX, tcr.AttachMode())

		for _, tcProbe := range probes.GetTCProbes(true, true) {
			clone := tcProbe.Copy()
			tcr.setAttachPoint(clone)

			assert.Equal(t, manager.TCAttachModeTCX, clone.TCAttachMode, tcProbe.EBPFFuncName)
			assert.Equal(t, manager.TCXAnchorHead, clone.TCXAnchor, tcProbe.EBPFFuncName)
			assert.Zero(t, clone.TCFilterPrio, tcProbe.EBPFFuncName)
			assert.Zero(t, clone.TCFilterHandle, tcProbe.EBPFFuncName)
		}
	})
}

// Head anchoring and cls_bpf both run the classifier attached last first: the raw packet classifiers must be cloned
// after the other classifiers of their direction to run first.
func TestTCProbesOrder(t *testing.T) {
	tcProbes := probes.GetTCProbes(true, true)
	index := func(funcName string) int {
		return slices.IndexFunc(tcProbes, func(p *manager.Probe) bool { return p.EBPFFuncName == funcName })
	}

	for classifier, rawPacket := range map[string]string{
		"classifier_egress":  "classifier_raw_packet_egress",
		"classifier_ingress": "classifier_raw_packet_ingress",
	} {
		require.GreaterOrEqual(t, index(classifier), 0, classifier)
		require.GreaterOrEqual(t, index(rawPacket), 0, rawPacket)
		assert.Less(t, index(classifier), index(rawPacket), "%s must be cloned before %s", classifier, rawPacket)
	}
}

func TestSendTCProgramsStats(t *testing.T) {
	tcr := NewResolver(testConfig(true))
	tcr.programs[ProgramKey{FuncName: "classifier_egress", IfIndex: 1}] = programEntry{attachMode: manager.TCAttachModeTCX}
	tcr.programs[ProgramKey{FuncName: "classifier_ingress", IfIndex: 1}] = programEntry{attachMode: manager.TCAttachModeTCX}
	tcr.programs[ProgramKey{FuncName: "classifier_egress", IfIndex: 2}] = programEntry{attachMode: manager.TCAttachModeLegacy}

	client := statsdclient.NewStatsdClient()
	tcr.SendTCProgramsStats(client)

	assert.Equal(t, map[string]int64{
		"tcx":    2,
		"legacy": 1,
	}, client.GetByPrefix(metrics.MetricTCProgram+":attach_mode:"))
}

func TestDetachInactiveTCXProbesWithoutLink(t *testing.T) {
	tcr := NewResolver(testConfig(true))
	moved := ProgramKey{FuncName: "classifier_egress", IfIndex: 1, NetNS: 10}
	movedTo := ProgramKey{FuncName: "classifier_egress", IfIndex: 1, NetNS: 20}
	other := ProgramKey{FuncName: "classifier_egress", IfIndex: 2, NetNS: 10}
	// probes that were never attached are inactive and have no link; the zero manager can't detach them
	for _, key := range []ProgramKey{moved, movedTo, other} {
		tcr.programs[key] = programEntry{probe: &manager.Probe{TCAttachMode: manager.TCAttachModeTCX}, attachMode: manager.TCAttachModeTCX}
	}
	m := &manager.Manager{}

	assert.Zero(t, tcr.DetachMovedDevice(1, 20, m), "stale links")
	assert.NotContains(t, tcr.programs, moved)
	assert.Len(t, tcr.programs, 2)

	_, staleLinks := tcr.FlushInactiveProbes(m, func(string) bool { return false })
	assert.Zero(t, staleLinks)
	assert.Empty(t, tcr.programs)

	client := statsdclient.NewStatsdClient()
	tcr.SendTCProgramsStats(client)
	assert.Zero(t, client.Get(metrics.MetricTCStaleLinks))
}

func TestDumpNetworkNamespacePrograms(t *testing.T) {
	tcr := NewResolver(testConfig(true))
	add := func(funcName string, direction manager.TrafficType, ifIndex, netns, programID uint32) {
		tcr.programs[ProgramKey{FuncName: funcName, IfIndex: ifIndex, NetNS: netns, NetworkDirection: direction}] = programEntry{
			programID:  programID,
			attachMode: manager.TCAttachModeTCX,
		}
	}
	add("classifier_egress", manager.Egress, 1, 10, 100)
	add("classifier_raw_packet_egress", manager.Egress, 1, 10, 101)
	add("classifier_ingress", manager.Ingress, 1, 10, 102)
	add("classifier_egress", manager.Egress, 2, 10, 103)
	add("classifier_egress", manager.Egress, 1, 20, 104)

	// without a handle on the namespace, the positions can't be queried
	dumps := tcr.DumpNetworkNamespacePrograms(10, nil, nil)

	assert.Equal(t, map[uint32][]ProgramDump{
		1: {
			{FuncName: "classifier_egress", Direction: "egress", AttachMode: "tcx", ProgramID: 100},
			{FuncName: "classifier_raw_packet_egress", Direction: "egress", AttachMode: "tcx", ProgramID: 101},
			{FuncName: "classifier_ingress", Direction: "ingress", AttachMode: "tcx", ProgramID: 102},
		},
		2: {
			{FuncName: "classifier_egress", Direction: "egress", AttachMode: "tcx", ProgramID: 103},
		},
	}, dumps)
}
