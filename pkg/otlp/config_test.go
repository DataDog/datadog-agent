// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadConfig(path string) (config.Config, error) {
	cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.SetupOTLP(cfg)
	cfg.SetConfigFile(path)
	err := cfg.ReadInConfig()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		enabled bool
	}{
		{
			name:    "bind_host",
			path:    "./testdata/bindhost.yaml",
			enabled: true,
		},
		{
			name:    "disabled",
			path:    "./testdata/disabled.yaml",
			enabled: false,
		},
		{
			name:    "invalid",
			path:    "./testdata/invalid.yaml",
			enabled: true,
		},
		{
			name:    "no bind_host",
			path:    "./testdata/nobindhost.yaml",
			enabled: true,
		},
		{
			name:    "nonlocal",
			path:    "./testdata/nonlocal.yaml",
			enabled: true,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg, err := loadConfig(testInstance.path)
			require.NoError(t, err)
			assert.Equal(t, testInstance.enabled, IsEnabled(cfg))
		})
	}
}

func TestFromAgentConfig(t *testing.T) {
	tests := []struct {
		name string
		path string
		cfg  PipelineConfig
		err  string
	}{
		{
			name: "bind_host",
			path: "./testdata/bindhost.yaml",
			cfg: PipelineConfig{
				BindHost:       "bindhost",
				HTTPPort:       1234,
				GRPCPort:       5678,
				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
			},
		},
		{
			name: "no bind_host",
			path: "./testdata/nobindhost.yaml",
			cfg: PipelineConfig{
				BindHost:       "localhost",
				HTTPPort:       1234,
				GRPCPort:       5678,
				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
			},
		},
		{
			name: "invalid",
			path: "./testdata/invalid.yaml",
			err: strings.Join([]string{
				"http port is invalid: -1 is out of [0, 65535] range",
				"gRPC port is invalid: -1 is out of [0, 65535] range",
				"internal trace port is invalid: -1 is out of [0, 65535] range",
			},
				"; ",
			),
		},
		{
			name: "nonlocal",
			path: "./testdata/nonlocal.yaml",
			cfg: PipelineConfig{
				BindHost:       "0.0.0.0",
				HTTPPort:       1234,
				GRPCPort:       5678,
				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
			},
		},
		{
			name: "all disabled",
			path: "./testdata/alldisabled.yaml",
			err:  "at least one OTLP signal needs to be enabled",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg, err := loadConfig(testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
			} else {
				assert.Equal(t, testInstance.cfg, pcfg)
			}
		})
	}
}
