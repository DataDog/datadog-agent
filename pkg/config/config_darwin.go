// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultGuiPort = 5002

var (
	defaultConfdPath            = path.Join(version.AgentPath, "etc/conf.d")
	defaultAdditionalChecksPath = path.Join(version.AgentPath, "etc/checks.d")
	defaultRunPath              = path.Join(version.AgentPath, "run")

	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = path.Join(version.AgentPath, "logs/security-agent.log")
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = path.Join(version.AgentPath, "logs/process-agent.log")
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress     = path.Join(version.AgentPath, "run/sysprobe.sock")
	defaultSystemProbeLogFilePath = path.Join(version.AgentPath, "logs/system-probe.log")
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = path.Join(version.AgentPath, "bin/agent/agent")
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}
