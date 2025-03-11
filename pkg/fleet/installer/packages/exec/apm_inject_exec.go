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
	"os"
	"os/exec"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// ApmInjectExec is a wrapper around the apm-inject-exec binary.
type ApmInjectExec struct {
	execBinPath string
}

// NewApmInjectExec returns a new ApmInjectExec.
func NewApmInjectExec(execBinPath string) *ApmInjectExec {
	return &ApmInjectExec{
		execBinPath: execBinPath,
	}
}

type apmInjectExecCmd struct {
	*exec.Cmd
	span *telemetry.Span
	ctx  context.Context
}

func (d *ApmInjectExec) newApmInjectExecCmd(ctx context.Context, command string, args ...string) *apmInjectExecCmd {
	span, ctx := telemetry.StartSpanFromContext(ctx, fmt.Sprintf("apmInject.%s", command))
	span.SetTag("args", args)
	cmd := exec.CommandContext(ctx, d.execBinPath, append([]string{command}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return &apmInjectExecCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

// Instrument turns on the APM injector
func (d *ApmInjectExec) Instrument(ctx context.Context) (exitCode int, err error) {
	cmd := d.newApmInjectExecCmd(ctx, "-i")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// Uninstrument turns off the APM injector
func (d *ApmInjectExec) Uninstrument(ctx context.Context) (exitCode int, err error) {
	cmd := d.newApmInjectExecCmd(ctx, "-u")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

func (d *apmInjectExecCmd) Run() (int, error) {
	var errBuf bytes.Buffer
	d.Stderr = &errBuf
	err := d.Cmd.Run()
	if err == nil {
		return d.Cmd.ProcessState.ExitCode(), nil
	}

	if len(errBuf.Bytes()) == 0 {
		return d.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w", err)
	}

	installerError := installerErrors.FromJSON(strings.TrimSpace(errBuf.String()))
	return d.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w \n%s", installerError, err.Error())
}
