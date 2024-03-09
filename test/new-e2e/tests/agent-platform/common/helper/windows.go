// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// Windows implement helper function for Windows distributions
type Windows struct {
	installFolder string
	configFolder  string
}

var _ Helper = &Windows{}

// NewWindowsHelper create a new instance of Windows helper
func NewWindowsHelper() *Windows {
	return NewWindowsHelperWithCustomPaths(windowsAgent.DefaultInstallPath, windowsAgent.DefaultConfigRoot)
}

// NewWindowsHelperWithCustomPaths create a new instance of Windows helper with custom paths
func NewWindowsHelperWithCustomPaths(installFolder, configFolder string) *Windows {
	return &Windows{
		installFolder: installFolder,
		configFolder:  configFolder,
	}
}

// GetInstallFolder return the install folder path
func (w *Windows) GetInstallFolder() string { return w.installFolder + "\\" }

// GetConfigFolder return the config folder path
func (w *Windows) GetConfigFolder() string { return w.configFolder + "\\" }

// GetBinaryPath return the datadog-agent binary path
func (w *Windows) GetBinaryPath() string { return w.GetInstallFolder() + `bin\agent.exe` }

// GetConfigFileName return the config file name
func (w *Windows) GetConfigFileName() string { return "datadog.yaml" }

// GetServiceName return the service name
func (w *Windows) GetServiceName() string { return "datadogagent" }

// AgentProcesses return the list of agent processes
func (w *Windows) AgentProcesses() []string {
	return []string{
		"agent.exe",
		"process-agent.exe",
		"trace-agent.exe",
		"security-agent.exe",
		"system-probe.exe",
		"dogstatsd.exe",
	}
}
