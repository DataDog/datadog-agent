// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
)

type agentHostExecutor struct {
	baseCommand string
	host        *components.RemoteHost
}

func newAgentHostExecutor(host *components.RemoteHost, params *agentclientparams.Params) agentCommandExecutor {
	var baseCommand string
	switch host.OSFamily {
	case os.WindowsFamily:
		installPath := params.AgentInstallPath
		if len(installPath) == 0 {
			installPath = defaultWindowsAgentInstallPath(host)
		}
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

// defaultWindowsAgentInstallPath returns a reasonable default for the AgentInstallPath.
//
// If the AgentInstallPath is not provided, it will attempt to read the install path from the registry.
// If the registry key is not found, it will return the default install path.
func defaultWindowsAgentInstallPath(host *components.RemoteHost) string {
	path, err := windowsAgent.GetInstallPathFromRegistry(host)
	if err != nil {
		path = windowsAgent.DefaultInstallPath
	}
	return path
}
