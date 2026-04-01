//go:build !linux

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package linux

import (
	"fmt"
	"runtime"
)

// ProbeBPFSyscall checks if the syscall EBPF is available on the system.
func ProbeBPFSyscall() error {
	return fmt.Errorf("eBPF is not available on your system %s", runtime.GOOS)
}

// GetCurrentKernelVersion returns an error for OS other than linux.
func GetCurrentKernelVersion() (_, _, _ uint32, err error) {
	return 0, 0, 0, fmt.Errorf("kernel version detection is not supported on %s",
		runtime.GOOS)
}
