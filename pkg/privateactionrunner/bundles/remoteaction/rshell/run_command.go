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
	"path/filepath"
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

// rshellCommandPrefix is the namespace prefix the rshell interpreter expects
// on every allowed-command name (e.g. "rshell:cat").
const rshellCommandPrefix = "rshell:"

// RunCommandHandler implements the runCommand action.
//
// Both allow-lists follow the same pattern: nil means the operator did not
// configure the axis, so the backend list is used verbatim. A non-nil
// (possibly empty) list means the operator opted in to restricting further.
type RunCommandHandler struct {
	// operatorAllowedPaths is the operator's filesystem allowlist from
	// datadog.yaml. Nil when the operator did not configure the key.
	operatorAllowedPaths []string
	// operatorPathsFilterEnabled distinguishes "unset" (nil slice, no
	// filtering) from "empty list" (operator explicitly allowed nothing).
	operatorPathsFilterEnabled bool
	// operatorAllowedCommands is the set of bare command names the
	// operator has allow-listed. Populated only when filtering is enabled.
	operatorAllowedCommands map[string]struct{}
	// operatorCommandsFilterEnabled distinguishes "unset" from "empty".
	operatorCommandsFilterEnabled bool
}

// NewRunCommandHandler creates a new RunCommandHandler. Pass nil for either
// operator list to disable filtering on that axis. A non-nil empty slice is
// an explicit "block everything on this axis" and is honored as such.
func NewRunCommandHandler(operatorAllowedPaths []string, operatorAllowedCommands []string) *RunCommandHandler {
	h := &RunCommandHandler{}
	if operatorAllowedPaths != nil {
		h.operatorPathsFilterEnabled = true
		h.operatorAllowedPaths = operatorAllowedPaths
	}
	if operatorAllowedCommands != nil {
		h.operatorCommandsFilterEnabled = true
		h.operatorAllowedCommands = make(map[string]struct{}, len(operatorAllowedCommands))
		for _, c := range operatorAllowedCommands {
			bare := strings.TrimPrefix(c, rshellCommandPrefix)
			if bare == "" {
				continue
			}
			h.operatorAllowedCommands[bare] = struct{}{}
		}
	}
	return h
}

// filterAllowedCommands returns the effective command allowlist given the
// per-task list from the backend.
//
// The backend is the authoritative gate. A nil backend list (field absent)
// produces an empty result — rshell then blocks every command. The operator
// config can only tighten further; it cannot grant commands the backend did
// not.
func (h *RunCommandHandler) filterAllowedCommands(backendAllowed []string) []string {
	if backendAllowed == nil {
		return nil
	}
	if !h.operatorCommandsFilterEnabled {
		return backendAllowed
	}
	filtered := make([]string, 0, len(backendAllowed))
	for _, c := range backendAllowed {
		bare := strings.TrimPrefix(c, rshellCommandPrefix)
		if _, ok := h.operatorAllowedCommands[bare]; ok {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// filterAllowedPaths returns the effective filesystem allowlist given the
// per-task paths from the backend.
//
// Mirrors filterAllowedCommands: the backend is the authoritative gate. A
// nil backend list (field absent) returns nil — rshell then blocks every
// filesystem access. When the operator did not configure its own list, the
// backend list passes through unchanged. Otherwise the two are intersected
// using sub-path-aware narrowing.
func (h *RunCommandHandler) filterAllowedPaths(backendAllowed []string) []string {
	if backendAllowed == nil {
		return nil
	}
	if !h.operatorPathsFilterEnabled {
		return backendAllowed
	}
	var result []string
	seen := make(map[string]struct{})
	for _, b := range backendAllowed {
		bClean := filepath.Clean(b)
		for _, a := range h.operatorAllowedPaths {
			aClean := filepath.Clean(a)
			narrower, ok := narrowerPath(aClean, bClean)
			if !ok {
				continue
			}
			if _, dup := seen[narrower]; dup {
				continue
			}
			seen[narrower] = struct{}{}
			result = append(result, narrower)
		}
	}
	return result
}

// narrowerPath returns the more-restrictive of a and b if one is equal to or
// an ancestor of the other, and false otherwise. Inputs are assumed to be
// [filepath.Clean]ed already.
func narrowerPath(a, b string) (string, bool) {
	if a == b {
		return a, true
	}
	if isUnderOrEqual(b, a) {
		// b is under a — b is narrower.
		return b, true
	}
	if isUnderOrEqual(a, b) {
		// a is under b — a is narrower.
		return a, true
	}
	return "", false
}

// isUnderOrEqual reports whether child is equal to, or a descendant of,
// parent. The trailing-separator check avoids prefix-sibling false positives
// (e.g. "/var/logger" must not be considered under "/var/log").
func isUnderOrEqual(child, parent string) bool {
	if child == parent {
		return true
	}
	sep := string(filepath.Separator)
	p := parent
	if !strings.HasSuffix(p, sep) {
		p += sep
	}
	return strings.HasPrefix(child, p)
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
	AllowedPaths    []string `json:"allowedPaths,omitempty"`
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
	log.Debugf("rshell runCommand: command=%q backendAllowedCommands=%v effectiveAllowedCommands=%v backendAllowedPaths=%v effectiveAllowedPaths=%v operatorAllowedPaths=%v",
		inputs.Command, inputs.AllowedCommands, effectiveAllowedCommands, inputs.AllowedPaths, effectiveAllowedPaths, h.operatorAllowedPaths)

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
