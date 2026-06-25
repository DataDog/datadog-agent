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
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/rshell/interp"

	"github.com/DataDog/datadog-agent/pkg/config/env"
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

// RunCommandHandlerConfig carries agent-side rshell policy settings.
type RunCommandHandlerConfig struct {
	OperatorAllowedPaths              []string
	OperatorAllowedPathsConfigured    bool
	OperatorAllowedCommands           []string
	OperatorAllowedCommandsConfigured bool
}

// RunCommandHandler implements the runCommand and runRemediationCommand actions.
//
// The two actions share all sandboxing logic and differ only in mode:
// - runCommand runs rshell in read-only mode (interp.ModeReadOnly) ;
// - runRemediationCommand runs it in remediation mode (interp.ModeRemediation)
// both still confined to the effective AllowedPaths sandbox.
//
// Both allow-lists are intersected unconditionally with the per-task backend
// list before being passed to rshell. They use different equivalence
// notions, and each axis has a sentinel value that means "allow whatever
// the backend allowed":
//
//   - commands compare by exact string equality, with one special case:
//     the literal "rshell:*" admits every backend entry in the "rshell:"
//     namespace. Other operator entries must be in the backend's namespaced
//     form to match.
//   - paths compare by containment with the narrower side winning; the
//     sentinel "/" admits every absolute path through containment.
//
// On either axis, an explicit empty operator list is the kill-switch.
type RunCommandHandler struct {
	operatorAllowedPaths              []string
	operatorAllowedPathsConfigured    bool
	operatorAllowedCommands           []string
	operatorAllowedCommandsConfigured bool
	mode                              interp.Mode
}

// newRunCommandHandler builds a run-command handler and precomputes the
// operator allowlists:
//
//  1. Paths are normalized, reduced to the broadest entries per access group,
//     and deduplicated so same-path read-write entries replace read-only ones.
//  2. Commands are deduplicated.
func newRunCommandHandler(operatorAllowedPaths []string, operatorAllowedCommands []string, mode interp.Mode, pathsConfigured, commandsConfigured bool) *RunCommandHandler {
	// remove duplicates
	commands := slices.Clone(operatorAllowedCommands)
	slices.Sort(commands)
	commands = slices.Compact(commands)
	return &RunCommandHandler{
		operatorAllowedPaths:              reducePathListToBroadest(cleanPathList(operatorAllowedPaths)),
		operatorAllowedPathsConfigured:    pathsConfigured,
		operatorAllowedCommands:           commands,
		operatorAllowedCommandsConfigured: commandsConfigured,
		mode:                              mode,
	}
}

func NewRunCommandHandler(cfg RunCommandHandlerConfig) *RunCommandHandler {
	return newRunCommandHandler(cfg.OperatorAllowedPaths, cfg.OperatorAllowedCommands, interp.ModeReadOnly, cfg.OperatorAllowedPathsConfigured, cfg.OperatorAllowedCommandsConfigured)
}

// NewRunRemediationCommandHandler builds the write-capable runRemediationCommand
// handler. It shares all sandboxing with runCommand and only switches rshell into
// remediation mode.
func NewRunRemediationCommandHandler(cfg RunCommandHandlerConfig) *RunCommandHandler {
	return newRunCommandHandler(cfg.OperatorAllowedPaths, cfg.OperatorAllowedCommands, interp.ModeRemediation, cfg.OperatorAllowedPathsConfigured, cfg.OperatorAllowedCommandsConfigured)
}

// filterAllowedCommands returns the effective command allowlist, passed to rshell:
// the signed task list limited to the rshell command namespace, optionally
// narrowed by explicitly configured agent-side commands.
func (h *RunCommandHandler) filterAllowedCommands(backendAllowed []string) []string {
	backendAllowed = onlyRshellPrefixedCommands(backendAllowed)
	if !h.operatorAllowedCommandsConfigured {
		return backendAllowed
	}
	return intersectAllowedCommands(backendAllowed, h.operatorAllowedCommands)
}

// filterAllowedPaths returns the effective path allowlist passed to rshell:
//
//  1. Normalize the signed backend paths.
//  2. If no operator allowlist is configured, reduce the backend paths to
//     remove duplicates and redundant descendants, then use that reduced list.
//  3. If an operator allowlist is configured, intersect operator and backend
//     paths by access group and containment, keeping the narrower matching path.
//  4. Remove same-path read-only entries when a read-write entry exists. For example,
//     if `/var/log:ro` and `/var/log:rw` both exist, only `/var/log:rw` is kept.
func (h *RunCommandHandler) filterAllowedPaths(backend []string) []string {
	backendPaths := cleanPathList(backend)
	if !h.operatorAllowedPathsConfigured {
		return reducePathListToBroadest(backendPaths)
	}
	return intersectAllowedPathsByAccess(h.operatorAllowedPaths, backendPaths)
}

// RunCommandInputs defines the user-supplied inputs for the runCommand action.
//
// The command allowlists are no longer carried in inputs: they are resolved
// from execution policies on the backend and delivered in the signed task
// (Attributes.RemoteAction.TargetCommands / Attributes.RemoteAction.TargetPaths).
// Inputs only carry the command to run.
type RunCommandInputs struct {
	Command string `json:"command"`
}

// RunCommandOutputs defines the outputs for the runCommand action.
//
// SandboxWarnings carries non-fatal diagnostic messages emitted by rshell
// during runner construction (e.g. an AllowedPaths entry that does not
// exist on the host). Empty when the sandbox configuration is clean. These
// messages indicate misconfiguration, not command failure: they are
// independent of ExitCode.
type RunCommandOutputs struct {
	ExitCode        int      `json:"exitCode"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	SandboxWarnings []string `json:"sandboxWarnings,omitempty"`
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

	// The backend allowlists come from the signed task, not from user inputs.
	var backendCommands []string
	var backendPaths []string
	if task.Data.Attributes.RemoteAction != nil {
		backendCommands = task.Data.Attributes.RemoteAction.TargetCommands
		backendPaths = task.Data.Attributes.RemoteAction.TargetPaths
	}
	effectiveAllowedCommands := h.filterAllowedCommands(backendCommands)
	effectiveAllowedPaths := h.filterAllowedPaths(backendPaths)
	log.Debugf("rshell runCommand (mode=%s): command=%q backendAllowedCommands=%v effectiveAllowedCommands=%v backendAllowedPaths=%v effectiveAllowedPaths=%v",
		h.mode, inputs.Command, backendCommands, effectiveAllowedCommands, backendPaths, effectiveAllowedPaths)

	prog, err := syntax.NewParser().Parse(strings.NewReader(inputs.Command), "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}

	for _, p := range effectiveAllowedPaths {
		statPath := pathSpecPath(p)
		if _, err := statFn(statPath); err != nil {
			log.Warnf("path %q not found, rshell may fail to execute commands", statPath)
		}
	}
	// rshell treats allowed paths as read-only unless carrying a ":rw" suffix,
	// so, unlike read-only mode, remediation mode must opt paths into writes.
	effectiveAllowedPathsRW := effectiveAllowedPaths
	if h.mode == interp.ModeRemediation {
		effectiveAllowedPathsRW = make([]string, len(effectiveAllowedPaths))
		for i, p := range effectiveAllowedPaths {
			effectiveAllowedPathsRW[i] = p + ":rw"
		}
	}
	var stdout, stderr bytes.Buffer
	// Route sandbox diagnostics to a dedicated sink so they do not leak
	// into the action's stderr field. We discard the streaming output and
	// read the messages back via runner.Warnings() into SandboxWarnings.
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.WarningsWriter(io.Discard),
		interp.AllowedPaths(effectiveAllowedPathsRW),
		interp.ProcPath(resolveProcPath()),
		interp.AllowedCommands(effectiveAllowedCommands),
		interp.WithMode(h.mode),
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
		ExitCode:        exitCode,
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		SandboxWarnings: runner.Warnings(),
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
