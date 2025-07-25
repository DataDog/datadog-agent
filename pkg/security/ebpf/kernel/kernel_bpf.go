// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

// Package kernel holds kernel related files
package kernel

import (
	"runtime"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"
)

// HaveMmapableMaps returns whether the kernel supports mmapable maps.
func (k *Version) HaveMmapableMaps() bool {
	return features.HaveMapFlag(features.BPF_F_MMAPABLE) == nil
}

// HaveRingBuffers returns whether the kernel supports ring buffer.
// https://github.com/torvalds/linux/commit/457f44363a8894135c85b7a9afd2bd8196db24ab
func (k *Version) HaveRingBuffers() bool {
	return features.HaveMapType(ebpf.RingBuf) == nil
}

// HasCgroupSysctlSupportWithRingbuf returns true if cgroup/sysctl programs are available with access to ringbuffer
func (k *Version) HasCgroupSysctlSupportWithRingbuf() bool {
	if features.HaveProgramType(ebpf.CGroupSysctl) != nil {
		return false
	}

	// features.HaveProgramHelper doesn't implement feature testing for cgroup/sysctl yet, keep the kernel version
	// fall back for now.
	if features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnRingbufOutput) == nil &&
		features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnGetSmpProcessorId) == nil &&
		features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnKtimeGetNs) == nil {
		return true
	}

	return k.Code >= Kernel5_8
}

// HasTracingHelpersInCgroupSysctlPrograms returns true if basic tracing helpers are available in cgroup/sysctl programs
// Namely, the helpers we care about are: bpf_probe_read_*, bpf_get_current_pid_tgid, bpf_get_current_task, etc
func (k *Version) HasTracingHelpersInCgroupSysctlPrograms() bool {
	if !k.HasCgroupSysctlSupportWithRingbuf() {
		return false
	}

	// features.HaveProgramHelper doesn't implement feature testing for cgroup/sysctl yet, keep the kernel version
	// fall back for now.
	if features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnGetCurrentTask) == nil &&
		features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnGetCurrentComm) == nil &&
		features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnProbeReadKernel) == nil &&
		features.HaveProgramHelper(ebpf.CGroupSysctl, asm.FnGetCurrentPidTgid) == nil {
		return true
	}

	return k.Code >= Kernel6_1
}

// HasSKStorage returns true if the kernel supports SK_STORAGE maps
// See https://github.com/torvalds/linux/commit/6ac99e8f23d4b10258406ca0dd7bffca5f31da9d
func (k *Version) HasSKStorage() bool {
	if features.HaveMapType(ebpf.SkStorage) == nil {
		return true
	}

	return k.Code != 0 && k.Code >= Kernel5_2
}

// HasSKStorageInTracingPrograms returns true if the kernel supports SK_STORAGE maps in tracing programs
// See https://github.com/torvalds/linux/commit/8e4597c627fb48f361e2a5b012202cb1b6cbcd5e
func (k *Version) HasSKStorageInTracingPrograms() bool {
	if !k.HasSKStorage() {
		return false
	}

	if !k.HaveFentrySupport() {
		return false
	}

	// features.HaveProgramHelper doesn't implement feature testing for ebpf.Tracing yet, keep the kernel version
	// fall back for now.
	if features.HaveProgramHelper(ebpf.Tracing, asm.FnSkStorageGet) == nil {
		return true
	}
	return k.Code != 0 && k.Code >= Kernel5_11
}

// HasBPFForEachMapElemHelper returns true if the kernel support the bpf_for_each_map_elem helper
// See https://github.com/torvalds/linux/commit/69c087ba6225b574afb6e505b72cb75242a3d844
func (k *Version) HasBPFForEachMapElemHelper() bool {
	// because of https://lore.kernel.org/bpf/20211231151018.3781550-1-houtao1@huawei.com/
	// we need a kernel 5.17 or higher on arm64 to use the bpf_for_each_map_elem helper
	if runtime.GOARCH == "arm64" && k.Code < Kernel5_17 {
		return false
	}

	if features.HaveProgramHelper(ebpf.PerfEvent, asm.FnForEachMapElem) == nil {
		return true
	}
	return k.Code != 0 && k.Code >= Kernel5_13
}

func (k *Version) commonFentryCheck(funcName string) bool {
	if features.HaveProgramType(ebpf.Tracing) != nil {
		return false
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceFEntry,
		AttachTo:   funcName,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
	}
	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return false
	}
	defer prog.Close()

	link, err := link.AttachTracing(link.TracingOptions{
		Program: prog,
	})
	if err != nil {
		return false
	}
	defer link.Close()

	return true
}

// HaveFentrySupport returns whether the kernel supports fentry probes
func (k *Version) HaveFentrySupport() bool {
	return k.commonFentryCheck("vfs_open")
}

// HaveFentrySupportWithStructArgs returns whether the kernel supports fentry probes with struct arguments
func (k *Version) HaveFentrySupportWithStructArgs() bool {
	return k.commonFentryCheck("audit_set_loginuid")
}

// HaveFentryNoDuplicatedWeakSymbols returns whether the kernel supports fentry probes with struct arguments
func (k *Version) HaveFentryNoDuplicatedWeakSymbols() bool {
	var symbol string
	switch runtime.GOARCH {
	case "amd64":
		symbol = "__ia32_sys_setregid16"
	default:
		return true
	}

	return k.commonFentryCheck(symbol)
}

// SupportCORE returns is CORE is supported
func (k *Version) SupportCORE() bool {
	_, err := btf.LoadKernelSpec()
	return err == nil
}

// HasBpfGetCurrentPidTgidForSchedCLS returns true if the kernel supports bpf_get_current_pid_tgid for Sched CLS program type
// https://github.com/torvalds/linux/commit/eb166e522c77699fc19bfa705652327a1e51a117
func (k *Version) HasBpfGetCurrentPidTgidForSchedCLS() bool {
	return features.HaveProgramHelper(ebpf.SchedCLS, asm.FnGetCurrentPidTgid) == nil
}
