// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/gopsutil/process"
	"github.com/moby/sys/mountinfo"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Getpid returns the current process ID in the host namespace
func Getpid() int32 {
	p, err := os.Readlink(filepath.Join(util.HostProc(), "/self"))
	if err == nil {
		if pid, err := strconv.Atoi(p); err == nil {
			return int32(pid)
		}
	}
	return int32(os.Getpid())
}

// MountInfoPath returns the path to the mountinfo file of the current pid in /proc
func MountInfoPath() string {
	return filepath.Join(util.HostProc(), "/self/mountinfo")
}

// MountInfoPidPath returns the path to the mountinfo file of a pid in /proc
func MountInfoPidPath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("/%d/mountinfo", pid))
}

// CgroupTaskPath returns the path to the cgroup file of a pid in /proc
func CgroupTaskPath(tgid, pid uint32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/task/%d/cgroup", tgid, pid))
}

// ProcExePath returns the path to the exe file of a pid in /proc
func ProcExePath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/exe", pid))
}

// StatusPath returns the path to the status file of a pid in /proc
func StatusPath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/status", pid))
}

// CapEffCapEprm returns the effective and permitted kernel capabilities of a process
func CapEffCapEprm(pid int32) (uint64, uint64, error) {
	var capEff, capPrm uint64
	contents, err := ioutil.ReadFile(StatusPath(pid))
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		tabParts := strings.SplitN(line, "\t", 2)
		if len(tabParts) < 2 {
			continue
		}
		value := tabParts[1]
		switch strings.TrimRight(tabParts[0], ":") {
		case "CapEff":
			capEff, err = strconv.ParseUint(value, 16, 64)
			if err != nil {
				return 0, 0, err
			}
		case "CapPrm":
			capPrm, err = strconv.ParseUint(value, 16, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}
	return capEff, capPrm, nil
}

// PidTTY returns the TTY of the given pid
func PidTTY(pid int32) string {
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
func ParseMountInfoFile(pid int32) ([]*mountinfo.Info, error) {
	f, err := os.Open(MountInfoPidPath(pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return mountinfo.GetMountsFromReader(f, nil)
}

// GetProcesses returns list of active processes
func GetProcesses() ([]*process.Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	var processes []*process.Process
	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			// the process does not exist anymore, continue
			continue
		}
		processes = append(processes, proc)
	}

	return processes, nil
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

	memInfo, err := p.MemoryInfo()
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
		MemInfo:    memInfo,
	}
}
