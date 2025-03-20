// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && (!linux_bpf || !nvml)

package gpu

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
)

// ProbeDependencies holds the dependencies for the probe
type ProbeDependencies struct {
	Telemetry      telemetry.Component
	ProcessMonitor any // uprobes.ProcessMonitor is only compiled with the linux_bpf build tag, so we need to use type any here
	NvmlLib        any // nvml.Interface is only compiled with the nvml build tag, so we need to use type any here
	WorkloadMeta   workloadmeta.Component
}

// NewProbeDependencies is not implemented on non-linux systems
func NewProbeDependencies(_ *config.Config, _ telemetry.Component, _ any, _ workloadmeta.Component) (ProbeDependencies, error) {
	return ProbeDependencies{}, nil
}

// Probe is not implemented on non-linux systems
type Probe struct{}

// NewProbe is not implemented on non-linux systems
func NewProbe(_ *config.Config, _ ProbeDependencies) (*Probe, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (p *Probe) Close() {}

// CollectConsumedEvents is not implemented on non-linux systems
func (p *Probe) CollectConsumedEvents(_ context.Context, _ int) ([][]byte, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetAndFlush is not implemented on non-linux systems
func (p *Probe) GetAndFlush() (model.GPUStats, error) {
	return model.GPUStats{}, nil
}
