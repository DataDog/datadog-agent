// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	gopsutilProcess "github.com/shirou/gopsutil/v4/process"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// EBPFOrigin eBPF origin
	EBPFOrigin = "ebpf"
	// EBPFLessOrigin eBPF less origin
	EBPFLessOrigin = "ebpfless"
)

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, ipc ipc.Component, opts Opts) (*Probe, error) {
	opts.normalize()

	p := newProbe(config, opts)

	acc, err := NewAgentContainerContext()
	if err != nil {
		return nil, err
	}

	if opts.EBPFLessEnabled {
		pp, err := NewEBPFLessProbe(p, config, ipc, opts)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
		p.agentContainerContext = acc
	} else {
		pp, err := NewEBPFProbe(p, config, ipc, opts)
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

// IsRawPacketNotSupported returns if the raw packet feature is supported
func IsRawPacketNotSupported(kv *kernel.Version) bool {
	return IsNetworkNotSupported(kv) || (kv.IsAmazonLinuxKernel() && kv.Code < kernel.Kernel4_15) || (kv.IsUbuntuKernel() && kv.Code < kernel.Kernel5_2)
}

// IsNetworkNotSupported returns if the network feature is supported
func IsNetworkNotSupported(kv *kernel.Version) bool {
	// TODO: Oracle because we are missing offset
	return kv.IsRH7Kernel() || kv.IsOracleUEKKernel()
}

// IsCgroupSysCtlNotSupported returns if the cgroup/sysctl program is supported
func IsCgroupSysCtlNotSupported(kv *kernel.Version, cgroup2MountPath string) bool {
	return len(cgroup2MountPath) == 0 || !kv.HasCgroupSysctlSupportWithRingbuf()
}

// IsNetworkFlowMonitorNotSupported returns if the network flow monitor feature is supported
func IsNetworkFlowMonitorNotSupported(kv *kernel.Version) bool {
	return IsNetworkNotSupported(kv) || !kv.IsMapValuesToMapHelpersAllowed() || !kv.HasBPFForEachMapElemHelper()
}

// NewAgentContainerContext returns the agent container context
func NewAgentContainerContext() (*events.AgentContainerContext, error) {
	pid := utils.Getpid()

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
	if err != nil {
		return nil, err
	}
	acc.ContainerID = cid
	return acc, nil
}
