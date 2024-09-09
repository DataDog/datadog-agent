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

// hostProcInternal is the testable/benchmarkable version of HostProc
func hostProcInternal(procFsRoot string, combineWith ...string) string {
	const sepLen = len(string(filepath.Separator))

	// pre-compute the total size of the string to avoid reallocations
	size := len(procFsRoot)
	allEmpty := true
	for _, c := range combineWith {
		if len(c) > math.MaxInt-sepLen {
			panic("int overflow in HostProc")
		}
		toadd := sepLen + len(c)

		if size > math.MaxInt-toadd {
			panic("int overflow in HostProc")
		}
		size += toadd

		if c != "" {
			allEmpty = false
		}
	}

	// early escape to return "" if procFsRoot is empty and all combineWith are empty
	// this is to match the behavior of filepath.Join
	if procFsRoot == "" && allEmpty {
		return ""
	}

	var builder strings.Builder
	builder.Grow(size)

	builder.WriteString(procFsRoot)
	for i, c := range combineWith {
		// the goal is not add the separator at the very beginning if procFsRoot is empty because we want to
		// get a relative path, again the goal is to match the behavior of filepath.Join
		if i != 0 || procFsRoot != "" {
			builder.WriteRune(filepath.Separator)
		}
		builder.WriteString(c)
	}

	return filepath.Clean(builder.String())
}

// HostProc returns the location of a host's procfs. This can and will be
// overridden when running inside a container.
func HostProc(combineWith ...string) string {
	return hostProcInternal(ProcFSRoot(), combineWith...)
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
