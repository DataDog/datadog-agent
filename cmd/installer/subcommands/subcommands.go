// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the installer subcommands
package subcommands

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/bootstrap"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/daemon"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/installer"
	"github.com/DataDog/datadog-agent/cmd/installer/user"
	"github.com/spf13/cobra"
)

// InstallerSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func InstallerSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		withDatadogAgent(daemon.Commands),
		withRoot(bootstrap.Commands),
		withRoot(installer.Commands),
	}
}

func withRoot(factory command.SubcommandFactory) command.SubcommandFactory {
	return withPersistentPreRunE(factory, func(global *command.GlobalParams) error {
		if !user.IsRoot() && global.AllowNoRoot {
			return nil
		}
		if !user.IsRoot() {
			return fmt.Errorf("this command requires root privileges")
		}
		return user.DatadogAgentToRoot()
	})
}

func withDatadogAgent(factory command.SubcommandFactory) command.SubcommandFactory {
	return withPersistentPreRunE(factory, func(global *command.GlobalParams) error {
		if !user.IsRoot() && global.AllowNoRoot {
			return nil
		}
		if !user.IsRoot() {
			return fmt.Errorf("this command requires root privileges")
		}
		return user.RootToDatadogAgent()
	})
}

func withPersistentPreRunE(factory command.SubcommandFactory, f func(*command.GlobalParams) error) command.SubcommandFactory {
	return func(global *command.GlobalParams) []*cobra.Command {
		commands := factory(global)
		for _, cmd := range commands {
			cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
				return f(global)
			}
		}
		return commands
	}
}
