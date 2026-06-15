// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"path/filepath"
	"strings"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// rshell execution modes (mirror rshell's --mode values).
const (
	modeReadOnly    = "read-only"
	modeRemediation = "remediation"
)

const (
	// agentMaxTimeout is the cluster agent's absolute ceiling on exec_command runtime.
	agentMaxTimeout = 60 * time.Second
	// agentDefaultTimeout is applied when the payload requests no timeout.
	agentDefaultTimeout = 30 * time.Second
)

// agentMaxAllowedCommands is the cluster agent's absolute ceiling of rshell commands
// permitted for exec_command. The effective allow-list for any action is the
// intersection of this set, the payload's requested set, and (when configured) the
// operator's dca-local restriction. Nothing can widen beyond this set. It is limited
// to read-only inspection builtins.
var agentMaxAllowedCommands = map[string]struct{}{
	"rshell:cat":     {},
	"rshell:grep":    {},
	"rshell:ls":      {},
	"rshell:head":    {},
	"rshell:tail":    {},
	"rshell:wc":      {},
	"rshell:find":    {},
	"rshell:cut":     {},
	"rshell:sort":    {},
	"rshell:uniq":    {},
	"rshell:tr":      {},
	"rshell:sed":     {},
	"rshell:strings": {},
	"rshell:echo":    {},
	"rshell:printf":  {},
	"rshell:pwd":     {},
	"rshell:uname":   {},
	"rshell:test":    {},
	"rshell:true":    {},
	"rshell:false":   {},
}

// effectivePolicy is the narrowed-down policy applied to an exec_command action.
type effectivePolicy struct {
	allowedCommands []string
	allowedPaths    []string
	mode            string
	timeout         time.Duration
}

// resolveExecPolicy computes the effective policy for an exec_command as the
// intersection of the payload's requested policy, the operator's dca-local
// restriction (when configured), and the cluster agent's absolute maximum. The
// result never widens beyond the agent maximum.
func resolveExecPolicy(params *kubeactions.ExecCommandParams) effectivePolicy {
	return effectivePolicy{
		allowedCommands: resolveAllowedCommands(params.GetAllowedCommands()),
		allowedPaths:    resolveAllowedPaths(params.GetAllowedPaths()),
		mode:            resolveMode(params.GetMode()),
		timeout:         resolveTimeout(params.GetTimeoutMs()),
	}
}

// resolveAllowedCommands intersects the requested commands with the agent maximum
// and, when configured, the dca-local restriction.
func resolveAllowedCommands(requested []string) []string {
	dcaLocal, hasDCALocal := configuredSet("kubeactions.exec_command.allowed_commands")
	var out []string
	for _, c := range requested {
		if _, ok := agentMaxAllowedCommands[c]; !ok {
			continue
		}
		if hasDCALocal {
			if _, ok := dcaLocal[c]; !ok {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// resolveAllowedPaths keeps the requested paths, filtered to those contained within
// the operator's dca-local allowed roots when that restriction is configured.
func resolveAllowedPaths(requested []string) []string {
	roots, hasRoots := configuredList("kubeactions.exec_command.allowed_paths")
	if !hasRoots {
		return requested
	}
	var out []string
	for _, p := range requested {
		if pathWithinAny(p, roots) {
			out = append(out, p)
		}
	}
	return out
}

// resolveMode downgrades remediation to read-only unless remediation is permitted.
func resolveMode(requested string) string {
	if requested == "" {
		return modeReadOnly
	}
	if requested == modeRemediation && !pkgconfigsetup.Datadog().GetBool("kubeactions.exec_command.allow_remediation") {
		return modeReadOnly
	}
	return requested
}

// resolveTimeout clamps the requested timeout to the agent maximum, defaulting when unset.
func resolveTimeout(timeoutMs uint32) time.Duration {
	t := time.Duration(timeoutMs) * time.Millisecond
	if t <= 0 {
		return agentDefaultTimeout
	}
	if t > agentMaxTimeout {
		return agentMaxTimeout
	}
	return t
}

// configuredSet returns the config key's value as a set. The boolean reports whether
// a restriction is active: an empty (or unset) list means "no dca-local restriction".
func configuredSet(key string) (map[string]struct{}, bool) {
	values := pkgconfigsetup.Datadog().GetStringSlice(key)
	if len(values) == 0 {
		return nil, false
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set, true
}

// configuredList returns the config key's value as a slice. The boolean reports whether
// a restriction is active: an empty (or unset) list means "no dca-local restriction".
func configuredList(key string) ([]string, bool) {
	values := pkgconfigsetup.Datadog().GetStringSlice(key)
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}

// pathWithinAny reports whether p is equal to or nested under any of the roots.
func pathWithinAny(p string, roots []string) bool {
	cp := filepath.Clean(p)
	for _, root := range roots {
		cr := filepath.Clean(root)
		if cp == cr || strings.HasPrefix(cp, cr+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
