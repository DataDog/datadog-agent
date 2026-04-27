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

// RunCommandHandler implements the runCommand action.
//
// Commands intersect by exact string equality (operator must use the
// "rshell:<name>" form). Paths intersect by containment with "narrower
// wins"; prefix siblings like "/var/logger" do not match "/var/log".
type RunCommandHandler struct {
	// Nil = operator did not configure (backend passes through). Empty =
	// explicit kill-switch.
	operatorAllowedPaths       []string
	operatorPathsFilterEnabled bool

	operatorAllowedCommands       []string
	operatorCommandsFilterEnabled bool
}

// NewRunCommandHandler creates a new RunCommandHandler. Pass nil to leave
// filtering off on an axis; a non-nil empty slice is the explicit
// kill-switch.
func NewRunCommandHandler(operatorAllowedPaths []string, operatorAllowedCommands []string) *RunCommandHandler {
	h := &RunCommandHandler{}
	if operatorAllowedPaths != nil {
		h.operatorPathsFilterEnabled = true
		h.operatorAllowedPaths = dedupeNonEmpty(operatorAllowedPaths)
	}
	if operatorAllowedCommands != nil {
		h.operatorCommandsFilterEnabled = true
		h.operatorAllowedCommands = dedupeNonEmpty(operatorAllowedCommands)
	}
	return h
}

// dedupeNonEmpty drops empties and duplicates, preserving first-seen order.
func dedupeNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// filterAllowedCommands returns the operator ∩ backend command list.
// Plain string equality; nil backend short-circuits to nil (rshell blocks).
func (h *RunCommandHandler) filterAllowedCommands(backendAllowed []string) []string {
	if backendAllowed == nil {
		return nil
	}
	if !h.operatorCommandsFilterEnabled {
		return backendAllowed
	}
	operatorSet := make(map[string]struct{}, len(h.operatorAllowedCommands))
	for _, c := range h.operatorAllowedCommands {
		operatorSet[c] = struct{}{}
	}
	filtered := make([]string, 0, len(backendAllowed))
	for _, c := range backendAllowed {
		if _, ok := operatorSet[c]; ok {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// filterAllowedPaths returns the operator ∩ backend path list with
// "narrower wins" containment matching.
func (h *RunCommandHandler) filterAllowedPaths(backendAllowed []string) []string {
	if backendAllowed == nil {
		return nil
	}
	if !h.operatorPathsFilterEnabled {
		return backendAllowed
	}
	filtered := make([]string, 0, len(backendAllowed))
	seen := make(map[string]struct{})
	admit := func(p string) {
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		filtered = append(filtered, p)
	}
	for _, b := range backendAllowed {
		for _, o := range h.operatorAllowedPaths {
			switch {
			case pathContains(b, o):
				admit(o)
			case pathContains(o, b):
				admit(b)
			}
		}
	}
	return filtered
}

// pathContains reports whether parent is an ancestor of (or equal to)
// child, with the path separator as the boundary so "/var/logger" does not
// match "/var/log".
func pathContains(parent, child string) bool {
	parent = path.Clean(parent)
	child = path.Clean(child)
	if parent == child {
		return true
	}
	if !strings.HasSuffix(parent, "/") {
		parent += "/"
	}
	return strings.HasPrefix(child, parent)
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
