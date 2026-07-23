// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && npm

package sender

import (
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	ddos "github.com/DataDog/datadog-agent/pkg/util/os"
)

type directSenderConsumer struct {
	log       log.Component
	processes map[uint32]*process
	mtx       sync.Mutex
	procprobe procutil.Probe

	proxyFilter          *dockerProxyFilter
	extractor            *serviceExtractor
	processNameExtractor *processNameExtractor
	pidAliveFunc         func(pid int) bool
	fetchProcesses       bool
}

func newDirectSenderConsumer(log log.Component, sysprobeconfig sysprobeconfig.Component) *directSenderConsumer {
	return &directSenderConsumer{
		log:                  log,
		processes:            make(map[uint32]*process),
		procprobe:            procutil.NewProcessProbe(),
		proxyFilter:          newDockerProxyFilter(log),
		extractor:            newServiceExtractor(sysprobeconfig),
		processNameExtractor: newProcessNameExtractor(),
		pidAliveFunc:         ddos.PidExists,
	}
}

// Copy implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) Copy(ev *model.Event) any {
	p := &process{
		Pid:       ev.GetProcessPid(),
		PPid:      ev.GetProcessPpid(),
		EventType: ev.GetEventType(),
		Exe:       ev.GetExecFilePath(),
	}
	return p
}

func (d *directSenderConsumer) handleNewProcess(p *process) {
	if p.Cwd != "" && p.Comm != "" && len(p.Cmdline) > 0 {
		return
	}

	pp, err := d.procprobe.ProcessFromPID(int32(p.Pid))
	if pp == nil || err != nil {
		return
	}

	if p.Cwd == "" {
		p.Cwd = pp.Cwd
	}
	if p.Comm == "" {
		p.Comm = pp.Comm
	}
	if len(p.Cmdline) == 0 {
		p.Cmdline = pp.Cmdline
	}
}

func (d *directSenderConsumer) collectProcesses() error {
	if !d.fetchProcesses {
		return nil
	}

	procs, err := d.procprobe.ProcessesByPID(time.Now(), false)
	if err != nil {
		return err
	}

	for _, pp := range procs {
		p := &process{
			Pid:       uint32(pp.Pid),
			PPid:      uint32(pp.Ppid),
			Cmdline:   pp.Cmdline,
			Cwd:       pp.Cwd,
			Comm:      pp.Comm,
			Exe:       pp.Exe,
			EventType: model.ExecEventType,
		}
		d.HandleEvent(p)
	}

	return nil
}
