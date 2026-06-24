// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamconsumerimpl

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const enabledEnvVar = "DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED"

// isEnabled reports whether the consumer should run, from env or datadog.yaml.
func isEnabled(cliConfigPath string) bool {
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

// readSettings overlays values from datadog.yaml onto a subset of the global config (defaults+env).
func readSettings(cliConfigPath string) configstreambootstrap.Settings {
	bs := configstreambootstrap.ReadBaseSettings()

	for _, path := range yamlCandidates(cliConfigPath) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			AuthTokenFilePath string `yaml:"auth_token_file_path"`
			IPCCertFilePath   string `yaml:"ipc_cert_file_path"`
			CmdHost           string `yaml:"cmd_host"`
			CmdPort           int    `yaml:"cmd_port"`
			VSockAddr         string `yaml:"vsock_addr"`
			RemoteAgent       struct {
				Registry struct {
					Enabled *bool `yaml:"enabled"`
				} `yaml:"registry"`
			} `yaml:"remote_agent"`
		}
		_ = yaml.Unmarshal(data, &cfg)

		if cfg.AuthTokenFilePath != "" {
			bs.AuthTokenFilePath = cfg.AuthTokenFilePath
		}
		if cfg.IPCCertFilePath != "" {
			bs.IPCCertFilePath = cfg.IPCCertFilePath
		}
		if cfg.CmdHost != "" {
			bs.CmdHost = cfg.CmdHost
		}
		if cfg.CmdPort > 0 {
			bs.CmdPort = cfg.CmdPort
		}
		if cfg.VSockAddr != "" {
			bs.VSockAddr = cfg.VSockAddr
		}
		if cfg.RemoteAgent.Registry.Enabled != nil {
			bs.RARRegistryEnabled = *cfg.RemoteAgent.Registry.Enabled
		}
		break
	}
	return bs
}

func yamlCandidates(cliConfigPath string) []string {
	out := make([]string, 0, 2)
	for _, path := range []string{cliConfigPath, defaultpaths.GetDefaultConfPath()} {
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

func resolvedConfigFile(cliConfigPath string) string {
	candidates := yamlCandidates(cliConfigPath)
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
