// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package subcommands is used to list the subcommands of CWS injector
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/command"
	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/injectcmd"
	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/setupcmd"
	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/tracecmd"
)

// CWSInjectorSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func CWSInjectorSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		setupcmd.Command,
		injectcmd.Command,
		tracecmd.Command,
	}
}
