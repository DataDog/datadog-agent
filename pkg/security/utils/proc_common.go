// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"github.com/shirou/gopsutil/v3/process"
)

// GetProcesses returns list of active processes
func GetProcesses() ([]*process.Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	var processes []*process.Process
	for _, pid := range pids {
		var proc *process.Process
		proc, err = process.NewProcess(pid)
		if err != nil {
			// the process does not exist anymore, continue
			continue
		}
		processes = append(processes, proc)
	}

	return processes, nil
}

// FilledProcess defines a filled process
type FilledProcess struct {
	Pid        int32
	Ppid       int32
	CreateTime int64
	Name       string
	Uids       []int32
	Gids       []int32
	MemInfo    *process.MemoryInfoStat
	Cmdline    []string
}

// GetFilledProcess returns a FilledProcess from a Process input
func GetFilledProcess(p *process.Process) (*FilledProcess, error) {
	ppid, err := p.Ppid()
	if err != nil {
		return nil, err
	}

	createTime, err := p.CreateTime()
	if err != nil {
		return nil, err
	}

	uids, err := p.Uids()
	if err != nil {
		return nil, err
	}

	gids, err := p.Gids()
	if err != nil {
		return nil, err
	}

	name, err := p.Name()
	if err != nil {
		return nil, err
	}

	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}

	cmdLine, err := p.CmdlineSlice()
	if err != nil {
		return nil, err
	}

	return &FilledProcess{
		Pid:        p.Pid,
		Ppid:       ppid,
		CreateTime: createTime,
		Name:       name,
		Uids:       uids,
		Gids:       gids,
		MemInfo:    memInfo,
		Cmdline:    cmdLine,
	}, nil
}
