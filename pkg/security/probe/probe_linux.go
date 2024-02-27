// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// EBPFOrigin eBPF origin
	EBPFOrigin = "ebpf"
	// EBPFLessOrigin eBPF less origin
	EBPFLessOrigin = "ebpfless"
)

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts, wmeta optional.Option[workloadmeta.Component]) (*Probe, error) {
	opts.normalize()

	p := newProbe(config, opts)

	if opts.EBPFLessEnabled {
		pp, err := NewEBPFLessProbe(p, config, opts, wmeta)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
	} else {
		pp, err := NewEBPFProbe(p, config, opts, wmeta)
		if err != nil {
			return nil, err
		}
		p.PlatformProbe = pp
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
