// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package exec provides wrappers to external executables
package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// DotnetLibraryExec is a wrapper around the dotnet-library-exec binary.
type DotnetLibraryExec struct {
	execBinPath string
}

// NewDotnetLibraryExec returns a new DotnetLibraryExec.
func NewDotnetLibraryExec(execBinPath string) *DotnetLibraryExec {
	return &DotnetLibraryExec{
		execBinPath: execBinPath,
	}
}

type dotnetLibraryExecCmd struct {
	*exec.Cmd
	span *telemetry.Span
	ctx  context.Context
}

func (d *DotnetLibraryExec) newDotnetLibraryExecCmd(ctx context.Context, command string, args ...string) *dotnetLibraryExecCmd {
	span, ctx := telemetry.StartSpanFromContext(ctx, "dotnetLibraryExec."+command)
	span.SetTag("args", args)
	cmd := exec.CommandContext(ctx, d.execBinPath, append([]string{command}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return &dotnetLibraryExecCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

// InstallVersion installs a version of the library.
func (d *DotnetLibraryExec) InstallVersion(ctx context.Context, homePath string) (exitCode int, err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "install-version", "--home-path", homePath)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// UninstallVersion cleans up dependencies of a version of the library.
func (d *DotnetLibraryExec) UninstallVersion(ctx context.Context, homePath string) (exitCode int, err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "uninstall-version", "--home-path", homePath)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// EnableIISInstrumentation enables the IIS instrumentation on the system.
func (d *DotnetLibraryExec) EnableIISInstrumentation(ctx context.Context, homePath string) (exitCode int, err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "enable-iis-instrumentation", "--home-path", homePath)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// RemoveIISInstrumentation removes the IIS instrumentation from the system.
func (d *DotnetLibraryExec) RemoveIISInstrumentation(ctx context.Context) (exitCode int, err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "remove-iis-instrumentation")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

func (d *dotnetLibraryExecCmd) Run() (int, error) {
	var mergedBuffer bytes.Buffer
	errWriter := io.MultiWriter(&mergedBuffer, os.Stderr)
	outWriter := io.MultiWriter(&mergedBuffer, os.Stdout)
	d.Stderr = errWriter
	d.Stdout = outWriter

	err := d.Cmd.Run()
	if err == nil {
		return d.Cmd.ProcessState.ExitCode(), nil
	}

	if len(mergedBuffer.Bytes()) == 0 {
		return d.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w", err)
	}

	installerError := installerErrors.FromJSON(strings.TrimSpace(mergedBuffer.String()))
	return d.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w \n%s", installerError, err.Error())
}
