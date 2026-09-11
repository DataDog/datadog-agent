// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func newMap(name string) *manager.Map { return &manager.Map{Name: name} }

func newProbe(name string) *manager.Probe {
	return &manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}}
}

func newTailCall(name string) manager.TailCallRoute {
	return manager.TailCallRoute{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}}
}

func newEmptyEBPFProgram() *ebpfProgram {
	return &ebpfProgram{Manager: &ddebpf.Manager{Manager: &manager.Manager{}}}
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
		Manager: &ddebpf.Manager{
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

// optionsCapture is a ddebpf.Modifier that records the manager options handed
// to it, so a test can inspect what a load attempt built.
type optionsCapture struct {
	opts     manager.Options
	captured bool
}

func (c *optionsCapture) String() string { return "optionsCapture" }

// BeforeInit clones the options rather than keeping the pointer: the manager
// and the modifiers that run after this one keep writing to them.
func (c *optionsCapture) BeforeInit(_ *manager.Manager, _ names.ModuleName, o *manager.Options) error {
	c.opts = *o
	c.opts.MapSpecEditors = maps.Clone(o.MapSpecEditors)
	c.opts.ConstantEditors = slices.Clone(o.ConstantEditors)
	c.opts.ExcludedFunctions = slices.Clone(o.ExcludedFunctions)
	c.captured = true
	return nil
}

// failProgramLoad is a manager.InstructionPatcherFunc that prepends `r0 = 1;
// exit` to every program so the verifier rejects it.
func failProgramLoad(m *manager.Manager) error {
	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	for _, p := range progs {
		if len(p.Instructions) == 0 {
			continue
		}
		// The first instruction carries the program's symbol, so the injected
		// prologue has to take it over.
		sym := p.Instructions[0].Symbol()
		p.Instructions[0] = p.Instructions[0].WithSymbol("")
		p.Instructions = append(asm.Instructions{
			asm.Mov.Imm(asm.R0, 1).WithSymbol(sym),
			asm.Return(),
		}, p.Instructions...)
	}
	return nil
}

// TestInitOptionsMatchAcrossBuildModes loads the USM programs with CO-RE and
// then with runtime compilation on a single ebpfProgram, and requires both
// attempts to have built the same manager options.
func TestInitOptionsMatchAcrossBuildModes(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < usmconfig.MinimumKernelVersion {
		t.Skip("USM is not supported on this kernel version")
	}

	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTP2Monitoring = true

	e, err := newEBPFProgram(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Stop(manager.CleanAll) })

	// Prepended so it observes the options as init built them, before the
	// other modifiers add their own edits. The errors telemetry modifier sizes
	// its maps from the loaded ELF, which legitimately differs between build
	// modes and would otherwise swamp the comparison.
	capture := &optionsCapture{}
	e.EnabledModifiers = append([]ddebpf.Modifier{capture}, e.EnabledModifiers...)

	// Force the CO-RE attempt to fail, so the sequence matches production: the
	// fallback only ever runs after a failed load.
	e.InstructionPatchers = append(e.InstructionPatchers, failProgramLoad)

	e.buildMode = buildmode.CORE
	coreErr := e.initCORE()
	require.True(t, capture.captured, "CO-RE attempt never built options")
	require.Error(t, coreErr, "CO-RE load was expected to fail")
	coreOptions := capture.opts

	// Only the CO-RE attempt is sabotaged; the fallback loads for real.
	e.InstructionPatchers = nil
	capture.captured = false
	e.buildMode = buildmode.RuntimeCompiled
	runtimeErr := e.initRuntimeCompiler()
	require.True(t, capture.captured, "runtime compiled attempt never built options: %v", runtimeErr)
	runtimeOptions := capture.opts

	assert.Equal(t, coreOptions.MapSpecEditors, runtimeOptions.MapSpecEditors,
		"both build modes must produce the same map spec editors")
	assert.ElementsMatch(t, coreOptions.ConstantEditors, runtimeOptions.ConstantEditors,
		"both build modes must inject the same constants")
	assert.ElementsMatch(t, coreOptions.ExcludedFunctions, runtimeOptions.ExcludedFunctions,
		"both build modes must exclude the same functions")
}

// failBuildModes sabotages the program load for the given build modes, letting
// the others load for real. Init sets e.buildMode before each attempt, so the
// patcher can tell which one it is running under.
func failBuildModes(e *ebpfProgram, modes ...buildmode.Type) {
	e.InstructionPatchers = append(e.InstructionPatchers, func(m *manager.Manager) error {
		if !slices.Contains(modes, e.buildMode) {
			return nil
		}
		return failProgramLoad(m)
	})
}

// newFallbackTestProgram builds a USM program with every build mode and every
// fallback enabled, so Init walks the full CO-RE -> runtime compiled ->
// prebuilt chain.
func newFallbackTestProgram(t *testing.T) *ebpfProgram {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < usmconfig.MinimumKernelVersion {
		t.Skip("USM is not supported on this kernel version")
	}

	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableCORE = true
	cfg.EnableRuntimeCompiler = true
	cfg.AllowRuntimeCompiledFallback = true
	cfg.AllowPrebuiltFallback = true

	e, err := newEBPFProgram(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Stop(manager.CleanAll) })
	return e
}

// TestRuntimeFallbackWorks fails the CO-RE load and requires Init to fall back
// to runtime compilation and succeed.
func TestRuntimeFallbackWorks(t *testing.T) {
	e := newFallbackTestProgram(t)
	failBuildModes(e, buildmode.CORE)

	require.NoError(t, e.Init(), "runtime compiled fallback failed after a failed CO-RE load")
	assert.Equal(t, buildmode.RuntimeCompiled, e.buildMode,
		"expected Init to settle on the runtime compiled build mode")
}

// TestPrebuiltFallbackWorks fails both the CO-RE and the runtime compiled loads
// and requires Init to fall back to prebuilt and succeed.
func TestPrebuiltFallbackWorks(t *testing.T) {
	e := newFallbackTestProgram(t)
	failBuildModes(e, buildmode.CORE, buildmode.RuntimeCompiled)

	require.NoError(t, e.Init(), "prebuilt fallback failed after failed CO-RE and runtime compiled loads")
	assert.Equal(t, buildmode.Prebuilt, e.buildMode,
		"expected Init to settle on the prebuilt build mode")
}
