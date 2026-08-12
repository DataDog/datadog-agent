// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package lockcontention

import (
	"fmt"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/cilium/ebpf/asm"
)

func EBPFPreemptCountSupported() bool {
	return features.HaveHelperInRawTracepoint(asm.FnThisCpuPtr) == nil
}

func PreemptCountConstants() (map[string]uint64, error) {
	preemptCountMissing, err := ddebpf.VerifyKernelFuncs("__preempt_count")
	if err != nil {
		return nil, fmt.Errorf("error verifying kernel symbol: %w", err)
	}

	if len(preemptCountMissing) == 0 {
		return map[string]uint64{"use_preempt_count": uint64(1)}, nil
	}

	return map[string]uint64{}, nil
}
