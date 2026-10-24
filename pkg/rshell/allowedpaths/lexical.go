// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LexicalPolicy checks only the filesystem-independent part of path grants.
// It holds no filesystem handles and cannot perform sandbox operations. A nil
// CheckPath error is NOT an authorization: the target must still check root
// availability, symlinks, access modes after resolution, file types, hard links,
// and OS permissions through a real Sandbox.
type LexicalPolicy struct {
	roots []lexicalRoot
}

type lexicalRoot struct {
	path    string
	mode    pathMode
	literal string // possible POSIX literal directory ending in :ro/:rw
}

// NewLexicalPolicy parses absolute grants without accessing the filesystem.
// Path syntax is native to this build, as in Sandbox. Access suffix parsing and
// root selection are shared with execution. Unlike New, malformed grants are
// errors, and missing local directories cannot remove grants. Colons other than
// the optional terminal access suffix and a Windows volume prefix are rejected
// to avoid accepting misspelled or stacked access modes as directory names.
func NewLexicalPolicy(paths []string) (*LexicalPolicy, error) {
	p := &LexicalPolicy{roots: make([]lexicalRoot, 0, len(paths))}
	for _, raw := range paths {
		path, mode, suffix := splitAllowedPathMode(raw)
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
			return nil, fmt.Errorf("AllowedPaths: %q must be an absolute path without NUL bytes", raw)
		}
		if strings.ContainsRune(path[len(filepath.VolumeName(path)):], ':') {
			return nil, fmt.Errorf("AllowedPaths: %q has an invalid access suffix (expected :ro or :rw)", raw)
		}
		root := lexicalRoot{path: filepath.Clean(path), mode: mode}
		if suffix && filepath.Separator == '/' {
			// Execution preserves an existing literal POSIX path with this
			// suffix as read-only. We cannot discover that remotely. Retain
			// this possible interpretation so it never causes a false denial.
			root.literal = filepath.Clean(raw)
		}
		p.roots = append(p.roots, root)
	}
	return p, nil
}

// CheckPath reports only definite lexical policy denials. A nil error always
// requires target-side authorization; it does not establish remote containment.
// Relative operands require an explicit absolute cwd; no local cwd is read.
func (p *LexicalPolicy) CheckPath(path, cwd string, operation PathOperation) error {
	write := operation == PathWrite || operation == PathRemove
	if !write && operation != PathRead && operation != PathStat && operation != PathLstat {
		return fmt.Errorf("unknown path operation %q", operation)
	}
	if (operation == PathStat || operation == PathLstat) && IsDevNull(path) {
		return nil
	}
	if !filepath.IsAbs(path) && !filepath.IsAbs(cwd) {
		return fmt.Errorf("relative path %q requires an absolute remote working directory", path)
	}
	abs := toAbs(path, cwd)
	root, _, found := resolvePath(p.roots, abs, func(r *lexicalRoot) string { return r.path },
		func(candidate, best *lexicalRoot) bool {
			// Execution also checks canonical roots before writing. Identical
			// lexical roots are aliases, so read-only wins a tie in either order.
			return best.mode == pathModeReadWrite && candidate.mode == pathModeReadOnly
		})
	if !found {
		if !write {
			for _, candidate := range p.roots {
				if candidate.literal != "" && isWithinRoot(candidate.literal, abs) {
					return nil // literal-suffix interpretation needs remote metadata
				}
			}
		}
		return fmt.Errorf("%s %s: no matching path grant", operation, path)
	}
	if write && root.mode != pathModeReadWrite {
		return fmt.Errorf("%s %s: matching path grant is read-only", operation, path)
	}
	return nil
}

// PathAccesses returns declared roots and access modes, not opened roots.
func (p *LexicalPolicy) PathAccesses() []PathAccess {
	paths := make([]PathAccess, len(p.roots))
	for i, root := range p.roots {
		paths[i] = PathAccess{Path: root.path, ReadWrite: root.mode == pathModeReadWrite}
	}
	return paths
}
