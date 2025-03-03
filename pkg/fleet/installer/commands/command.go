// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package commands contains the installer subcommands
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	mockInstaller installer.Installer
)

// Cmd is the base command
type Cmd struct {
	// Span is the current span for the command
	Span *telemetry.Span
	// Ctx is the current context for the command
	Ctx context.Context
	// Env is the current environment for the command
	Env *env.Env

	t *telemetry.Telemetry
}

// NewCmd creates a new command
func NewCmd(operation string) *Cmd {
	env := env.FromEnv()
	t := newTelemetry(env)
	span, ctx := telemetry.StartSpanFromEnv(context.Background(), operation)
	setInstallerUmask(span)
	return &Cmd{
		t:    t,
		Ctx:  ctx,
		Span: span,
		Env:  env,
	}
}

// Stop stops the command
func (c *Cmd) Stop(err error) {
	c.Span.Finish(err)
	if c.t != nil {
		c.t.Stop()
	}
}

type installerCmd struct {
	*Cmd
	installer.Installer
}

func newInstallerCmd(operation string) (_ *installerCmd, err error) {
	cmd := NewCmd(operation)
	defer func() {
		if err != nil {
			cmd.Stop(err)
		}
	}()
	var i installer.Installer
	if mockInstaller != nil {
		i = mockInstaller
	} else {
		i, err = installer.NewInstaller(cmd.Env)
	}
	if err != nil {
		return nil, err
	}
	return &installerCmd{
		Installer: i,
		Cmd:       cmd,
	}, nil
}

func (i *installerCmd) stop(err error) {
	i.Cmd.Stop(err)
	err = i.Installer.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to close Installer: %v\n", err)
	}
}

type telemetryConfigFields struct {
	APIKey string `yaml:"api_key"`
	Site   string `yaml:"site"`
}

// telemetryConfig is a best effort to get the API key / site from `datadog.yaml`.
func telemetryConfig() telemetryConfigFields {
	configPath := "/etc/datadog-agent/datadog.yaml"
	if runtime.GOOS == "windows" {
		configPath = "C:\\ProgramData\\Datadog\\datadog.yaml"
	}
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return telemetryConfigFields{}
	}
	var config telemetryConfigFields
	err = yaml.Unmarshal(rawConfig, &config)
	if err != nil {
		return telemetryConfigFields{}
	}
	return config
}

func newTelemetry(env *env.Env) *telemetry.Telemetry {
	config := telemetryConfig()
	apiKey := env.APIKey
	if apiKey == "" {
		apiKey = config.APIKey
	}
	site := env.Site
	if site == "" {
		site = config.Site
	}
	t := telemetry.NewTelemetry(env.HTTPClient(), apiKey, site, "datadog-installer") // No sampling rules for commands
	return t
}

// RootCommands returns the root commands
func RootCommands() []*cobra.Command {
	return []*cobra.Command{
		InstallCommand(),
		SetupCommand(),
		BootstrapCommand(),
		RemoveCommand(),
		InstallExperimentCommand(),
		RemoveExperimentCommand(),
		PromoteExperimentCommand(),
		InstallConfigExperimentCommand(),
		RemoveConfigExperimentCommand(),
		PromoteConfigExperimentCommand(),
		GarbageCollectCommand(),
		PurgeCommand(),
		IsInstalledCommand(),
		ApmCommands(),
		GetStateCommand(),
	}
}

// UnprivilegedCommands returns the unprivileged commands
func UnprivilegedCommands() []*cobra.Command {
	return []*cobra.Command{
		VersionCommand(),
		DefaultPackagesCommand(),
	}
}

// VersionCommand is the command to print the version
func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the version of the installer",
		GroupID: "installer",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(version.AgentVersion)
		},
	}
}

// DefaultPackagesCommand is the commanf to list the default packages to install
func DefaultPackagesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "default-packages",
		Short:   "Print the list of default packages to install",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) error {
			defaultPackages := installer.DefaultPackages(env.FromEnv())
			fmt.Fprintf(os.Stdout, "%s\n", strings.Join(defaultPackages, "\n"))
			return nil
		},
	}
}

// SetupCommand is the command to setup the installer
func SetupCommand() *cobra.Command {
	flavor := ""
	cmd := &cobra.Command{
		Use:     "setup",
		Hidden:  true,
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			cmd := NewCmd("setup")
			defer func() { cmd.Stop(err) }()
			if flavor == "" {
				return setup.Agent7InstallScript(cmd.Ctx, cmd.Env)
			}
			return setup.Setup(cmd.Ctx, cmd.Env, flavor)
		},
	}
	cmd.Flags().StringVar(&flavor, "flavor", "", "The setup flavor")
	return cmd
}

// InstallCommand is the command to install a package
func InstallCommand() *cobra.Command {
	var installArgs []string
	var forceInstall bool
	cmd := &cobra.Command{
		Use:     "install <url>",
		Short:   "Install a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("install")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.url", args[0])
			if forceInstall {
				return i.ForceInstall(i.Ctx, args[0], installArgs)
			}
			return i.Install(i.Ctx, args[0], installArgs)
		},
	}
	cmd.Flags().StringArrayVarP(&installArgs, "install_args", "A", nil, "Arguments to pass to the package")
	cmd.Flags().BoolVar(&forceInstall, "force", false, "Install packages, even if they are already up-to-date.")
	return cmd
}

// RemoveCommand is the command to remove a package
func RemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <package>",
		Short:   "Remove a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("remove")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			return i.Remove(i.Ctx, args[0])
		},
	}
	return cmd
}

// PurgeCommand is the command to purge all packages installed with the installer
func PurgeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Purge all packages installed with the installer",
		GroupID: "installer",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("purge")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Purge(i.Ctx)
			return nil
		},
	}
	return cmd
}

// InstallExperimentCommand is the command to install an experiment
func InstallExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install-experiment <url>",
		Short:   "Install an experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("install_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.url", args[0])
			return i.InstallExperiment(i.Ctx, args[0])
		},
	}
	return cmd
}

// RemoveExperimentCommand is the command to remove an experiment
func RemoveExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove-experiment <package>",
		Short:   "Remove an experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("remove_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			return i.RemoveExperiment(i.Ctx, args[0])
		},
	}
	return cmd
}

// PromoteExperimentCommand is the command to promote an experiment
func PromoteExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "promote-experiment <package>",
		Short:   "Promote an experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("promote_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			return i.PromoteExperiment(i.Ctx, args[0])
		},
	}
	return cmd
}

// InstallConfigExperimentCommand is the command to install a config experiment
func InstallConfigExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install-config-experiment <package> <version> <config>",
		Short:   "Install a config experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("install_config_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			i.Span.SetTag("params.version", args[1])
			return i.InstallConfigExperiment(i.Ctx, args[0], args[1], []byte(args[2]))
		},
	}
	return cmd
}

// RemoveConfigExperimentCommand is the command to remove a config experiment
func RemoveConfigExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove-config-experiment <package>",
		Short:   "Remove a config experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("remove_config_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			return i.RemoveConfigExperiment(i.Ctx, args[0])
		},
	}
	return cmd
}

// PromoteConfigExperimentCommand is the command to promote a config experiment
func PromoteConfigExperimentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "promote-config-experiment <package>",
		Short:   "Promote a config experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("promote_config_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.Span.SetTag("params.package", args[0])
			return i.PromoteConfigExperiment(i.Ctx, args[0])
		},
	}
	return cmd
}

// GarbageCollectCommand is the command to remove unused packages
func GarbageCollectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "garbage-collect",
		Short:   "Remove unused packages",
		GroupID: "installer",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("garbage_collect")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			return i.GarbageCollect(i.Ctx)
		},
	}
	return cmd
}

const (
	// ReturnCodeIsInstalledFalse is the return code when a package is not installed
	ReturnCodeIsInstalledFalse = 10
)

// IsInstalledCommand is the command to check if a package is installed
func IsInstalledCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "is-installed <package>",
		Short:   "Check if a package is installed",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("is_installed")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			installed, err := i.IsInstalled(i.Ctx, args[0])
			if err != nil {
				return err
			}
			if !installed {
				// Return a specific code to differentiate from other errors
				// `return err` will lead to a return code of -1
				os.Exit(ReturnCodeIsInstalledFalse)
			}
			return nil
		},
	}
	return cmd
}

// GetStateCommand is the command to get the package & config states
func GetStateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "get-states",
		Short:   "Get the package & config states",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("get_states")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			states, err := i.States(i.Ctx)
			if err != nil {
				return err
			}
			configStates, err := i.ConfigStates(i.Ctx)
			if err != nil {
				return err
			}

			pStates := repository.PackageStates{
				States:       states,
				ConfigStates: configStates,
			}

			pStatesRaw, err := json.Marshal(pStates)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "%s\n", pStatesRaw)
			return nil
		},
	}
	return cmd
}
