// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package oomkill is the system-probe side of the OOM Kill check
package oomkill

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Probe is not implemented on non-linux systems
type Probe struct{}

// NewProbe is not implemented on non-linux systems
func NewProbe(cfg *ebpf.Config) (*Probe, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *Probe) Close() {}

// Get is not implemented on non-linux systems
func (t *Probe) Get() []model.OOMKillStats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *Probe) GetAndFlush() []model.OOMKillStats {
	return nil
}
