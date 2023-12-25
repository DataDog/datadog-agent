// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package subcommands is used to list the subcommands of CWS profiler
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/profiler/command"
	"github.com/DataDog/datadog-agent/cmd/profiler/subcommands/startcmd"
)

// CWSProfilerSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func CWSProfilerSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		startcmd.Command,
	}
}
