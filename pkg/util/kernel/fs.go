// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/moby/sys/mountinfo"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// MountInfoPidPath returns the path to the mountinfo file of a pid in /proc
func MountInfoPidPath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("/%d/mountinfo", pid))
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
