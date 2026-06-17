// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	gopsutil "github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func (d *directSenderConsumer) handleNewProcess(p *process) {
	if p.Cwd != "" && p.Comm != "" && p.Exe != "" {
		return
	}

	pp, err := gopsutil.NewProcess(int32(p.Pid))
	if err != nil {
		return
	}
	if p.Cwd == "" {
		p.Cwd, _ = pp.Cwd()
	}
	if p.Comm == "" {
		p.Comm, _ = pp.Name()
	}
	if p.Exe == "" {
		p.Exe, _ = pp.Exe()
	}
}

func (d *directSenderConsumer) collectProcesses() error {
	if !d.fetchProcesses {
		return nil
	}

	procs, err := gopsutil.Processes()
	if err != nil {
		return err
	}

	for _, pp := range procs {
		ppid, _ := pp.Ppid()
		cmdline, _ := pp.CmdlineSlice()
		cwd, _ := pp.Cwd()
		comm, _ := pp.Name()
		exe, _ := pp.Exe()
		p := &process{
			Pid:       uint32(pp.Pid),
			PPid:      uint32(ppid),
			Cmdline:   cmdline,
			Cwd:       cwd,
			Comm:      comm,
			Exe:       exe,
			EventType: model.ExecEventType,
		}
		d.HandleEvent(p)
	}

	return nil
}
