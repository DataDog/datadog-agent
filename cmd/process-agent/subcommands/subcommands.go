// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	cmdevents "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/events"
)

// ProcessAgentSubcommands returns SubcommandFactories for the subcommands in the Process Agent
func ProcessAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdevents.Commands,
	}
}
