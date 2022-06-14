// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package process

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

type Process struct {
	Pid         uint32
	ContainerID string
	Envs        []string
	StartTime   int64
}

type Handler func(*Process)

type Monitor struct {
	sync.Mutex

	program  *ebpfProgram
	handlers []Handler
}

func NewMonitor(cfg *config.Config) (*Monitor, error) {
	pm := &Monitor{}
	prog, err := newEbpfProgram(cfg, func(pe *ebpf.ProcessExecEvent) {
		if pe == nil {
			return
		}

		p := &Process{Pid: pe.Process.Pid}

		switch ebpf.ProcessEventType(pe.Event.Type) {
		case ebpf.ProcessEventTypeExec:
			p.StartTime = int64(pe.Proc_entry.Timestamp)
		case ebpf.ProcessEventTypeFork:
			p.StartTime = int64(pe.Pid_entry.Fork_timestamp)
		}

		for _, h := range pm.handlers {
			h(p)
		}
	}, nil)

	if err != nil {
		return nil, err
	}

	pm.program = prog
	return pm, nil
}

func (pm *Monitor) Start() error {
	if err := pm.program.Init(); err != nil {
		return err
	}

	return pm.program.Start()
}

func (pm *Monitor) Stop() error {
	return pm.program.Stop()
}

func (pm *Monitor) AddHandler(h Handler) {
	if h == nil {
		return
	}

	pm.Lock()
	defer pm.Unlock()

	pm.handlers = append(pm.handlers, h)
}
