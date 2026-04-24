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
// Both allow-lists are stored as dedup-preserving slices of operator entries
// plus a boolean that distinguishes "unset" from "empty". The filter
// functions intersect with the backend list under two different notions of
// equivalence:
//
//   - commands compare by exact string equality. The backend stamps every
//     entry with the "rshell:" namespace (e.g. "rshell:cat"); operators must
//     spell entries the same way in datadog.yaml for them to match;
//   - paths compare by containment, with the narrower side winning, so that
//     an operator entry of "/var/log/nginx" is admitted even when the
//     backend only lists "/var/log". Prefix siblings like "/var/logger" do
//     not match "/var/log" — boundary is a path separator.
type RunCommandHandler struct {
	// operatorAllowedPaths is the operator's filesystem allowlist from
	// datadog.yaml. Populated only when filtering is enabled.
	operatorAllowedPaths []string
	// operatorPathsFilterEnabled distinguishes "unset" (nil slice, no
	// filtering) from "empty list" (operator explicitly allowed nothing).
	operatorPathsFilterEnabled bool
	// operatorAllowedCommands is the operator allow-list, stored verbatim.
	// Entries are expected to be namespaced ("rshell:<name>") to match the
	// backend form; anything else silently fails to intersect.
	operatorAllowedCommands []string
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
		h.operatorAllowedPaths = dedupeNonEmpty(operatorAllowedPaths)
	}
	if operatorAllowedCommands != nil {
		h.operatorCommandsFilterEnabled = true
		h.operatorAllowedCommands = dedupeNonEmpty(operatorAllowedCommands)
	}
	return h
}

// dedupeNonEmpty returns the input slice with empties removed and duplicates
// dropped, preserving first-seen order.
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

// filterAllowedCommands returns the effective command allowlist given the
// per-task list from the backend.
//
// The backend is the authoritative gate. A nil backend list (field absent)
// produces an empty result — rshell then blocks every command. The operator
// config can only tighten further; it cannot grant commands the backend did
// not. Comparison is plain string equality: operator entries in datadog.yaml
// must be spelled in the backend's namespaced form ("rshell:<name>") to
// match.
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

// filterAllowedPaths is the paths analogue of filterAllowedCommands, but
// with containment-aware intersection: the narrower of each matching
// (backend, operator) pair wins. This admits an operator sub-path when the
// backend entry is a parent directory, and vice versa. Disjoint pairs are
// skipped. Prefix siblings do not match (boundary is a path separator), so
// "/var/logger" does not satisfy a "/var/log" rule.
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
				// operator entry is equal or narrower — admit it.
				admit(o)
			case pathContains(o, b):
				// backend entry is strictly narrower — admit it.
				admit(b)
			}
		}
	}
	return filtered
}

// pathContains reports whether parent is an ancestor of (or equal to) child.
// Paths are assumed to be in forward-slash form — the operator-side
// contract (see the datadog.yaml loader) rejects Windows-native backslash
// paths up front, so this function does not deal with separator
// normalization. Leading and trailing slashes are normalized via
// path.Clean so "/var/log/" and "/var/log" compare equal. Prefix-sibling
// strings like "/var/logger" do not count as contained in "/var/log".
func pathContains(parent, child string) bool {
	parent = path.Clean(parent)
	child = path.Clean(child)
	if parent == child {
		return true
	}
	// Ensure parent ends with exactly one separator before the prefix
	// check, so "/var/log" tests as a prefix of "/var/log/nginx" but not
	// of "/var/logger".
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
