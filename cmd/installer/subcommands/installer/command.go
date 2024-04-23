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
	"time"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/fleet/bootstraper"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	envRegistry     = "DD_INSTALLER_REGISTRY"
	envRegistryAuth = "DD_INSTALLER_REGISTRY_AUTH"
	envAPIKey       = "DD_API_KEY"
	envSite         = "DD_SITE"
)

// Commands returns the installer subcommands.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	return []*cobra.Command{bootstrapCommand(), installCommand(), removeCommand(), installExperimentCommand(), removeExperimentCommand(), promoteExperimentCommand(), garbageCollectCommand()}
}

type cmd struct {
	t            *telemetry.Telemetry
	ctx          context.Context
	span         ddtrace.Span
	registry     string
	registryAuth string
	apiKey       string
	site         string
}

func newCmd(operation string) *cmd {
	span, ctx := newSpan(operation)
	registry := os.Getenv(envRegistry)
	registryAuth := os.Getenv(envRegistryAuth)
	apiKey := os.Getenv(envAPIKey)
	site := os.Getenv(envSite)
	return &cmd{
		t:            newTelemetry(),
		ctx:          ctx,
		span:         span,
		registry:     registry,
		registryAuth: registryAuth,
		apiKey:       apiKey,
		site:         site,
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

func newInstallerCmd(operation string) *installerCmd {
	cmd := newCmd(operation)
	var opts []installer.Option
	if cmd.registry != "" {
		opts = append(opts, installer.WithRegistry(cmd.registry))
	}
	if cmd.registryAuth != "" {
		opts = append(opts, installer.WithRegistryAuth(cmd.registryAuth))
	}
	i := installer.NewInstaller(opts...)
	return &installerCmd{
		Installer: i,
		cmd:       cmd,
	}
}

type bootstraperCmd struct {
	*cmd
	opts []bootstraper.Option
}

func newBootstraperCmd(operation string) *bootstraperCmd {
	cmd := newCmd(operation)
	var opts []bootstraper.Option
	if cmd.registry != "" {
		opts = append(opts, bootstraper.WithRegistry(cmd.registry))
	}
	if cmd.registryAuth != "" {
		opts = append(opts, bootstraper.WithRegistryAuth(cmd.registryAuth))
	}
	if cmd.apiKey != "" {
		opts = append(opts, bootstraper.WithAPIKey(cmd.apiKey))
	}

	return &bootstraperCmd{
		opts: opts,
		cmd:  cmd,
	}
}

func newTelemetry() *telemetry.Telemetry {
	apiKey := os.Getenv(envAPIKey)
	site := os.Getenv(envSite)
	if apiKey == "" || site == "" {
		fmt.Printf("telemetry disabled: missing DD_API_KEY or DD_SITE\n")
		return nil
	}
	t, err := telemetry.NewTelemetry(apiKey, site, "datadog-installer")
	if err != nil {
		fmt.Printf("failed to initialize telemetry: %v\n", err)
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
			return bootstraper.Bootstrap(ctx, b.opts...)
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	return cmd
}

func installCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install <url>",
		Short:   "Install a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newInstallerCmd("install")
			defer func() { i.Stop(err) }()
			i.span.SetTag("params.url", args[0])
			return i.Install(i.ctx, args[0])
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
			i := newInstallerCmd("remove")
			defer func() { i.Stop(err) }()
			i.span.SetTag("params.package", args[0])
			return i.Remove(i.ctx, args[0])
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
			i := newInstallerCmd("install-experiment")
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
			i := newInstallerCmd("remove-experiment")
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
			i := newInstallerCmd("promote-experiment")
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
			i := newInstallerCmd("garbage-collect")
			defer func() { i.Stop(err) }()
			return i.GarbageCollect(i.ctx)
		},
	}
	return cmd
}
