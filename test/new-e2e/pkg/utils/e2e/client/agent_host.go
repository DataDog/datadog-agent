// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/components/os"
)

type agentHostExecutor struct {
	baseCommand string
	host        *components.RemoteHost
}

func newAgentHostExecutor(host *components.RemoteHost) agentCommandExecutor {
	var baseCommand string
	switch host.OSFamily {
	case os.WindowsFamily:
		baseCommand = `& "$env:ProgramFiles\Datadog\Datadog Agent\bin\agent.exe"`
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
