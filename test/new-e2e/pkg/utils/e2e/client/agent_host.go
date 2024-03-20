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

func newAgentHostExecutor(host *components.RemoteHost) agentCommandExecutor {
	var err error
	var path string
	switch host.OSFamily {
	case os.WindowsFamily:
		// If the agent is installed get the path from the registry, fallback to default path
		path, err = windowsAgent.GetInstallPathFromRegistry(host)
		if err != nil {
			path = windowsAgent.DefaultInstallPath
		}
	}
	return newAgentHostExecutorWithInstallPath(host, path)
}

func newAgentHostExecutorWithInstallPath(host *components.RemoteHost, installPath string) agentCommandExecutor {
	var baseCommand string
	switch host.OSFamily {
	case os.WindowsFamily:
		baseCommand = fmt.Sprintf(`& "%s\bin\agent.exe"`, installPath)
	case os.LinuxFamily:
		baseCommand = "sudo datadog-agent"
	case os.MacOSFamily:
		baseCommand = "datadog-agent"
	default:
		panic(fmt.Sprintf("unsupported OS family: %v", host.OSFamily))
	}

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
