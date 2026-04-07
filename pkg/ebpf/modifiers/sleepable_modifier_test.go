// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package modifiers

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var allProgs = []string{
	"test_modifier_x64",
	"test_replaced_x64",
	"test_modifier_arm64",
	"test_replaced_arm64",
	"test_womodifier_x64",
	"test_womodifier_arm64",
	"test_telemetry_x64",
	"test_telemetry_arm64",
	"test_tracepoint",
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

func setTelemetryMapEditors(opts *manager.Options) {
	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	opts.MapSpecEditors[telemetry.MapErrTelemetryMapName] = manager.MapSpecEditor{
		MaxEntries: 1,
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[telemetry.HelperErrTelemetryMapName] = manager.MapSpecEditor{
		MaxEntries: 1,
		EditorFlag: manager.EditMaxEntries,
	}
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
		setTelemetryMapEditors(&opts)
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
		setTelemetryMapEditors(&opts)
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

	const kptrRestrictPath = "/proc/sys/kernel/kptr_restrict"
	oldVal, err := os.ReadFile(kptrRestrictPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(kptrRestrictPath, []byte("0"), 0644))
	t.Cleanup(func() {
		os.WriteFile(kptrRestrictPath, oldVal, 0644)
	})

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

	err = ddebpf.LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		setTelemetryMapEditors(&opts)
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

func TestSleepableModifierRejectsTracepoint(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	probeName := "test_tracepoint"
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
		setTelemetryMapEditors(&opts)
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
		require.Error(t, err, "sleepable modifier should reject tracepoint programs")

		return nil
	})
	require.NoError(t, err)
}

func TestSleepableModifierTelemetryRemapping(t *testing.T) {
	skipTestIfSleepableEBPFProgramsNotSupported(t)

	collector := telemetry.NewEBPFErrorsCollector()
	require.NotNil(t, collector, "telemetry not supported on this kernel")

	probeName := "test_telemetry" + archSuffix()
	excluded := excludeAllExcept(probeName)
	moduleName := names.NewModuleName("sleepable_test")

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

	sleepableModifier := SleepableProgramModifier{
		ProbeIDs: []manager.ProbeIdentificationPair{
			{EBPFFuncName: probeName},
		},
	}
	telemetryModifier := telemetry.ErrorsTelemetryModifier{}

	err := ddebpf.LoadCOREAsset("sleepable.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		setTelemetryMapEditors(&opts)
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

		err = telemetryModifier.BeforeInit(mgr, moduleName, &opts)
		require.NoError(t, err)

		err = sleepableModifier.BeforeInit(mgr, moduleName, &opts)
		require.NoError(t, err)

		err = mgr.InitWithOptions(nil, opts)
		require.NoError(t, err)

		err = telemetryModifier.AfterInit(mgr, moduleName, &opts)
		require.NoError(t, err)

		err = mgr.Start()
		require.NoError(t, err)

		return nil
	})
	require.NoError(t, err)

	// Trigger the fexit probe via openat(2) syscall.
	// The BPF program reads from 0xdeadbeef via bpf_probe_read_user_with_telemetry,
	// which fails with EFAULT. After the sleepable modifier replaces it with
	// bpf_copy_from_user, the telemetry should report the error under bpf_copy_from_user.
	path, _ := syscall.BytePtrFromString("/dev/null")
	dirfd := unix.AT_FDCWD
	syscall.Syscall6(syscall.SYS_OPENAT, uintptr(dirfd), uintptr(unsafe.Pointer(path)), syscall.O_RDONLY, 0, 0, 0)

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	copyFromUserFound := false
	for _, promMetric := range metrics {
		dtoMetric := dto.Metric{}
		require.NoError(t, promMetric.Write(&dtoMetric))

		var helperLabel, errorLabel string
		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "helper":
				helperLabel = label.GetValue()
			case "error":
				errorLabel = label.GetValue()
			}
		}

		if helperLabel == "" {
			continue
		}

		assert.NotEqual(t, "bpf_probe_read_user", helperLabel,
			"telemetry should not report errors under bpf_probe_read_user after remapping")

		if helperLabel == "bpf_copy_from_user" && errorLabel == "EFAULT" {
			copyFromUserFound = true
		}
	}

	assert.True(t, copyFromUserFound,
		"expected telemetry to report bpf_copy_from_user EFAULT errors after remapping")
}
