// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package modifiers

import (
	"runtime"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var allProgs = []string{
	"test_modifier_x64",
	"test_replaced_x64",
	"test_modifier_arm64",
	"test_replaced_arm64",
	"test_womodifier_x64",
	"test_womodifier_arm64",
}

func archSuffix() string {
	if runtime.GOARCH == "arm64" {
		return "_arm64"
	}
	return "_x64"
}

func excludeAllExcept(keep ...string) []string {
	var excluded []string
	for _, p := range allProgs {
		found := false
		for _, k := range keep {
			if p == k {
				found = true
				break
			}
		}
		if !found {
			excluded = append(excluded, p)
		}
	}
	return excluded
}

func skipTestIfSleepableEBPFProgramsNotSupported(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	if kv < kernel.VersionCode(5, 10, 0) {
		t.Skip("Sleepable EBPF programs not supported")
	}
}

func TestSleepableProgramWithModifier(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	probeName := "test_modifier" + archSuffix()
	excluded := excludeAllExcept(probeName)

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		},
	}

	t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
	modifier := SleepableProgramModifier{
		ProbeIDs: []manager.ProbeIdentificationPair{
			{EBPFFuncName: probeName},
		},
	}
	mname := names.NewModuleName("ebpf")
	err := ddebpf.LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ExcludedFunctions = excluded
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		}

		err := mgr.LoadELF(buf)
		require.NoError(t, err)

		err = modifier.BeforeInit(mgr, mname, &opts)
		require.NoError(t, err)
		err = mgr.InitWithOptions(nil, opts)
		require.NoError(t, err)

		err = mgr.Start()

		require.NoError(t, err)

		return nil
	})

	require.NoError(t, err)
}

func TestSleepableProgramWithoutModifier(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	probeName := "test_womodifier" + archSuffix()
	excluded := excludeAllExcept(probeName)

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		},
	}

	t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
	ddebpf.LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ExcludedFunctions = excluded
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		}

		err := mgr.LoadELF(buf)
		require.NoError(t, err)

		err = mgr.InitWithOptions(nil, opts)
		require.Error(t, err)

		return nil
	})
}

func TestSleepableModifierReplacesProbeReadUser(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	probeName := "test_replaced" + archSuffix()
	excluded := excludeAllExcept(probeName)

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		},
	}

	t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })

	modifier := SleepableProgramModifier{
		ProbeIDs: []manager.ProbeIdentificationPair{
			{EBPFFuncName: probeName},
		},
	}
	mname := names.NewModuleName("ebpf")

	err := ddebpf.LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ExcludedFunctions = excluded
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probeName,
				},
			},
		}

		err := mgr.LoadELF(buf)
		require.NoError(t, err)

		err = modifier.BeforeInit(mgr, mname, &opts)
		require.NoError(t, err)

		err = mgr.InitWithOptions(nil, opts)
		require.NoError(t, err)

		err = mgr.Start()
		require.NoError(t, err)

		return nil
	})
	require.NoError(t, err)

	progs, found, err := mgr.GetProgram(manager.ProbeIdentificationPair{EBPFFuncName: probeName})
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, progs)

	info, err := progs[0].Info()
	require.NoError(t, err)

	insns, err := info.Instructions()
	require.NoError(t, err)

	iter := insns.Iterate()
	for iter.Next() {
		ins := iter.Ins
		if !ins.IsBuiltinCall() {
			continue
		}
		assert.NotEqual(t, int64(asm.FnProbeReadUser), ins.Constant,
			"found bpf_probe_read_user call that should have been replaced with bpf_copy_from_user")
	}
}
