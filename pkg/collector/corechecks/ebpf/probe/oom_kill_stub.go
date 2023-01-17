// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf
// +build !linux_bpf

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// OOMKillProbe is not implemented on non-linux systems
type OOMKillProbe struct{}

// NewOOMKillProbe is not implemented on non-linux systems
func NewOOMKillProbe(cfg *ebpf.Config) (*OOMKillProbe, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *OOMKillProbe) Close() {}

// Get is not implemented on non-linux systems
func (t *OOMKillProbe) Get() []OOMKillStats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *OOMKillProbe) GetAndFlush() []OOMKillStats {
	return nil
}
