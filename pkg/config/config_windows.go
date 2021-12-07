// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

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
	defaultSyslogURI            = ""
	defaultGuiPort              = 5002
	// defaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	defaultSecurityAgentLogFile = "c:\\programdata\\datadog\\logs\\security-agent.log"
	defaultProcessAgentLogFile  = "c:\\programdata\\datadog\\logs\\process-agent.log"

	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress     = "localhost:3333"
	defaultSystemProbeLogFilePath = "c:\\programdata\\datadog\\logs\\system-probe.log"

	defaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
)

// ServiceName is the name that'll be used to register the Agent
const ServiceName = "DatadogAgent"

func osinit() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfdPath = filepath.Join(pd, "conf.d")
		defaultAdditionalChecksPath = filepath.Join(pd, "checks.d")
		defaultRunPath = filepath.Join(pd, "run")
		defaultSecurityAgentLogFile = filepath.Join(pd, "logs", "security-agent.log")
		defaultSystemProbeLogFilePath = filepath.Join(pd, "logs", "system-probe.log")
	} else {
		winutil.LogEventViewer(ServiceName, 0x8000000F, defaultConfdPath)
	}
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}

func init() {
	if pd, err := winutil.GetProgramDataDir(); err == nil {
		defaultProcessAgentLogFile = filepath.Join(pd, "logs", "process-agent.log")
	}
	if _here, err := executable.Folder(); err == nil {
		agentFilePath := filepath.Join(_here, "..", "..", "embedded", "agent.exe")
		if _, err := os.Stat(agentFilePath); err == nil {
			defaultDDAgentBin = agentFilePath
		}
	}
}
