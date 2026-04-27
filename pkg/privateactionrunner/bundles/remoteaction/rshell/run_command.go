// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/rshell/interp"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultProcPath         = "/proc"
	containerizedPathPrefix = "/host"
)

// statFn is the function used to check path existence. It defaults to os.Stat
// and can be overridden in tests.
var statFn = os.Stat

// RunCommandHandler implements the runCommand action.
//
// Both axes use a sentinel value that means "allow whatever the backend
// allowed":
//
//   - commands compare by exact string equality, with "rshell:*" as a
//     special case admitting every backend entry in the rshell namespace.
//   - paths compare by containment with the narrower side winning; the
//     sentinel "/" admits every absolute path.
//
// On either axis, an explicit empty operator list is the kill-switch.
type RunCommandHandler struct {
	operatorAllowedPaths    []string
	operatorAllowedCommands []string
}

// NewRunCommandHandler clones, sorts, and deduplicates the operator
// commands; cleans and reduces the operator paths to broadest form. The
// caller's slices are not mutated.
func NewRunCommandHandler(operatorAllowedPaths []string, operatorAllowedCommands []string) *RunCommandHandler {
	commands := slices.Clone(operatorAllowedCommands)
	slices.Sort(commands)
	return &RunCommandHandler{
		operatorAllowedPaths:    reducePathListToBroadest(cleanPathList(operatorAllowedPaths)),
		operatorAllowedCommands: slices.Compact(commands),
	}
}

// filterAllowedCommands returns the operator ∩ backend list. Plain string
// equality except for the "rshell:*" sentinel which admits every backend
// entry in the rshell namespace.
func (h *RunCommandHandler) filterAllowedCommands(backendAllowed []string) []string {
	if len(backendAllowed) == 0 || len(h.operatorAllowedCommands) == 0 {
		return []string{}
	}
	if slices.Contains(h.operatorAllowedCommands, setup.RShellCommandAllowAllWildcard) {
		return onlyRshellPrefixedCommands(backendAllowed)
	}
	filtered := make([]string, 0)
	for _, c := range backendAllowed {
		if slices.Contains(h.operatorAllowedCommands, c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// filterAllowedPaths returns the operator ∩ backend list with "narrower
// wins" containment matching. Sentinel "/" admits every backend entry.
func (h *RunCommandHandler) filterAllowedPaths(backend []string) []string {
	if len(backend) == 0 || len(h.operatorAllowedPaths) == 0 {
		return []string{}
	}
	backend = reducePathListToBroadest(cleanPathList(backend))
	if slices.Contains(h.operatorAllowedPaths, setup.RShellPathAllowAll) {
		return backend
	}
	return intersectPathLists(h.operatorAllowedPaths, backend)
}

// RunCommandInputs defines the inputs for the runCommand action.
//
// The backend is the authoritative source for both allowlists. A nil Go
// slice (field absent or explicit JSON null) blocks everything on its
// respective axis — rshell refuses to run any command or open any file.
// A non-nil list is intersected with the operator config before being
// handed to rshell.
type RunCommandInputs struct {
	Command         string   `json:"command"`
	AllowedCommands []string `json:"allowedCommands"`
	AllowedPaths    []string `json:"allowedPaths"`
}

// RunCommandOutputs defines the outputs for the runCommand action.
type RunCommandOutputs struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// Run executes the command through the rshell restricted interpreter.
// The environment is intentionally empty; no host environment variables are forwarded.
func (h *RunCommandHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunCommandInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.Command == "" {
		return nil, errors.New("command is required")
	}

	effectiveAllowedCommands := h.filterAllowedCommands(inputs.AllowedCommands)
	effectiveAllowedPaths := h.filterAllowedPaths(inputs.AllowedPaths)
	log.Debugf("rshell runCommand: command=%q backendAllowedCommands=%v effectiveAllowedCommands=%v backendAllowedPaths=%v effectiveAllowedPaths=%v",
		inputs.Command, inputs.AllowedCommands, effectiveAllowedCommands, inputs.AllowedPaths, effectiveAllowedPaths)

	prog, err := syntax.NewParser().Parse(strings.NewReader(inputs.Command), "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}

	for _, p := range effectiveAllowedPaths {
		if _, err := statFn(p); err != nil {
			log.Warnf("path %q not found, rshell may fail to execute commands", p)
		}
	}
	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.AllowedPaths(effectiveAllowedPaths),
		interp.ProcPath(resolveProcPath()),
		interp.AllowedCommands(effectiveAllowedCommands),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}
	defer runner.Close()

	// Spans emitted by rshell inherit the rshell service via context.
	runErr := runner.Run(telemetry.WithService(ctx, observability.RshellService), prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			return nil, runErr
		}
	}

	return &RunCommandOutputs{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// resolveProcPath returns the proc filesystem path appropriate for the current
// environment. In containerized deployments with host mounts, it returns
// /host/proc; otherwise it falls back to /proc.
func resolveProcPath() string {
	if env.IsContainerized() {
		hostProc := path.Join(containerizedPathPrefix, defaultProcPath)
		if _, err := statFn(hostProc); err == nil {
			return hostProc
		}
	}
	return defaultProcPath
}
