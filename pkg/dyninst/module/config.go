// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Config is the configuration for the dynamic instrumentation module.
type Config struct {
	ebpf.Config
	DynamicInstrumentationEnabled bool
	LogUploaderURL                string
	DiagsUploaderURL              string
}

// NewConfig creates a new Config object
func NewConfig(spConfig *sysconfigtypes.Config) (*Config, error) {
	var diEnabled bool
	if spConfig != nil {
		_, diEnabled = spConfig.EnabledModules[config.DynamicInstrumentationModule]
	}
	agentHost := getAgentHost()
	return &Config{
		Config:                        *ebpf.NewConfig(),
		DynamicInstrumentationEnabled: diEnabled,
		LogUploaderURL:                fmt.Sprintf("http://%s:8126/debugger/v1/input", agentHost),
		DiagsUploaderURL:              fmt.Sprintf("http://%s:8126/debugger/v1/diagnostics", agentHost),
	}, nil
}

func getAgentHost() string {
	ddAgentHost := os.Getenv("DD_AGENT_HOST")
	if ddAgentHost == "" {
		ddAgentHost = "localhost"
	}
	return ddAgentHost
}
