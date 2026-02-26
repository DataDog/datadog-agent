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
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
	slogHandlers "github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// AnnotationHumanReadableErrors is the annotation key for commands that should
	// display errors in human-readable format instead of JSON.
	//
	// For example, `setup` is run by humans and its output should be human readable.
	AnnotationHumanReadableErrors = "human-readable-errors"
)

type cmd struct {
	t              *telemetry.Telemetry
	span           *telemetry.Span
	ctx            context.Context
	env            *env.Env
	stopSigHandler context.CancelFunc
}

func setupStdoutLogger(_ *env.Env) {
	level := "warn"
	if envLevel, found := os.LookupEnv("DD_LOG_LEVEL"); found && envLevel != "" {
		level = envLevel
	}
	formatter := func(_ context.Context, r slog.Record) string {
		return r.Message + "\n"
	}
	handler := slogHandlers.NewFormat(formatter, os.Stdout)
	loggerInterface := pkglogslog.NewWrapper(handler)
	pkglog.SetupLogger(loggerInterface, level)
}

// newCmd creates a new command
func newCmd(operation string) *cmd {
	env := env.FromEnv()
	if !env.IsFromDaemon {
		setupStdoutLogger(env)
	}
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
		i, err = installer.NewInstaller(cmd.ctx, cmd.env)
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
	configPath := filepath.Join(paths.AgentConfigDir, "datadog.yaml")
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
	_, ddSiteSet := os.LookupEnv("DD_SITE")
	if !ddSiteSet && config.Site != "" {
		site = config.Site
	}

	// Update env fields with corrected values so subprocesses inherit the right config
	env.APIKey = apiKey
	env.Site = site

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
		extensionsCommands(),
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
		Annotations: map[string]string{
			AnnotationHumanReadableErrors: "true",
		},
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
		Use:     "install-config-experiment <package> <operations>",
		Short:   "Install a config experiment",
		GroupID: "installer",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("install_config_experiment")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.package", args[0])

			// Parse operations from command-line argument
			var operations config.Operations
			err = json.Unmarshal([]byte(args[1]), &operations)
			if err != nil {
				return fmt.Errorf("could not decode operations: %w", err)
			}

			// Read decrypted secrets from stdin
			// For backwards compatibility, accept empty stdin (no secrets)
			var decryptedSecrets map[string]string
			decoder := json.NewDecoder(os.Stdin)
			err = decoder.Decode(&decryptedSecrets)
			if err != nil && err != io.EOF {
				return fmt.Errorf("could not decode secrets from stdin: %w", err)
			}
			// If stdin is empty or EOF, use empty map
			if decryptedSecrets == nil {
				decryptedSecrets = make(map[string]string)
			}

			i.span.SetTag("params.deployment_id", operations.DeploymentID)
			i.span.SetTag("params.operations", operations.FileOperations)
			return i.InstallConfigExperiment(i.ctx, args[0], operations, decryptedSecrets)
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
	return i.ConfigAndPackageStates(i.ctx)
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

// extensionsCommands are the extensions installer commands
func extensionsCommands() *cobra.Command {
	ctlCmd := &cobra.Command{
		Use:     "extension [command]",
		Short:   "Interact with the extensions of a package",
		GroupID: "extension",
	}
	ctlCmd.AddCommand(extensionInstallCommand(), extensionRemoveCommand(), extensionSaveCommand(), extensionRestoreCommand())
	return ctlCmd
}

func extensionInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [url] [extensions...]",
		Short: "Install one or more extensions for a package",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("extension_install")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.url", args[0])
			i.span.SetTag("params.extensions", strings.Join(args[1:], ","))
			return i.InstallExtensions(i.ctx, args[0], args[1:])
		},
	}
	return cmd
}

func extensionRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [package] [extensions...]",
		Short: "Remove one or more extensions for a package",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("extension_remove")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.package", args[0])
			i.span.SetTag("params.extensions", strings.Join(args[1:], ","))
			return i.RemoveExtensions(i.ctx, args[0], args[1:])
		},
	}
	return cmd
}

func extensionSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "save [package] [path]",
		Short:  "Save the extensions for a package",
		Args:   cobra.ExactArgs(2),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("extension_save")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.package", args[0])
			i.span.SetTag("params.path", args[1])
			return i.SaveExtensions(i.ctx, args[0], args[1])
		},
	}
	return cmd
}

func extensionRestoreCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "restore [package] [path]",
		Short:  "Restore the extensions for a package",
		Args:   cobra.ExactArgs(2),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("extension_restore")
			if err != nil {
				return err
			}
			defer func() { i.stop(err) }()
			i.span.SetTag("params.package", args[0])
			i.span.SetTag("params.path", args[1])
			return i.RestoreExtensions(i.ctx, args[0], args[1])
		},
	}
	return cmd
}
