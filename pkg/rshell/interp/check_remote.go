// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/expand"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
)

// DefaultRemoteCheckTimeout bounds CheckRemote when RemotePolicy.Timeout is zero.
const DefaultRemoteCheckTimeout = 5 * time.Second

// RemotePolicy contains effective grants already authenticated and authorized
// by the embedding application. CheckRemote does not resolve hostnames, load
// persisted policies, or authenticate callers. The target must use the same
// rshell version and platform conventions (path syntax and builtin registry).
type RemotePolicy struct {
	// AllowedCommands uses the same rshell:<command> names as AllowedCommands.
	// Empty grants deny every command. There is no implicit wildcard.
	AllowedCommands []string
	// AllowedPaths contains absolute remote roots, optionally ending in :ro
	// (the default) or :rw. No root is opened or skipped based on local state.
	// Colons are reserved for the access suffix and Windows volume prefix.
	AllowedPaths []string
	// Mode defaults to ModeReadOnly. ModeRemediation enables write capabilities,
	// which still require their path, command, and service grants.
	Mode Mode
	// AllowedSystemServices grants exact unit/action pairs. Invalid grants are
	// configuration errors. Empty action lists grant nothing, as in execution.
	AllowedSystemServices []SystemServiceControlGrant
	// ElevatableCommands permits the sudo marker for these rshell:<command>
	// names, in addition to AllowedCommands. No callback or backend is used.
	ElevatableCommands []string
	// Env contains explicit KEY=value pairs. No host environment is inherited.
	// Standard shell variables are initialized as in a fresh Runner; PWD is
	// unknown without Dir, and ALLOWED_PATHS requires target root discovery.
	Env []string
	// Dir is an optional absolute remote working directory. If empty, relative
	// path operands remain indeterminate even if there is just one path grant.
	Dir string
	// Timeout bounds this call, including configuration and script analysis.
	// Zero uses DefaultRemoteCheckTimeout; negative values are rejected. Like
	// MaxExecutionTime, each call gets a fresh deadline and an earlier context
	// deadline takes precedence.
	Timeout time.Duration
}

// CheckRemote previews script authorization against policy without constructing
// a Runner or inspecting the embedding application's filesystem, cwd, streams,
// environment, systemd, or elevation backends. It reuses Runner.Check's parser,
// validation, traversal, builtin argument analysis, and resource limits.
//
// Known policy denials take precedence over uncertainty. Lexically permitted
// path operations are CheckIndeterminate with CheckRequiresExecution: remote
// root availability, symlink containment, file type, hard links, and OS access
// cannot be established here. Requested path strings are kept in diagnostics.
//
// CheckAllowed is an authorization preview, not an execution-success guarantee.
// Actual execution must reauthorize against the target's current policy and
// state using the normal Runner and sandbox. All syntactic branches are checked,
// even if they might not execute. Syntax errors and denials are report data;
// configuration errors, cancellation, and timeouts use the Go error return.
func CheckRemote(ctx context.Context, script string, policy RemotePolicy) (*CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if policy.Timeout < 0 {
		return nil, errors.New("CheckRemote: Timeout must be >= 0")
	}
	timeout := policy.Timeout
	if timeout == 0 {
		timeout = DefaultRemoteCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if policy.Mode != "" && policy.Mode != ModeReadOnly && policy.Mode != ModeRemediation {
		return nil, fmt.Errorf("CheckRemote: unrecognised execution mode %q", policy.Mode)
	}
	if policy.Dir != "" && (!filepath.IsAbs(policy.Dir) || strings.ContainsRune(policy.Dir, 0)) {
		return nil, errors.New("CheckRemote: Dir must be an absolute remote path without NUL bytes")
	}
	commands, err := parseAllowedCommands(policy.AllowedCommands)
	if err != nil {
		return nil, err
	}
	elevatable, err := parseAllowedCommands(policy.ElevatableCommands)
	if err != nil {
		return nil, fmt.Errorf("ElevatableCommands: %w", err)
	}
	paths, err := allowedpaths.NewLexicalPolicy(policy.AllowedPaths)
	if err != nil {
		return nil, err
	}
	// Execution's permissive configuration parser skips invalid entries with
	// warnings. Remote policy input is strict: validate before sharing that parser.
	for _, grant := range policy.AllowedSystemServices {
		if err := validateSystemServiceName(grant.Service); err != nil {
			return nil, fmt.Errorf("AllowedSystemServices: %w", err)
		}
		for _, action := range grant.Actions {
			if action != SystemServiceAllActions && !validSystemServiceAction(action) {
				return nil, fmt.Errorf("AllowedSystemServices: unsupported action %q", action)
			}
		}
	}
	services, _ := parseSystemServiceGrants(policy.AllowedSystemServices)
	for _, pair := range policy.Env {
		if strings.IndexByte(pair, '=') <= 0 || strings.ContainsRune(pair, 0) {
			return nil, errors.New("CheckRemote: Env must contain KEY=value pairs without NUL bytes")
		}
	}
	env := &overlayEnviron{parent: expand.ListEnviron(append([]string(nil), policy.Env...)...)}
	for name, value := range map[string]string{
		"PWD": policy.Dir, "IFS": " \t\n", "OPTIND": "1", "RSHELL_VERSION": version.Version,
	} {
		env.setUncapped(name, expand.Variable{Set: true, Kind: expand.String, Str: value})
	}
	state := &checkState{
		env: env, values: map[string]checkValue{}, dir: policy.Dir, dirKnown: policy.Dir != "",
		budget: &checkBudget{ctx: ctx},
	}
	if policy.Dir == "" {
		state.values["PWD"] = checkValue{}
	}
	if policy.AllowedPaths != nil {
		state.values["ALLOWED_PATHS"] = checkValue{}
	}
	registerBuiltins()
	return checkScriptPolicy(ctx, script, checkPolicy{
		allowedCommands: commands, elevatableCommands: elevatable, elevationEnabled: true,
		remediationMode: policy.Mode == ModeRemediation, systemServices: services,
		paths: paths, pathsConfigured: policy.AllowedPaths != nil, remote: true,
	}, state, nil)
}
