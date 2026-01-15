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
	"io"
	"os"
	"os/exec"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// APMInjectExec is a wrapper around the ddinjector-installer.exe binary.
type APMInjectExec struct {
	execBinPath       string
	debug             bool
	silent            bool
	logPath           string
	ddAgentVersion    string
	ddinjectorPackage string
}

// NewAPMInjectExec returns a new APMInjectExec.
func NewAPMInjectExec(execBinPath string) *APMInjectExec {
	return &APMInjectExec{
		execBinPath: execBinPath,
	}
}

// WithDebug enables debug logging.
func (a *APMInjectExec) WithDebug(debug bool) *APMInjectExec {
	a.debug = debug
	return a
}

// WithSilent enables silent mode (no console output).
func (a *APMInjectExec) WithSilent(silent bool) *APMInjectExec {
	a.silent = silent
	return a
}

// WithLogPath sets the log file path.
func (a *APMInjectExec) WithLogPath(logPath string) *APMInjectExec {
	a.logPath = logPath
	return a
}

// WithDDAgentVersion sets the version of the Datadog Agent.
func (a *APMInjectExec) WithDDAgentVersion(version string) *APMInjectExec {
	a.ddAgentVersion = version
	return a
}

// WithDDInjectorPackage sets the path of the ddinjector package.
func (a *APMInjectExec) WithDDInjectorPackage(packagePath string) *APMInjectExec {
	a.ddinjectorPackage = packagePath
	return a
}

type apmInjectExecCmd struct {
	*exec.Cmd
	span *telemetry.Span
	ctx  context.Context
}

func (a *APMInjectExec) newAPMInjectExecCmd(ctx context.Context, command string, args ...string) *apmInjectExecCmd {
	span, ctx := telemetry.StartSpanFromContext(ctx, "apmInjectExec."+command)
	span.SetTag("args", args)

	// Build the command arguments
	cmdArgs := []string{command}

	// Add global flags
	if a.debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if a.silent {
		cmdArgs = append(cmdArgs, "--silent")
	}
	if a.logPath != "" {
		cmdArgs = append(cmdArgs, "--log-path", a.logPath)
	}

	// Add command-specific arguments
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, a.execBinPath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return &apmInjectExecCmd{
		Cmd:  cmd,
		span: span,
		ctx:  ctx,
	}
}

// Install installs the DDInjector kernel driver.
func (a *APMInjectExec) Install(ctx context.Context) (exitCode int, err error) {
	args := []string{}

	// Add install-specific flags
	if a.ddAgentVersion != "" {
		args = append(args, "--dd-agent-version", a.ddAgentVersion)
	}
	if a.ddinjectorPackage != "" {
		args = append(args, "--ddinjector-package", a.ddinjectorPackage)
	}

	cmd := a.newAPMInjectExecCmd(ctx, "install", args...)
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// Uninstall uninstalls the DDInjector kernel driver.
func (a *APMInjectExec) Uninstall(ctx context.Context) (exitCode int, err error) {
	cmd := a.newAPMInjectExecCmd(ctx, "uninstall")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

// UpdateSkipList updates the default skip list.
func (a *APMInjectExec) UpdateSkipList(ctx context.Context) (exitCode int, err error) {
	cmd := a.newAPMInjectExecCmd(ctx, "update-skiplist")
	defer func() { cmd.span.Finish(err) }()
	return cmd.Run()
}

func (a *apmInjectExecCmd) Run() (int, error) {
	var mergedBuffer bytes.Buffer
	errWriter := io.MultiWriter(&mergedBuffer, os.Stderr)
	outWriter := io.MultiWriter(&mergedBuffer, os.Stdout)
	a.Stderr = errWriter
	a.Stdout = outWriter

	err := a.Cmd.Run()
	if err == nil {
		return a.Cmd.ProcessState.ExitCode(), nil
	}

	if len(mergedBuffer.Bytes()) == 0 {
		return a.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w", err)
	}

	installerError := installerErrors.FromJSON(strings.TrimSpace(mergedBuffer.String()))
	return a.Cmd.ProcessState.ExitCode(), fmt.Errorf("run failed: %w \n%s", installerError, err.Error())
}
