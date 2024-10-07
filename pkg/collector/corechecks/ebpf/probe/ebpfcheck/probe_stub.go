// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package ebpfcheck is the system-probe side of the eBPF check
package ebpfcheck

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Probe is not implemented on non-linux systems
type Probe struct{}

// NewProbe is not implemented on non-linux systems
func NewProbe(*ebpf.Config) (*Probe, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *Probe) Close() {}

// GetAndFlush is not implemented on non-linux systems
func (t *Probe) GetAndFlush() model.EBPFStats {
	return model.EBPFStats{}
}
