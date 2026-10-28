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

const (
	coreAgentIPCEnvVar     = "DD_SYSTEM_PROBE_CONFIG_CORE_AGENT_IPC_ENABLED"
	fleetPoliciesDirEnvVar = "DD_FLEET_POLICIES_DIR"
)

// Mirrors the system_probe_config.core_agent_ipc.enabled schema default; it is read
// before any config component exists, so the two have to be kept in sync by hand.
const defaultCoreAgentIPCEnabled = true

// sysprobeYAML holds the system-probe.yaml keys coreAgentIPCEnabled needs.
type sysprobeYAML struct {
	FleetPoliciesDir  string `yaml:"fleet_policies_dir"`
	SystemProbeConfig struct {
		CoreAgentIPC struct {
			Enabled *bool `yaml:"enabled"`
		} `yaml:"core_agent_ipc"`
	} `yaml:"system_probe_config"`
}

// coreAgentIPCEnabled reports whether system-probe may talk to the core agent. It decides
// which modules go into the fx graph, so it runs before that graph (and the config
// components) are built: the config stream consumer in particular is a dependency of the
// core config component. It follows the config loader's precedence: the fleet policy
// file, then the environment, then system-probe.yaml. The fleet policies directory
// stored in the Windows registry is not looked up.
func coreAgentIPCEnabled(sysprobeConfPath, fleetPoliciesDirPath string) bool {
	local := readSysprobeYAML(sysprobeConfFile(sysprobeConfPath))

	if fleetPoliciesDirPath == "" {
		fleetPoliciesDirPath = os.Getenv(fleetPoliciesDirEnvVar)
	}
	if fleetPoliciesDirPath == "" {
		fleetPoliciesDirPath = local.FleetPoliciesDir
	}
	if fleetPoliciesDirPath != "" {
		fleet := readSysprobeYAML(filepath.Join(fleetPoliciesDirPath, "system-probe.yaml"))
		if enabled := fleet.SystemProbeConfig.CoreAgentIPC.Enabled; enabled != nil {
			return *enabled
		}
	}

	if v, ok := os.LookupEnv(coreAgentIPCEnvVar); ok {
		if enabled, err := strconv.ParseBool(v); err == nil {
			return enabled
		}
	}

	if enabled := local.SystemProbeConfig.CoreAgentIPC.Enabled; enabled != nil {
		return *enabled
	}
	return defaultCoreAgentIPCEnabled
}

// readSysprobeYAML parses the keys coreAgentIPCEnabled needs, leaving them unset when the
// file is missing or invalid.
func readSysprobeYAML(path string) sysprobeYAML {
	var cfg sysprobeYAML
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}
	return cfg
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
