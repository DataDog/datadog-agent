// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumer implements a component that consumes config streams from the core agent.
//
// team: agent-configuration
package configstreamconsumer

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"
)

const enabledEnvVar = "DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED"

// Mirrors comp/core/config.DefaultConfPath without taking that dep (would cycle through impl).
var defaultConfPath = func() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\Datadog`
	}
	return "/etc/datadog-agent"
}()

// Params for the configstreamconsumer component.
type Params struct {
	// ClientName identifies this remote agent (e.g. "system-probe"). Required.
	ClientName string
	// CLIConfigPath is the binary's resolved -config / --cfgpath (file or dir).
	CLIConfigPath string
	// ReadyTimeout caps NewComponent's wait for the first snapshot. Defaults to 60s.
	ReadyTimeout time.Duration
}

// Component is the config stream consumer. IsActive is true once the initial snapshot
// has been applied to the global config builder.
type Component interface {
	IsActive() bool
}

// IsEnabled reports whether the consumer should run, from env or datadog.yaml.
func IsEnabled(cliConfigPath string) bool {
	if v, ok := os.LookupEnv(enabledEnvVar); ok {
		if enabled, err := strconv.ParseBool(v); err == nil {
			return enabled
		}
	}
	for _, path := range yamlCandidates(cliConfigPath) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			RemoteAgent struct {
				ConfigStream struct {
					Consumer struct {
						Enabled bool `yaml:"enabled"`
					} `yaml:"consumer"`
				} `yaml:"configstream"`
			} `yaml:"remote_agent"`
		}
		_ = yaml.Unmarshal(data, &cfg)
		return cfg.RemoteAgent.ConfigStream.Consumer.Enabled
	}
	return false
}

func yamlCandidates(cliConfigPath string) []string {
	out := make([]string, 0, 2)
	for _, path := range []string{cliConfigPath, defaultConfPath} {
		if path == "" {
			continue
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			path = filepath.Join(path, "datadog.yaml")
		}
		out = append(out, path)
	}
	return out
}
