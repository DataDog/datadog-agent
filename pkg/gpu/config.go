// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"strings"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

const gpuNS = "gpu_monitoring"

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	*ebpf.Config
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// NewConfig generates a new configuration for the GPU monitoring probe.
func NewConfig() *Config {
	cfg := ddconfig.SystemProbe()
	sysconfig.Adjust(cfg)

	return &Config{
		Config: ebpf.NewConfig(),
	}
}
