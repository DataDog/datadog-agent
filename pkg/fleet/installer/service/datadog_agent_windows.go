// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"os"
	"os/exec"
	"path/filepath"
)

func msiexec(target, operation string, args []string) (err error) {
	programData, err := internal.GetProgramDataDirForProduct("Datadog Installer")
	if err != nil {
		return nil
	}
	updaterPath := filepath.Join(programData, "datadog-agent", target)
	msis, err := filepath.Glob(filepath.Join(updaterPath, "datadog-agent-*-1-x86_64.msi"))
	if err != nil {
		return nil
	}
	if len(msis) != 1 {
		return fmt.Errorf("too many MSIs in package")
	}

	tmpDir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("install-%s-*", filepath.Base(msis[0])))
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}

	logPath := filepath.Join(tmpDir, "install.log")
	cmd := exec.Command("msiexec", append([]string{operation, msis[0], "/qn", "/l", logPath, "MSIFASTINSTALL=7"}, args...)...)
	return cmd.Run()
}

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, args []string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	return msiexec("stable", "/i", args)
}

// StartAgentExperiment noop
func StartAgentExperiment(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to start agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	return msiexec("experiment", "/i", nil)
}

// StopAgentExperiment noop
func StopAgentExperiment(ctx context.Context) (err error) {
	err = RemoveAgent(ctx)
	span, ctx := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to stop agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()

	// TODO: Need args here to restore DDAGENTUSER
	return msiexec("stable", "/i", nil)
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(_ context.Context) error {
	// noop
	return nil
}

// RemoveAgent noop
func RemoveAgent(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to remove agent: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	return msiexec("stable", "/x", nil)
}
