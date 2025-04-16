// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

func newMap(name string) *manager.Map { return &manager.Map{Name: name} }

func newProbe(name string) *manager.Probe {
	return &manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}}
}

func newTailCall(name string) manager.TailCallRoute {
	return manager.TailCallRoute{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}}
}

func newEmptyEBPFProgram() *ebpfProgram {
	return &ebpfProgram{Manager: &ebpf.Manager{Manager: &manager.Manager{}}}
}

// Common Assertions
func assertContains(t *testing.T, e *ebpfProgram, maps, probes, calls int) {
	require.Len(t, e.Maps, maps)
	require.Len(t, e.Probes, probes)
	require.Len(t, e.tailCallRouter, calls)
}

func TestConfigureManagerWithSupportedProtocols_Sanity(t *testing.T) {
	e := newEmptyEBPFProgram()

	protocolSpecs := []*protocols.ProtocolSpec{
		{
			Maps: []*manager.Map{
				newMap("map1"),
			},
			Probes: []*manager.Probe{
				newProbe("probe1"),
			},
			TailCalls: []manager.TailCallRoute{
				newTailCall("tailcall1"),
			},
		},
	}

	cleanup := e.configureManagerWithSupportedProtocols(protocolSpecs)
	assertContains(t, e, 1, 1, 1)
	cleanup()
	assertContains(t, e, 0, 0, 0)
}

func TestConfigureManagerWithSupportedProtocols_NoDuplicates(t *testing.T) {
	e := newEmptyEBPFProgram()

	protocolSpecs := []*protocols.ProtocolSpec{
		{
			Maps: []*manager.Map{
				newMap("map1"),
			},
			Probes: []*manager.Probe{
				newProbe("probe1"),
			},
			TailCalls: []manager.TailCallRoute{
				newTailCall("tailcall1"),
			},
		},
		{
			Maps: []*manager.Map{
				newMap("map1"), // Duplicate
			},
			Probes: []*manager.Probe{
				newProbe("probe1"), // Duplicate
			},
			TailCalls: []manager.TailCallRoute{
				newTailCall("tailcall1"), // Duplicate
			},
		},
	}

	cleanup := e.configureManagerWithSupportedProtocols(protocolSpecs)
	assertContains(t, e, 1, 1, 1)
	cleanup()
	assertContains(t, e, 0, 0, 0)
}

func TestConfigureManagerWithSupportedProtocols_CleanupOnlyRemovesAdded(t *testing.T) {
	e := &ebpfProgram{
		Manager: &ebpf.Manager{
			Manager: &manager.Manager{
				Maps: []*manager.Map{
					newMap("existingMap"),
				},
				Probes: []*manager.Probe{
					newProbe("existingProbe"),
				},
			},
		},
		tailCallRouter: []manager.TailCallRoute{
			newTailCall("existingTailCall"),
		},
	}

	protocolSpecs := []*protocols.ProtocolSpec{
		{
			Maps: []*manager.Map{
				newMap("newMap"),
			},
			Probes: []*manager.Probe{
				newProbe("newProbe"),
			},
			TailCalls: []manager.TailCallRoute{
				newTailCall("newTailCall"),
			},
		},
	}

	cleanup := e.configureManagerWithSupportedProtocols(protocolSpecs)
	assertContains(t, e, 2, 2, 2)

	cleanup()
	assertContains(t, e, 1, 1, 1)
	assert.Equal(t, "existingMap", e.Maps[0].Name)
	assert.Equal(t, "existingProbe", e.Probes[0].EBPFFuncName)
	assert.Equal(t, "existingTailCall", e.tailCallRouter[0].ProbeIdentificationPair.EBPFFuncName)
}
