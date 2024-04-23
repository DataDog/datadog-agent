// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// InstallerExec is an implementation of the Installer interface that uses the installer binary.
type InstallerExec struct {
	installerBinPath string

	registry     string
	registryAuth string
	// telemetry options
	apiKey string
	site   string

	// FIXME: decide where we want to host the status logic
	pm installer.Installer
}

// NewInstallerExec returns a new InstallerExec.
func NewInstallerExec(installerBinPath string, registry string, registryAuth string, apiKey string, site string) *InstallerExec {
	return &InstallerExec{
		installerBinPath: installerBinPath,
		registry:         registry,
		registryAuth:     registryAuth,
		apiKey:           apiKey,
		site:             site,
		pm:               installer.NewInstaller(),
	}
}

type installerCmd struct {
	*exec.Cmd
	span tracer.Span
	ctx  context.Context
}

func (i *InstallerExec) newInstallerCmd(ctx context.Context, command string, args ...string) *installerCmd {
	span, ctx := tracer.StartSpanFromContext(ctx, fmt.Sprintf("installer.%s", command))
	span.SetTag("args", args)
	span.SetTag("config.registry", i.registry)
	span.SetTag("config.registryAuth", i.registryAuth)
	span.SetTag("config.site", i.site)
	cmd := exec.CommandContext(ctx, i.installerBinPath, append([]string{command}, args...)...)
	env := []string{
		fmt.Sprintf("DD_INSTALLER_REGISTRY=%s", i.registry),
		fmt.Sprintf("DD_INSTALLER_REGISTRY_AUTH=%s", i.registryAuth),
		fmt.Sprintf("DD_API_KEY=%s", i.apiKey),
		fmt.Sprintf("DD_SITE=%s", i.site),
	}
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	env = append(env, telemetry.EnvFromSpanContext(span.Context())...)
	cmd.Env = env
	return &installerCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

// Install installs a package.
func (i *InstallerExec) Install(ctx context.Context, url string) (err error) {
	cmd := i.newInstallerCmd(ctx, "install", url)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// Remove removes a package.
func (i *InstallerExec) Remove(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// InstallExperiment installs an experiment.
func (i *InstallerExec) InstallExperiment(ctx context.Context, url string) (err error) {
	cmd := i.newInstallerCmd(ctx, "install-experiment", url)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// RemoveExperiment removes an experiment.
func (i *InstallerExec) RemoveExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove-experiment", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// PromoteExperiment promotes an experiment to stable.
func (i *InstallerExec) PromoteExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "promote-experiment", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// GarbageCollect runs the garbage collector.
func (i *InstallerExec) GarbageCollect(ctx context.Context) (err error) {
	cmd := i.newInstallerCmd(ctx, "garbage-collect")
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// State returns the state of a package.
func (i *InstallerExec) State(pkg string) (repository.State, error) {
	return i.pm.State(pkg)
}

// States returns the states of all packages.
func (i *InstallerExec) States() (map[string]repository.State, error) {
	return i.pm.States()
}
