// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu provides the GPU monitoring functionality.
package gpu

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// GPUConfigNS is the namespace for the GPU monitoring probe.
const GPUConfigNS = "gpu_monitoring"

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	*ebpf.Config
}

// NewConfig generates a new configuration for the GPU monitoring probe.
func NewConfig() *Config {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)

	return &Config{
		Config: ebpf.NewConfig(),
	}
}
