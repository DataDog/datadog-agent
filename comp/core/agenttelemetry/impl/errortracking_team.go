// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"path"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

const repoModulePrefix = "github.com/DataDog/datadog-agent/"

// Frames under these repository paths are logging plumbing between the call
// site and the handler. They are never the root cause of an error log.
var plumbingPrefixes = []string{
	"pkg/util/log/",
	"comp/core/log/",
	"comp/core/agenttelemetry/",
}

// Frames under these repository paths are shared helpers that many teams call
// when they are owned by the same team as the prefix itself (the generic
// helpers); a package under a prefix with its own CODEOWNERS owner, such as
// pkg/util/kubernetes, keeps that owner. When a generic helper logs an error,
// the failing caller is the better owner, so the attribution walks down to the
// first frame outside the helpers.
var sharedHelperPrefixes = []string{
	"pkg/util/",
}

// stackFrame is the part of a symbolized frame that attribution needs.
type stackFrame struct {
	function string
	file     string
}

// teamTag returns the tag suffix naming the team that owns the root cause of
// the error log, or "" when no owner resolves.
func teamTag(e errortracking.ErrorLog) string {
	if e.PCsLen == 0 {
		return ""
	}
	frames := make([]stackFrame, 0, e.PCsLen)
	callers := runtime.CallersFrames(e.PCs[:e.PCsLen])
	for {
		frame, more := callers.Next()
		frames = append(frames, stackFrame{function: frame.Function, file: frame.File})
		if !more {
			break
		}
	}
	if team := owningTeam(codeOwnersRules(), frames); team != "" {
		return ",team:" + team
	}
	return ""
}

// owningTeam picks the team for a stack ordered from the innermost frame
// outward. It takes the first repository frame that is not plumbing; if that
// frame lives in a shared helper package it keeps walking to the first
// non-helper frame that has an owner, and falls back to the helper frame's owner
// when there is none. Standard library and third-party frames never produce a team.
func owningTeam(rules []ownerRule, frames []stackFrame) string {
	first, seenFirst := "", false
	for _, f := range frames {
		p, ok := repoPath(f)
		if !ok || hasAnyPrefix(p, plumbingPrefixes) {
			continue
		}
		team := ownerOfPath(rules, p)
		if !seenFirst {
			first, seenFirst = team, true
		}
		if team != "" && !isSharedHelper(rules, p, team) {
			return team
		}
	}
	return first
}

// isSharedHelper reports whether path is in a generic helper package: under a
// shared helper prefix and owned by the same team as the prefix itself.
func isSharedHelper(rules []ownerRule, p, team string) bool {
	for _, prefix := range sharedHelperPrefixes {
		if strings.HasPrefix(p, prefix) && team == ownerOfPath(rules, prefix+"x.go") {
			return true
		}
	}
	return false
}

// repoPath returns the repository-relative path of a Go source frame. It reads
// the path from the file when the build recorded the module path (trimmed
// paths) or a GOPATH-style checkout. Otherwise it joins the package directory
// from the function name with the file name, which holds for any checkout
// location. Package main has no module path in its function name, so a
// non-trimmed absolute path is cut at its last "/cmd/" directory; release
// builds are not guaranteed to use -trimpath.
func repoPath(f stackFrame) (string, bool) {
	if !strings.HasSuffix(f.file, ".go") {
		return "", false
	}
	if i := strings.Index(f.file, repoModulePrefix); i >= 0 {
		return f.file[i+len(repoModulePrefix):], true
	}
	rest, ok := strings.CutPrefix(f.function, repoModulePrefix)
	if !ok {
		if i := strings.LastIndex(f.file, "/cmd/"); i >= 0 && strings.HasPrefix(f.function, "main.") {
			return f.file[i+1:], true
		}
		return "", false
	}
	slash := strings.LastIndex(rest, "/")
	dot := strings.Index(rest[slash+1:], ".")
	if dot < 0 {
		return "", false
	}
	return rest[:slash+1+dot] + "/" + path.Base(f.file), true
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
