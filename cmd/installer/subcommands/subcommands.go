// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the installer subcommands
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/bootstrap"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/experiment"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/purge"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/run"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/status"
)

// InstallerSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func InstallerSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		run.Commands,
		bootstrap.Commands,
		status.Commands,
		experiment.Commands,
		purge.Commands,
	}
}
