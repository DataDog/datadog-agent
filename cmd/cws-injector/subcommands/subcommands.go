// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/cws-injector/command"
	"github.com/DataDog/datadog-agent/cmd/cws-injector/subcommands/inject"
	"github.com/DataDog/datadog-agent/cmd/cws-injector/subcommands/setup"
)

// CWSInjectorSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func CWSInjectorSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		setup.Command,
		inject.Command,
	}
}
