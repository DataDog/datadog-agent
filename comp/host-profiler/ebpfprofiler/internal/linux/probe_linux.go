//go:build linux

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package linux

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

type kernelVersion struct {
	major, minor, patch uint32
}

var getKernelVersion = sync.OnceValues(func() (kernelVersion, error) {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return kernelVersion{}, err
	}
	var major, minor, patch uint32
	_, _ = fmt.Fscanf(bytes.NewReader(uname.Release[:]), "%d.%d.%d", &major, &minor, &patch)
	return kernelVersion{major: major, minor: minor, patch: patch}, nil
})

// ProbeBPFSyscall checks if the syscall EBPF is available on the system.
func ProbeBPFSyscall() error {
	_, _, errNo := unix.Syscall(unix.SYS_BPF, uintptr(unix.BPF_PROG_TYPE_UNSPEC), uintptr(0), 0)
	if errNo == unix.ENOSYS {
		return errors.New("eBPF syscall is not available on your system")
	}
	return nil
}

// GetCurrentKernelVersion returns the major, minor and patch version of the kernel of the host
// from the utsname struct.
func GetCurrentKernelVersion() (major, minor, patch uint32, err error) {
	v, err := getKernelVersion()
	return v.major, v.minor, v.patch, err
}
