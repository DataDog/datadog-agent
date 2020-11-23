// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package utils

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/gopsutil/process"
	"github.com/moby/sys/mountinfo"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// MountInfoPath returns the path to the mountinfo file of the current pid in /proc
func MountInfoPath() string {
	return filepath.Join(util.HostProc(), "/self/mountinfo")
}

// MountInfoPidPath returns the path to the mountinfo file of a pid in /proc
func MountInfoPidPath(pid uint32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("/%d/mountinfo", pid))
}

// CgroupTaskPath returns the path to the cgroup file of a pid in /proc
func CgroupTaskPath(tgid, pid uint32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/task/%d/cgroup", tgid, pid))
}

// ProcExePath returns the path to the exe file of a pid in /proc
func ProcExePath(pid uint32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/exe", pid))
}

// PidTTY returns the TTY of the given pid
func PidTTY(pid uint32) string {
	fdPath := filepath.Join(util.HostProc(), fmt.Sprintf("%d/fd/0", pid))

	ttyPath, err := os.Readlink(fdPath)
	if err != nil {
		return ""
	}

	if ttyPath == "/dev/null" {
		return ""
	}

	if strings.HasPrefix(ttyPath, "/dev/pts") {
		return "pts" + path.Base(ttyPath)
	}

	if strings.HasPrefix(ttyPath, "/dev") {
		return path.Base(ttyPath)
	}

	return ""
}

// ParseMountInfoFile collects the mounts for a specific process ID.
func ParseMountInfoFile(pid uint32) ([]*mountinfo.Info, error) {
	f, err := os.Open(MountInfoPidPath(pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return mountinfo.GetMountsFromReader(f, nil)
}

// GetFilledProcess returns a FilledProcess from a Process input
// TODO: make a PR to export a similar function in Datadog/gopsutil. We only populate the fields we need for now.
func GetFilledProcess(p *process.Process) *process.FilledProcess {
	ppid, err := p.Ppid()
	if err != nil {
		return nil
	}

	createTime, err := p.CreateTime()
	if err != nil {
		return nil
	}

	uids, err := p.Uids()
	if err != nil {
		return nil
	}

	gids, err := p.Gids()
	if err != nil {
		return nil
	}

	name, err := p.Name()
	if err != nil {
		return nil
	}

	return &process.FilledProcess{
		Pid:        p.Pid,
		Ppid:       ppid,
		CreateTime: createTime,
		Name:       name,
		Uids:       uids,
		Gids:       gids,
	}
}
