// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf

package gpu

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	errNotSupported      = errors.New("GPU Monitoring is not supported")
	minimumKernelVersion = kernel.VersionCode(5, 8, 0)
)

func checkGPUSupported() error {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("%w: could not determine the current kernel version: %w", errNotSupported, err)
	}

	if kversion < minimumKernelVersion {
		return fmt.Errorf("%w: a Linux kernel version of %s or higher is required; we detected %s", errNotSupported, minimumKernelVersion, kversion)
	}

	hasUretprobeSyscallSeccompBug, err := kernelbugs.HasUretprobeSyscallSeccompBug()
	if err != nil {
		return fmt.Errorf("%w: could not determine if the kernel has a bug that might cause segmentation faults when using uretprobe: %w", errNotSupported, err)
	}

	if hasUretprobeSyscallSeccompBug {
		return fmt.Errorf("%w: the kernel has a bug that might cause segmentation faults when using uretprobe; please upgrade to a kernel version that has been patched", errNotSupported)
	}

	return nil
}
