// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
)

type agentHostExecutor struct {
	baseCommand string
	host        *components.RemoteHost
}

func newDefaultAgentHostExecutor(host *components.RemoteHost) agentCommandExecutor {
	var baseCommand string
	switch host.OSFamily {
	case os.WindowsFamily:
		baseCommand = agentHostBaseCommandWithInstallPath(host, defaultWindowsAgentInstallPath(host))
	case os.LinuxFamily:
		baseCommand = "sudo datadog-agent"
	case os.MacOSFamily:
		baseCommand = "datadog-agent"
	default:
		panic(fmt.Sprintf("unsupported OS family: %v", host.OSFamily))
	}

	return newAgentHostExecutor(host, baseCommand)
}

func newAgentHostExecutor(host *components.RemoteHost, baseCommand string) agentCommandExecutor {
	return &agentHostExecutor{
		baseCommand: baseCommand,
		host:        host,
	}
}

func (ae agentHostExecutor) execute(arguments []string) (string, error) {
	parameters := ""
	if len(arguments) > 0 {
		parameters = `"` + strings.Join(arguments, `" "`) + `"`
	}

	return ae.host.Execute(ae.baseCommand + " " + parameters)
}

func agentHostBaseCommandWithInstallPath(host *components.RemoteHost, installPath string) string {
	var baseCommand string
	switch host.OSFamily {
	case os.WindowsFamily:
		baseCommand = fmt.Sprintf(`& "%s\bin\agent.exe"`, installPath)
	default:
		panic(fmt.Sprintf("OS family %v does not support custom install paths", host.OSFamily))
	}
	return baseCommand
}

func defaultWindowsAgentInstallPath(host *components.RemoteHost) string {
	// If the agent is installed get the path from the registry, fallback to default path
	path, err := windowsAgent.GetInstallPathFromRegistry(host)
	if err != nil {
		path = windowsAgent.DefaultInstallPath
	}
	return path
}
