// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux,!linux_bpf

package probe

import (
	"github.com/DataDog/ebpf/manager"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	resolvers *Resolvers
}

// Init initialises the probe
func (p *Probe) Init() error {
	return nil
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (p *Probe) ApplyFilterPolicy(eventType eval.EventType, mode PolicyMode, flags PolicyFlag) error {
	return nil
}

// ApplyApprovers applies approvers
func (p *Probe) ApplyApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	return nil
}

// RegisterProbesSelectors register the given probes selectors
func (p *Probe) RegisterProbesSelectors(selectors []manager.ProbesSelector) error {
	return nil
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config) (*Probe, error) {
	p := &Probe{}

	resolvers, err := NewResolvers(p)
	if err != nil {
		return nil, err
	}

	p.resolvers = resolvers

	return p, nil
}
