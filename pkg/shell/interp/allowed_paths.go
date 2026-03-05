// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// AllowedPaths restricts file and directory access to the specified directories.
// Paths must be absolute directories that exist. When set, only files within
// these directories can be opened, read, or executed.
//
// When not set (default), all file access is blocked.
// An empty slice also blocks all file access.
//
// The restriction is enforced using os.Root (Go 1.24+), which uses openat
// syscalls for atomic path validation — immune to symlink and ".." traversal attacks.
func AllowedPaths(paths []string) RunnerOption {
	return func(r *Runner) error {
		cleaned := make([]string, len(paths))
		for i, p := range paths {
			abs, err := filepath.Abs(p)
			if err != nil {
				return fmt.Errorf("AllowedPaths: cannot resolve %q: %w", p, err)
			}
			info, err := os.Stat(abs)
			if err != nil {
				return fmt.Errorf("AllowedPaths: cannot stat %q: %w", abs, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("AllowedPaths: %q is not a directory", abs)
			}
			cleaned[i] = abs
		}
		r.allowedPaths = cleaned
		return nil
	}
}

// findMatchingRoot returns the matching os.Root and relative path for an absolute path.
// It returns false if no root matches.
func findMatchingRoot(absPath string, roots []*os.Root, allowedPaths []string) (*os.Root, string, bool) {
	for i, ap := range allowedPaths {
		rel, err := filepath.Rel(ap, absPath)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		return roots[i], rel, true
	}
	return nil, "", false
}

// wrapOpenHandler wraps an OpenHandlerFunc to restrict file opens to allowed paths.
// The file is opened through os.Root for atomic path validation.
func wrapOpenHandler(roots []*os.Root, allowedPaths []string) OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		absPath := path
		if !filepath.IsAbs(absPath) {
			hc := HandlerCtx(ctx)
			absPath = filepath.Join(hc.Dir, absPath)
		}

		root, relPath, ok := findMatchingRoot(absPath, roots, allowedPaths)
		if !ok {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
		}

		return root.OpenFile(relPath, flag, perm)
	}
}

// wrapReadDirHandler returns a ReadDirHandlerFunc that restricts directory reads to allowed paths.
func wrapReadDirHandler(roots []*os.Root, allowedPaths []string) ReadDirHandlerFunc {
	return func(ctx context.Context, path string) ([]fs.DirEntry, error) {
		absPath := path
		if !filepath.IsAbs(absPath) {
			hc := HandlerCtx(ctx)
			absPath = filepath.Join(hc.Dir, absPath)
		}

		root, relPath, ok := findMatchingRoot(absPath, roots, allowedPaths)
		if !ok {
			return nil, &os.PathError{Op: "readdir", Path: path, Err: os.ErrPermission}
		}

		f, err := root.Open(relPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		entries, err := f.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		// os.Root's ReadDir does not guarantee sorted order like os.ReadDir.
		// Sort to match POSIX glob expansion expectations.
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			if a.Name() < b.Name() {
				return -1
			}
			if a.Name() > b.Name() {
				return 1
			}
			return 0
		})
		return entries, nil
	}
}

// wrapExecHandler wraps an ExecHandlerFunc to restrict command execution to allowed paths.
// It resolves the command to an absolute path, validates it against allowed roots,
// then delegates to next for actual execution. Returns exit code 127 if not found.
func wrapExecHandler(roots []*os.Root, allowedPaths []string, next ExecHandlerFunc) ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		path, err := LookPathDir(hc.Dir, hc.Env, args[0])
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: not found\n", args[0])
			return ExitStatus(127)
		}

		_, _, ok := findMatchingRoot(path, roots, allowedPaths)
		if !ok {
			fmt.Fprintf(hc.Stderr, "%s: not found\n", args[0])
			return ExitStatus(127)
		}

		return next(ctx, args)
	}
}
