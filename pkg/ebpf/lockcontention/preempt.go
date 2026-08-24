// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package lockcontention

import (
	"errors"
	"fmt"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
)

// ErrThisCPUPtrNotPresent tells us that the helper bpf_this_cpu_ptr is not present on the running kernel
var ErrThisCPUPtrNotPresent = errors.New("required helper bpf_this_cpu_ptr not present")

// ErrRequiredVarsMissingInBTF tells us that required variables are missing from the running kernel's BTF
var ErrRequiredVarsMissingInBTF = errors.New("preempt_count in ebpf not supported. Neither __preempt_count nor pcpu_hot is available in kernel BTF")

// PreemptCountConstants checks if getting preempt_count from ebpf programs is supported.
// If so it returns constants we need to override to select how this count is read in ebpf.
func PreemptCountConstants(cache *btf.Cache) (map[string]uint64, error) {
	if err := features.HaveHelperInRawTracepoint(asm.FnThisCpuPtr); err != nil {
		return nil, ErrThisCPUPtrNotPresent
	}

	arch := kernel.Arch()
	switch arch {
	case "x86":
		return preemptCountAMD64(cache)
	case "arm64":
		return preemptCountARM64()
	default:
		return nil, fmt.Errorf("unsupported runtime architecture :%s", arch)
	}
}

func preemptCountAMD64(cache *btf.Cache) (map[string]uint64, error) {
	preemptCountMissing, err := ddebpf.VerifyKernelFuncs("__preempt_count")
	if err != nil {
		return nil, fmt.Errorf("error verifying kernel symbol: %w", err)
	}

	kSpec, err := cache.Kernel()
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel btf spec: %w", err)
	}

	// if the kernel btf was create with an older version of pahole it may not
	// include per cpu variables, even if the symbol is exported in kallsyms.
	// This happens on debian 11 shipped with kernel 5.10.
	var typ *btf.Var
	preemptCountExistsInBTF := kSpec.TypeByName("__preempt_count", &typ) == nil

	if len(preemptCountMissing) == 0 && preemptCountExistsInBTF {
		return map[string]uint64{"use_preempt_count": uint64(1)}, nil
	}

	// if __preempt_count is not present make sure that pcpu_hot is atleast present
	// otherwise we cannot get the preempt_count. pcpu_hot was introduced in 6.1 but
	// we may hit this case when __preempt_count is missing from btf for versions <6.1
	pcpuHotExistsInBTF := kSpec.TypeByName("pcpu_hot", &typ) == nil
	if !pcpuHotExistsInBTF {
		return nil, ErrRequiredVarsMissingInBTF
	}

	var pcpuHot *btf.Struct
	if err := kSpec.TypeByName("pcpu_hot", &pcpuHot); err != nil {
		return nil, fmt.Errorf("unable to get definition for struct pcpu_hot: %w", err)
	}

	if !btfStructHasField(pcpuHot.Members, "preempt_count") {
		return nil, errors.New("__preempt_count missing from kernel's btf and pcpu_hot->preempt_count does not exist")
	}

	return map[string]uint64{}, nil
}

func btfStructHasField(members []btf.Member, name string) bool {
	for _, m := range members {
		if m.Name == name {
			return true
		}
		if m.Name == "" {
			var inner []btf.Member
			switch t := m.Type.(type) {
			case *btf.Struct:
				inner = t.Members
			case *btf.Union:
				inner = t.Members
			}
			if btfStructHasField(inner, name) {
				return true
			}
		}
	}

	return false
}

func preemptCountARM64() (map[string]uint64, error) {
	if err := features.HaveHelperInRawTracepoint(asm.FnGetCurrentTaskBtf); err != nil {
		return nil, errors.New("required helper bpf_get_current_task_btf not present")
	}

	return map[string]uint64{}, nil
}
