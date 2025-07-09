// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the subcommands for system-probe
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/config"
	cmdcoverage "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/coverage"
	cmddebug "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/debug"
	cmdmodrestart "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/modrestart"
	cmdrun "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"
	cmdruntime "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime"
	cmdversion "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/version"
)

// SysprobeSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func SysprobeSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdrun.Commands,
		cmdversion.Commands,
		cmdmodrestart.Commands,
		cmddebug.Commands,
		cmdconfig.Commands,
		cmdruntime.Commands,
		cmdcoverage.Commands,
	}
}
