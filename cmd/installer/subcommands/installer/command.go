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

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Commands returns the installer subcommands.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	return []*cobra.Command{installCommand(), removeCommand(), installExperimentCommand(), removeExperimentCommand(), promoteExperimentCommand(), garbageCollectCommand()}
}

type installerCmd struct {
	installer.Installer
	t    *telemetry.Telemetry
	ctx  context.Context
	span ddtrace.Span
}

func (i *installerCmd) Stop(err error) {
	i.span.Finish(tracer.WithError(err))
	if i.t != nil {
		err := i.t.Stop(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop telemetry: %v\n", err)
		}
	}
}

func newInstallerCmd(operation string) *installerCmd {
	span, ctx := newSpan(operation)
	registry := os.Getenv("DD_INSTALLER_REGISTRY")
	registryAuth := os.Getenv("DD_INSTALLER_REGISTRY_AUTH")
	var installerOpts []installer.Options
	if registry != "" {
		installerOpts = append(installerOpts, installer.WithRegistry(registry))
	}
	if registryAuth != "" {
		installerOpts = append(installerOpts, installer.WithRegistryAuth(installer.RegistryAuth(registryAuth)))
	}
	i := installer.NewInstaller(installerOpts...)
	return &installerCmd{
		Installer: i,
		t:         newTelemetry(),
		ctx:       ctx,
		span:      span,
	}
}

func newTelemetry() *telemetry.Telemetry {
	apiKey := os.Getenv("DD_API_KEY")
	site := os.Getenv("DD_SITE")
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
