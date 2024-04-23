// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	// FIXME: use runtime.Executable() everywhere instead of hardcoding the path
	installerBin = filepath.Join(setup.InstallPath, "bin", "installer", "installer")
)

type installerExec struct {
	registry     string
	registryAuth string
	// telemetry options
	apiKey string
	site   string

	// FIXME: decide where we want to host the status logic
	pm installer.Installer
}

func newInstallerExec(registry string, registryAuth string, apiKey string, site string) *installerExec {
	return &installerExec{
		registry:     registry,
		registryAuth: registryAuth,
		apiKey:       apiKey,
		site:         site,
		pm:           installer.NewInstaller(),
	}
}

type installerCmd struct {
	*exec.Cmd
	span tracer.Span
	ctx  context.Context
}

func (i *installerExec) newInstallerCmd(ctx context.Context, command string, args ...string) *installerCmd {
	span, ctx := tracer.StartSpanFromContext(ctx, fmt.Sprintf("installer.%s", command))
	span.SetTag("args", args)
	span.SetTag("config.registry", i.registry)
	span.SetTag("config.registryAuth", i.registryAuth)
	span.SetTag("config.site", i.site)
	cmd := exec.CommandContext(ctx, installerBin, append([]string{command}, args...)...)
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

func (i *installerExec) Install(ctx context.Context, url string) (err error) {
	cmd := i.newInstallerCmd(ctx, "install", url)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) Remove(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) InstallExperiment(ctx context.Context, url string) (err error) {
	cmd := i.newInstallerCmd(ctx, "install-experiment", url)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) RemoveExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "remove-experiment", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) PromoteExperiment(ctx context.Context, pkg string) (err error) {
	cmd := i.newInstallerCmd(ctx, "promote-experiment", pkg)
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) GarbageCollect(ctx context.Context) (err error) {
	cmd := i.newInstallerCmd(ctx, "garbage-collect")
	defer func() { cmd.span.Finish(tracer.WithError(err)) }()
	return cmd.Run()
}

func (i *installerExec) State(pkg string) (repository.State, error) {
	return i.pm.State(pkg)
}

func (i *installerExec) States() (map[string]repository.State, error) {
	return i.pm.States()
}
