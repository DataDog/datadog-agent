// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands implement security agent subcommands
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	cmdcompliance "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/compliance"
	cmdruntime "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/runtime"
	cmdstart "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/start"
)

// SecurityAgentSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags. The build tags in use right now are
// !windows && kubeapiserver (check, and any parent command that uses check),
// kubeapiserver (config),
// and linux (runtime).
func SecurityAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdcompliance.Commands,
		cmdruntime.Commands,
		cmdstart.Commands,
	}
}
