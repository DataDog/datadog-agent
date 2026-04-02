// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/require"
)

func skipTestIfSleepableEBPFProgramsNotSupported(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	if kv < kernel.VersionCode(5, 10, 0) {
		t.Skip("Sleepable EBPF programs not supported")
	}
}

func TestSleepableProgramWithModifier(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fexit__x64_sys_open",
				},
			},
		},
	}

	t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
	modifier := SleepableProgramModifier{}
	mname := names.NewModuleName("ebpf")
	err := LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fexit__x64_sys_open",
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

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fexit__x64_sys_open",
				},
			},
		},
	}

	t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
	LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fexit__x64_sys_open",
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
