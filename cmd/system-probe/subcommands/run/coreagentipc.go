// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const coreAgentIPCEnvVar = "DD_SYSTEM_PROBE_CONFIG_CORE_AGENT_IPC_ENABLED"

// Mirrors the system_probe_config.core_agent_ipc.enabled schema default; it is read
// before any config component exists, so the two have to be kept in sync by hand.
const defaultCoreAgentIPCEnabled = true

// coreAgentIPCEnabled reports whether system-probe may talk to the core agent, from the
// environment or system-probe.yaml. It decides which modules go into the fx graph, so it
// runs before that graph (and the config components) are built: the config stream
// consumer in particular is a dependency of the core config component.
func coreAgentIPCEnabled(sysprobeConfPath string) bool {
	if v, ok := os.LookupEnv(coreAgentIPCEnvVar); ok {
		if enabled, err := strconv.ParseBool(v); err == nil {
			return enabled
		}
	}

	data, err := os.ReadFile(sysprobeConfFile(sysprobeConfPath))
	if err != nil {
		return defaultCoreAgentIPCEnabled
	}
	var cfg struct {
		SystemProbeConfig struct {
			CoreAgentIPC struct {
				Enabled *bool `yaml:"enabled"`
			} `yaml:"core_agent_ipc"`
		} `yaml:"system_probe_config"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil || cfg.SystemProbeConfig.CoreAgentIPC.Enabled == nil {
		return defaultCoreAgentIPCEnabled
	}
	return *cfg.SystemProbeConfig.CoreAgentIPC.Enabled
}

// sysprobeConfFile resolves the system-probe.yaml path the same way the system-probe
// config loader does: a .yaml path is used as is, a directory gets system-probe.yaml
// appended, and no path at all means the platform default.
func sysprobeConfFile(sysprobeConfPath string) string {
	switch {
	case sysprobeConfPath == "":
		return defaultpaths.GetDefaultSysProbeConfFile()
	case strings.HasSuffix(sysprobeConfPath, ".yaml"):
		return sysprobeConfPath
	default:
		return filepath.Join(sysprobeConfPath, "system-probe.yaml")
	}
}
