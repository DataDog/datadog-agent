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
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cmd struct {
	t              *telemetry.Telemetry
	span           *telemetry.Span
	ctx            context.Context
	env            *env.Env
	stopSigHandler context.CancelFunc
}

// newCmd creates a new command
func newCmd(operation string) *cmd {
	env := env.FromEnv()
	t := newTelemetry(env)
	span, ctx := telemetry.StartSpanFromEnv(context.Background(), operation)
	ctx, stop := context.WithCancel(ctx)
	handleSignals(ctx, stop)
	setInstallerUmask(span)
	return &cmd{
		t:              t,
		ctx:            ctx,
		span:           span,
		env:            env,
		stopSigHandler: stop,
	}
}

func handleSignals(ctx context.Context, stop context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-sigChan:
			// Wait for 10 seconds to allow the command to finish properly
			time.Sleep(10 * time.Second)
			stop()
		}
	}()
}

// Stop stops the command
func (c *cmd) stop(err error) {
	c.span.Finish(err)
	if c.t != nil {
		c.t.Stop()
	}
	c.stopSigHandler()
}

type installerCmd struct {
	*cmd
	installer.Installer
}

func newInstallerCmd(operation string) (_ *installerCmd, err error) {
	cmd := newCmd(operation)
	defer func() {
		if err != nil {
			cmd.stop(err)
		}
	}()
	var i installer.Installer
	if MockInstaller != nil {
		i = MockInstaller
	} else {
		i, err = installer.NewInstaller(cmd.env)
	}
	if err != nil {
		return nil, err
	}
	return &installerCmd{
		Installer: i,
		cmd:       cmd,
	}, nil
}

func (i *installerCmd) stop(err error) {
	i.cmd.stop(err)
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
	if _, set := os.LookupEnv("DD_SITE"); !set && config.Site != "" {
		site = config.Site
	}
	t := telemetry.NewTelemetry(env.HTTPClient(), apiKey, site, "datadog-installer") // No sampling rules for commands
	return t
}

// RootCommands returns the root commands
func RootCommands() []*cobra.Command {
	return []*cobra.Command{
		installCommand(),
		setupCommand(),
		setupInstallerCommand(),
		bootstrapCommand(),
		removeCommand(),
		installExperimentCommand(),
		removeExperimentCommand(),
		promoteExperimentCommand(),
		installConfigExperimentCommand(),
		removeConfigExperimentCommand(),
		promoteConfigExperimentCommand(),
		garbageCollectCommand(),
		purgeCommand(),
		isInstalledCommand(),
		apmCommands(),
		getStateCommand(),
		statusCommand(),
		postinstCommand(),
		isPrermSupportedCommand(),
		prermCommand(),
		hooksCommand(),
		packageCommand(),
	}
}

// UnprivilegedCommands returns the unprivileged commands
func UnprivilegedCommands() []*cobra.Command {
	return []*cobra.Command{
		versionCommand(),
		defaultPackagesCommand(),
	}
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the version of the installer",
		GroupID: "installer",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(version.AgentVersion)
		},
	}
}

func defaultPackagesCommand() *cobra.Command {
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

func setupCommand() *cobra.Command {
	flavor := ""
	cmd := &cobra.Command{
		Use:     "setup",
		Hidden:  true,
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			cmd := newCmd("setup")
			defer func() { cmd.stop(err) }()
			if flavor == "" {
				return setup.Agent7InstallScript(cmd.ctx, cmd.env)
			}
			return setup.Setup(cmd.ctx, cmd.env, flavor)
		},
	}
	cmd.Flags().StringVar(&flavor, "flavor", "", "The setup flavor")
	return cmd
}

func installCommand() *cobra.Command {
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
			i.span.SetTag("params.url", args[0])
			if forceInstall {
				return i.ForceInstall(i.ctx, args[0], installArgs)
			}
			return i.Install(i.ctx, args[0], installArgs)
		},
	}
	cmd.Flags().StringArrayVarP(&installArgs, "install_args", "A", nil, "Arguments to pass to the package")
	cmd.Flags().BoolVar(&forceInstall, "force", false, "Install packages, even if they are already up-to-date.")
	return cmd
}

func setupInstallerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "setup-installer <stablePath>",
		Short:   "Sets up the installer package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("setup_installer")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.stablePath", args[0])
			return i.SetupInstaller(i.ctx, args[0])
		},
	}
	return cmd
}

func removeCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			return i.Remove(i.ctx, args[0])
		},
	}
	return cmd
}

func purgeCommand() *cobra.Command {
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
			i.Purge(i.ctx)
			return nil
		},
	}
	return cmd
}

func installExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.url", args[0])
			return i.InstallExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func removeExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			return i.RemoveExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func promoteExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			return i.PromoteExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func installConfigExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			i.span.SetTag("params.version", args[1])
			return i.InstallConfigExperiment(i.ctx, args[0], args[1], []byte(args[2]))
		},
	}
	return cmd
}

func removeConfigExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			return i.RemoveConfigExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func promoteConfigExperimentCommand() *cobra.Command {
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
			i.span.SetTag("params.package", args[0])
			return i.PromoteConfigExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func garbageCollectCommand() *cobra.Command {
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
			return i.GarbageCollect(i.ctx)
		},
	}
	return cmd
}

const (
	// ReturnCodeIsInstalledFalse is the return code when a package is not installed
	ReturnCodeIsInstalledFalse = 10
)

func isInstalledCommand() *cobra.Command {
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
			installed, err := i.IsInstalled(i.ctx, args[0])
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

func getState() (*repository.PackageStates, error) {
	i, err := newInstallerCmd("get_states")
	if err != nil {
		return nil, err
	}
	defer i.stop(err)
	states, err := i.States(i.ctx)
	if err != nil {
		return nil, err
	}
	configStates, err := i.ConfigStates(i.ctx)
	if err != nil {
		return nil, err
	}
	return &repository.PackageStates{
		States:       states,
		ConfigStates: configStates,
	}, nil
}

func getStateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "get-states",
		Short:   "Get the package & config states",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			pStates, err := getState()
			if err != nil {
				return
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

// packageCommand runs a package-specific command
func packageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		GroupID: "installer",
		Use:     "package-command <package> <command>",
		Short:   "Run a package-specific command",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("package_command")
			defer i.stop(err)

			packageName := args[0]
			command := args[1]
			i.span.SetTag("params.package", packageName)
			i.span.SetTag("params.command", command)

			return packages.RunPackageCommand(i.ctx, packageName, command)
		},
	}

	return cmd
}
