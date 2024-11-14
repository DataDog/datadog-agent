// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"os"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	gopsutilProcess "github.com/shirou/gopsutil/v3/process"
)

const (
	// EBPFOrigin eBPF origin
	EBPFOrigin = "ebpf"
	// EBPFLessOrigin eBPF less origin
	EBPFLessOrigin = "ebpfless"
)

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts, telemetry telemetry.Component) (*Probe, error) {
	opts.normalize()

	p := newProbe(config, opts)

	acc, err := NewAgentContainerContext()
	if err != nil {
		return nil, err
	}

	if opts.EBPFLessEnabled {
		pp, err := NewEBPFLessProbe(p, config, opts, telemetry)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
		p.agentContainerContext = acc
	} else {
		pp, err := NewEBPFProbe(p, config, opts, telemetry)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
		p.agentContainerContext = acc
	}

	return p, nil
}

// Origin returns origin
func (p *Probe) Origin() string {
	if p.Opts.EBPFLessEnabled {
		return EBPFLessOrigin
	}
	return EBPFOrigin
}

func NewAgentContainerContext() (*events.AgentContainerContext, error) {
	pid := os.Getpid()

	procProcess, err := gopsutilProcess.NewProcess(int32(pid))
	if err != nil {
		return nil, err
	}
	createTime, err := procProcess.CreateTime()
	if err != nil {
		return nil, err
	}
	acc := &events.AgentContainerContext{
		CreatedAt: uint64(createTime),
	}

	cid, err := utils.GetProcContainerID(uint32(pid), uint32(pid))
	if err != nil { // could fail if not in container
		return acc, nil
	}
	acc.ContainerID = cid
	return acc, nil
}
