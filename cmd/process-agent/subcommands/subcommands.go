// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	cmdcheck "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/check"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/config"
	cmdevents "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/events"
	cmdstatus "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/status"
	cmdtaggerlist "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/taggerlist"
	cmdversion "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/workloadlist"
)

// ProcessAgentSubcommands returns SubcommandFactories for the subcommands in the Process Agent
func ProcessAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdcheck.Commands,
		cmdconfig.Commands,
		cmdevents.Commands,
		cmdstatus.Commands,
		cmdtaggerlist.Commands,
		cmdversion.Commands,
		cmdworkloadlist.Commands,
	}
}
