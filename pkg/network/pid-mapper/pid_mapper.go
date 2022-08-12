// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package mapper

import (
	"fmt"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	manager "github.com/DataDog/ebpf-manager"
)

type PidMapper struct {
	ebpfProgram *ebpfProgram
}

func NewPidMapper(c *config.Config, sockToPid *ebpf.Map) (*PidMapper, error) {
	p, err := newEBPFProgram(c)
	if err != nil {
		return nil, fmt.Errorf("error creating ebpf program: %w", err)
	}

	if err := p.Init(c, sockToPid); err != nil {
		return nil, fmt.Errorf("error initializing ebpf programs: %w", err)
	}

	initializor, initializorDone, err := p.Start()
	if err != nil {
		return nil, err
	}

	if err := initializor(); err != nil {
		p.Stop(manager.CleanInternal)
		return nil, fmt.Errorf("error getting existing sock->pid mappings: %w", err)
	}

	if err := initializorDone(); err != nil {
		p.Stop(manager.CleanInternal)
		return nil, fmt.Errorf("error cleaning up after building existing mappings: %w", err)
	}

	return &PidMapper{
		p,
	}, nil
}

func (p *PidMapper) Stop() {
	p.ebpfProgram.Stop(manager.CleanInternal)
}
