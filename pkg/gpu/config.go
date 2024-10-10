// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gpu provides the GPU monitoring functionality.
package gpu

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// GPUConfigNS is the namespace for the GPU monitoring probe.
const GPUConfigNS = "gpu_monitoring"

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	*ebpf.Config
	ScanTerminatedProcessesInterval time.Duration
	InitialProcessSync              bool
}

// NewConfig generates a new configuration for the GPU monitoring probe.
func NewConfig() *Config {
	return &Config{
		Config:                          ebpf.NewConfig(),
		ScanTerminatedProcessesInterval: 5 * time.Second,
		InitialProcessSync:              true,
	}
}
