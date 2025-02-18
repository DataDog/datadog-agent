// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package exec

import (
	"bytes"
	"context"
	"fmt"
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
	span, ctx := telemetry.StartSpanFromContext(ctx, fmt.Sprintf("dotnetLibraryExec.%s", command))
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

// Install installs the library.
func (d *DotnetLibraryExec) Install(ctx context.Context, homePath string) (err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "install", "--home-path", homePath)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// UninstallVersion cleans up dependencies of a version of the library.
func (d *DotnetLibraryExec) UninstallVersion(ctx context.Context, homePath string) (err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "uninstall-version", "--home-path", homePath)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// UninstallProduct cleans up the env variables and other parameters that are not cleaned up in UninstallVersion.
// This is meant to be called when we completely uninstall the library from the system.
func (d *DotnetLibraryExec) UninstallProduct(ctx context.Context) (err error) {
	cmd := d.newDotnetLibraryExecCmd(ctx, "uninstall-product")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

func (iCmd *dotnetLibraryExecCmd) Run() error {
	var errBuf bytes.Buffer
	iCmd.Stderr = &errBuf
	err := iCmd.Cmd.Run()
	if err == nil {
		return nil
	}

	if len(errBuf.Bytes()) == 0 {
		return fmt.Errorf("run failed: %w", err)
	}

	installerError := installerErrors.FromJSON(strings.TrimSpace(errBuf.String()))
	return fmt.Errorf("run failed: %w \n%s", installerError, err.Error())
}
