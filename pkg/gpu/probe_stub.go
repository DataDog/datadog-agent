// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux_bpf && linux

package gpu

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
)

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	Telemetry telemetry.Component
	NvmlLib   nvml.Interface
}

// Probe is not implemented on non-linux systems
type Probe struct{}

// NewProbe is not implemented on non-linux systems
func NewProbe(_ *config.Config, _ ProbeDependencies) (*Probe, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *Probe) Close() {}

// GetAndFlush is not implemented on non-linux systems
func (t *Probe) GetAndFlush() (model.GPUStats, error) {
	return model.GPUStats{}, nil
}
