// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	cmdcontrolsvc "github.com/DataDog/datadog-agent/cmd/agent/subcommands/controlsvc"
	cmdinstallsvc "github.com/DataDog/datadog-agent/cmd/agent/subcommands/installsvc"
	cmdremovesvc "github.com/DataDog/datadog-agent/cmd/agent/subcommands/removesvc"
)

// windowsSubcommands returns SubcommandFactories for subcommands dependent on the `windows` build tag.
func windowsSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdcontrolsvc.Commands,
		cmdinstallsvc.Commands,
		cmdremovesvc.Commands,
	}
}
