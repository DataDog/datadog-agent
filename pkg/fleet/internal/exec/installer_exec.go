// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// InstallerExec is an implementation of the Installer interface that uses the installer binary.
type InstallerExec struct {
	env              *env.Env
	installerBinPath string
}

// NewInstallerExec returns a new InstallerExec.
func NewInstallerExec(env *env.Env, installerBinPath string) *InstallerExec {
	return &InstallerExec{
		env:              env,
		installerBinPath: installerBinPath,
	}
}

type installerCmd struct {
	*exec.Cmd
	span tracer.Span
	ctx  context.Context
}

func (i *InstallerExec) newInstallerCmd(ctx context.Context, command string, args ...string) *installerCmd {
	env := i.env.ToEnv()
	span, ctx := tracer.StartSpanFromContext(ctx, fmt.Sprintf("installer.%s", command))
	span.SetTag("args", args)
	cmd := exec.CommandContext(ctx, i.installerBinPath, append([]string{command}, args...)...)
	env = append(os.Environ(), env...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	env = append(env, telemetry.EnvFromSpanContext(span.Context())...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return &installerCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

// Install installs a package.
func (i *InstallerExec) Install(ctx context.Context, url string, _ []string) (err error) {
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

// Purge - noop, must be called by the package manager on uninstall.
func (i *InstallerExec) Purge(_ context.Context) {
	panic("don't call Purge directly")
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

// InstrumentAPMInjector instruments the APM auto-injector.
func (i *InstallerExec) InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	cmd := i.newInstallerCmd(ctx, "apm instrument", method)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// UninstrumentAPMInjector uninstruments the APM auto-injector.
func (i *InstallerExec) UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	cmd := i.newInstallerCmd(ctx, "apm uninstrument", method)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

// IsInstalled checks if a package is installed.
func (i *InstallerExec) IsInstalled(ctx context.Context, pkg string) (_ bool, err error) {
	cmd := i.newInstallerCmd(ctx, "is-installed", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	err = cmd.Run()
	if err != nil && cmd.ProcessState.ExitCode() == 10 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DefaultPackages returns the default packages to install.
func (i *InstallerExec) DefaultPackages(ctx context.Context) (_ []string, err error) {
	cmd := i.newInstallerCmd(ctx, "default-packages")
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error running default-packages: %w\n%s", err, stderr.String())
	}
	var defaultPackages []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if line == "" {
			continue
		}
		defaultPackages = append(defaultPackages, line)
	}
	return defaultPackages, nil
}

// State returns the state of a package.
func (i *InstallerExec) State(pkg string) (repository.State, error) {
	repositories := repository.NewRepositories(installer.PackagesPath, installer.LocksPack)
	return repositories.Get(pkg).GetState()
}

// States returns the states of all packages.
func (i *InstallerExec) States() (map[string]repository.State, error) {
	repositories := repository.NewRepositories(installer.PackagesPath, installer.LocksPack)
	return repositories.GetState()
}
