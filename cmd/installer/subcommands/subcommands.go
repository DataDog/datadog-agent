// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the installer subcommands
package subcommands

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/daemon"
	"github.com/DataDog/datadog-agent/cmd/installer/user"
	installer "github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
)

// installerCommands returns the installer subcommands.
func installerCommands(_ *command.GlobalParams) []*cobra.Command {
	return installer.RootCommands()
}

// installerUnprivilegedCommands returns the unprivileged installer subcommands.
func installerUnprivilegedCommands(_ *command.GlobalParams) []*cobra.Command {
	return installer.UnprivilegedCommands()
}

func withRoot(factory command.SubcommandFactory) command.SubcommandFactory {
	return withPersistentPreRunE(factory, func(global *command.GlobalParams) error {
		if !user.IsRoot() && global.AllowNoRoot {
			return nil
		}
		if !user.IsRoot() {
			return user.ErrRootRequired
		}
		return user.DatadogAgentToRoot()
	})
}

// withDatadogAgent wraps a command factory to downgrade the running user to dd-agent
func withDatadogAgent(factory command.SubcommandFactory) command.SubcommandFactory {
	return withPersistentPreRunE(factory, func(global *command.GlobalParams) error {
		if !user.IsRoot() && global.AllowNoRoot {
			return nil
		}
		if !user.IsRoot() {
			return user.ErrRootRequired
		}
		return user.RootToDatadogAgent()
	})
}

func withPersistentPreRunE(factory command.SubcommandFactory, f func(*command.GlobalParams) error) command.SubcommandFactory {
	return func(global *command.GlobalParams) []*cobra.Command {
		commands := factory(global)
		for _, cmd := range commands {
			cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
				return f(global)
			}
		}
		return commands
	}
}

// InstallerSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func InstallerSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		installerUnprivilegedCommands,
		withRoot(installerCommands),
		withDatadogAgent(daemon.Commands),
	}
}
