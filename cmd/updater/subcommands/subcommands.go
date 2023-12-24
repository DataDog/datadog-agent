// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the updater subcommands
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/cmd/updater/subcommands/bootstrap"
	"github.com/DataDog/datadog-agent/cmd/updater/subcommands/run"
)

// UpdaterSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func UpdaterSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		run.Commands,
		bootstrap.Commands,
	}
}
