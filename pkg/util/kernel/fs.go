// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/moby/sys/mountinfo"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// MountInfoPidPath returns the path to the mountinfo file of a pid in /proc
func MountInfoPidPath(pid int32) string {
	return filepath.Join(ProcFSRoot(), fmt.Sprintf("/%d/mountinfo", pid))
}

// ParseMountInfoFile collects the mounts for a specific process ID.
func ParseMountInfoFile(pid int32) ([]*mountinfo.Info, error) {
	f, err := os.Open(MountInfoPidPath(pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return mountinfo.GetMountsFromReader(f, nil)
}

// ProcFSRoot retrieves the current procfs dir we should use
var ProcFSRoot = funcs.MemoizeNoError(func() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}
	if os.Getenv("DOCKER_DD_AGENT") != "" {
		if _, err := os.Stat("/host"); err == nil {
			return "/host/proc"
		}
	}
	return "/proc"
})

// SysFSRoot retrieves the current sysfs dir we should use
var SysFSRoot = funcs.MemoizeNoError(func() string {
	if v := os.Getenv("HOST_SYS"); v != "" {
		return v
	}
	if os.Getenv("DOCKER_DD_AGENT") != "" {
		if _, err := os.Stat("/host"); err == nil {
			return "/host/sys"
		}
	}
	return "/sys"
})

// HostProc returns the location of a host's procfs. This can and will be
// overridden when running inside a container.
func HostProc(combineWith ...string) string {
	const sepLen = len(string(filepath.Separator))

	root := ProcFSRoot()

	size := len(root)
	for _, c := range combineWith {
		if len(c) > math.MaxInt-sepLen {
			panic("int overflow in HostProcV2")
		}
		toadd := sepLen + len(c)

		if size > math.MaxInt-toadd {
			panic("int overflow in HostProcV2")
		}
		size += toadd
	}

	var builder strings.Builder
	builder.Grow(size)

	builder.WriteString(ProcFSRoot())
	for _, c := range combineWith {
		builder.WriteRune(filepath.Separator)
		builder.WriteString(c)
	}
	return filepath.Clean(builder.String())
}

// RootNSPID returns the current PID from the root namespace
var RootNSPID = funcs.Memoize(func() (int, error) {
	pidPath := filepath.Join(ProcFSRoot(), "self")
	pidStr, err := os.Readlink(pidPath)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(pidStr)
})
