// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	cmdversion "github.com/DataDog/datadog-agent/cmd/process-agent/app/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/process-agent/app/subcommands/workloadlist"
)

// ProcessAgentSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func ProcessAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdversion.Commands,
		cmdworkloadlist.Commands,
	}
}
