// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks
// +build !windows,clusterchecks

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/command"
	cmdflare "github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/subcommands/flare"
	cmdrun "github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/subcommands/run"
)

// ClusterAgentSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func ClusterAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdflare.Commands,
		cmdrun.Commands,
	}
}
