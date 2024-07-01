// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	defaultConfdPath            = "c:\\programdata\\datadog\\conf.d"
	defaultAdditionalChecksPath = "c:\\programdata\\datadog\\checks.d"
	defaultRunPath              = "c:\\programdata\\datadog\\run"
	defaultGuiPort              = 5002
	// DefaultUpdaterLogFile is the default updater log file
	DefaultUpdaterLogFile = "c:\\programdata\\datadog\\logs\\updater.log"
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = "c:\\programdata\\datadog\\logs\\security-agent.log"
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = "C:\\ProgramData\\Datadog\\logs\\process-agent.log"
	// DefaultOTelAgentLogFile is the default otel-agent log file
	DefaultOTelAgentLogFile = "C:\\ProgramData\\Datadog\\logs\\otel-agent.log"
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"
	// defaultEventMonitorAddress is the default address to be used for connecting to the event monitor
	defaultEventMonitorAddress    = "localhost:3335"
	defaultSystemProbeLogFilePath = "c:\\programdata\\datadog\\logs\\system-probe.log"
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
	// InstallPath is the default install path for the agent
	InstallPath = "c:\\Program Files\\Datadog\\Datadog Agent"
)

func osinit() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfdPath = filepath.Join(pd, "conf.d")
		defaultAdditionalChecksPath = filepath.Join(pd, "checks.d")
		defaultRunPath = filepath.Join(pd, "run")
		DefaultSecurityAgentLogFile = filepath.Join(pd, "logs", "security-agent.log")
		defaultSystemProbeLogFilePath = filepath.Join(pd, "logs", "system-probe.log")
		DefaultProcessAgentLogFile = filepath.Join(pd, "logs", "process-agent.log")
	}

	// Process Agent
	if _here, err := executable.Folder(); err == nil {
		agentFilePath := filepath.Join(_here, "..", "..", "embedded", "agent.exe")
		if _, err := os.Stat(agentFilePath); err == nil {
			DefaultDDAgentBin = agentFilePath
		}
	}
}
