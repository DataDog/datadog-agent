// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	wincommand "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/command"
)

type agentHostExecutor struct {
	baseCommand string
	host        *Host
}

func newAgentHostExecutor(osFamily os.Family, host *Host, params *agentclientparams.Params) agentCommandExecutor {
	var baseCommand string
	switch osFamily {
	case os.WindowsFamily:
		installPath := params.AgentInstallPath
		if len(installPath) == 0 {
			installPath = defaultWindowsAgentInstallPath(host)
		}
		fmt.Printf("Using default install path: %s\n", installPath)
		baseCommand = fmt.Sprintf(`& "%s\bin\agent.exe"`, installPath)
	case os.LinuxFamily:
		baseCommand = "sudo datadog-agent"
	case os.MacOSFamily:
		baseCommand = "datadog-agent"
	default:
		panic(fmt.Sprintf("unsupported OS family: %v", osFamily))
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
// If the Agent is installed, the installPath is read from the registry.
// If the registry key is not found, returns the default install path.
func defaultWindowsAgentInstallPath(host *Host) string {
	path, err := host.Execute(wincommand.GetInstallPathFromRegistry())
	if err != nil {
		path = wincommand.DefaultInstallPath
	}
	return strings.TrimSpace(path)
}
