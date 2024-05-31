// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains the installer subcommands
package installer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/fleet/bootstraper"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/spf13/cobra"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	envUpgrade                          = "DD_UPGRADE"
	envAPMInstrumentationNoConfigChange = "DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE"
	envSystemProbeEnsureConfig          = "DD_SYSTEM_PROBE_ENSURE_CONFIG"
	envRuntimeSecurityConfigEnabled     = "DD_RUNTIME_SECURITY_CONFIG_ENABLED"
	envComplianceConfigEnabled          = "DD_COMPLIANCE_CONFIG_ENABLED"
	envInstallOnly                      = "DD_INSTALL_ONLY"
	envNoAgentInstall                   = "DD_NO_AGENT_INSTALL"
	envAPMInstrumentationLibraries      = "DD_APM_INSTRUMENTATION_LIBRARIES"
	envAppSecEnabled                    = "DD_APPSEC_ENABLED"
	envIASTEnabled                      = "DD_IAST_ENABLED"
	envAPMInstrumentationEnabled        = "DD_APM_INSTRUMENTATION_ENABLED"
	envRepoURL                          = "DD_REPO_URL"
	envRepoURLDeprecated                = "REPO_URL"
	envRPMRepoGPGCheck                  = "DD_RPM_REPO_GPGCHECK"
	envAgentMajorVersion                = "DD_AGENT_MAJOR_VERSION"
	envAgentMinorVersion                = "DD_AGENT_MINOR_VERSION"
	envAgentDistChannel                 = "DD_AGENT_DIST_CHANNEL"
)

// Commands returns the installer subcommands.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	return []*cobra.Command{bootstrapCommand(), installCommand(), removeCommand(), installExperimentCommand(), removeExperimentCommand(), promoteExperimentCommand(), garbageCollectCommand(), purgeCommand(), isInstalledCommand()}
}

// UnprivilegedCommands returns the unprivileged installer subcommands.
func UnprivilegedCommands(_ *command.GlobalParams) []*cobra.Command {
	return []*cobra.Command{versionCommand(), defaultPackagesCommand()}
}

type cmd struct {
	t    *telemetry.Telemetry
	ctx  context.Context
	span ddtrace.Span
	env  *env.Env
}

func newCmd(operation string) *cmd {
	env := env.FromEnv()
	t := newTelemetry(env)
	span, ctx := newSpan(operation)
	return &cmd{
		t:    t,
		ctx:  ctx,
		span: span,
		env:  env,
	}
}

func (c *cmd) Stop(err error) {
	c.span.Finish(tracer.WithError(err))
	if c.t != nil {
		err := c.t.Stop(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop telemetry: %v\n", err)
		}
	}
}

type installerCmd struct {
	*cmd
	installer.Installer
}

func newInstallerCmd(operation string) (_ *installerCmd, err error) {
	cmd := newCmd(operation)
	defer func() {
		if err != nil {
			cmd.Stop(err)
		}
	}()
	i, err := installer.NewInstaller(cmd.env)
	if err != nil {
		return nil, err
	}
	return &installerCmd{
		Installer: i,
		cmd:       cmd,
	}, nil
}

type bootstraperCmd struct {
	*cmd
}

func newBootstraperCmd(operation string) *bootstraperCmd {
	cmd := newCmd(operation)
	cmd.span.SetTag("env.DD_UPGRADE", os.Getenv(envUpgrade))
	cmd.span.SetTag("env.DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE", os.Getenv(envAPMInstrumentationNoConfigChange))
	cmd.span.SetTag("env.DD_SYSTEM_PROBE_ENSURE_CONFIG", os.Getenv(envSystemProbeEnsureConfig))
	cmd.span.SetTag("env.DD_RUNTIME_SECURITY_CONFIG_ENABLED", os.Getenv(envRuntimeSecurityConfigEnabled))
	cmd.span.SetTag("env.DD_COMPLIANCE_CONFIG_ENABLED", os.Getenv(envComplianceConfigEnabled))
	cmd.span.SetTag("env.DD_INSTALL_ONLY", os.Getenv(envInstallOnly))
	cmd.span.SetTag("env.DD_NO_AGENT_INSTALL", os.Getenv(envNoAgentInstall))
	cmd.span.SetTag("env.DD_APM_INSTRUMENTATION_LIBRARIES", os.Getenv(envAPMInstrumentationLibraries))
	cmd.span.SetTag("env.DD_APPSEC_ENABLED", os.Getenv(envAppSecEnabled))
	cmd.span.SetTag("env.DD_IAST_ENABLED", os.Getenv(envIASTEnabled))
	cmd.span.SetTag("env.DD_APM_INSTRUMENTATION_ENABLED", os.Getenv(envAPMInstrumentationEnabled))
	cmd.span.SetTag("env.DD_REPO_URL", os.Getenv(envRepoURL))
	cmd.span.SetTag("env.REPO_URL", os.Getenv(envRepoURLDeprecated))
	cmd.span.SetTag("env.DD_RPM_REPO_GPGCHECK", os.Getenv(envRPMRepoGPGCheck))
	cmd.span.SetTag("env.DD_AGENT_MAJOR_VERSION", os.Getenv(envAgentMajorVersion))
	cmd.span.SetTag("env.DD_AGENT_MINOR_VERSION", os.Getenv(envAgentMinorVersion))
	return &bootstraperCmd{
		cmd: cmd,
	}
}

func newTelemetry(env *env.Env) *telemetry.Telemetry {
	if env.APIKey == "" {
		fmt.Printf("telemetry disabled: missing DD_API_KEY\n")
		return nil
	}
	t, err := telemetry.NewTelemetry(env, "datadog-installer")
	if err != nil {
		fmt.Printf("failed to initialize telemetry: %v\n", err)
		return nil
	}
	err = t.Start(context.Background())
	if err != nil {
		fmt.Printf("failed to start telemetry: %v\n", err)
		return nil
	}
	return t
}

func newSpan(operationName string) (ddtrace.Span, context.Context) {
	var spanOptions []ddtrace.StartSpanOption
	spanContext, ok := telemetry.SpanContextFromEnv()
	if ok {
		spanOptions = append(spanOptions, tracer.ChildOf(spanContext))
	}
	return tracer.StartSpanFromContext(context.Background(), operationName, spanOptions...)
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

func bootstrapCommand() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:     "bootstrap",
		Short:   "Bootstraps the package with the first version.",
		GroupID: "bootstrap",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			b := newBootstraperCmd("bootstrap")
			defer func() { b.Stop(err) }()
			ctx, cancel := context.WithTimeout(b.ctx, timeout)
			defer cancel()
			return bootstraper.Bootstrap(ctx, b.env)
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	return cmd
}

func installCommand() *cobra.Command {
	var installArgs []string
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
			defer func() { i.Stop(err) }()
			i.span.SetTag("params.url", args[0])
			return i.Install(i.ctx, args[0], installArgs)
		},
	}
	cmd.Flags().StringArrayVarP(&installArgs, "install_args", "A", nil, "Arguments to pass to the package")
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
			defer func() { i.Stop(err) }()
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
			defer func() { i.Stop(err) }()
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
			defer func() { i.Stop(err) }()
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
			defer func() { i.Stop(err) }()
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
			defer func() { i.Stop(err) }()
			i.span.SetTag("params.package", args[0])
			return i.PromoteExperiment(i.ctx, args[0])
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
			defer func() { i.Stop(err) }()
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
			defer func() { i.Stop(err) }()
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
