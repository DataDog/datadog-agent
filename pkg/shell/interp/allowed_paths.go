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
	"strings"
)

// AllowedPaths restricts file and directory access to the specified directories.
// Paths must be absolute directories that exist. When set, only files within
// these directories can be opened, read, or executed. /dev/null is always allowed.
//
// A nil slice (the default) means unrestricted access.
// An empty slice blocks all file access except /dev/null.
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
// /dev/null is always allowed via the next handler.
// For all other paths, the file is opened through os.Root for atomic path validation.
func wrapOpenHandler(roots []*os.Root, allowedPaths []string, next OpenHandlerFunc) OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		if path == "/dev/null" {
			return next(ctx, path, flag, perm)
		}

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

// wrapReadDirHandler returns a ReadDirHandlerFunc2 that restricts directory reads to allowed paths.
func wrapReadDirHandler(roots []*os.Root, allowedPaths []string) ReadDirHandlerFunc2 {
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
		return f.ReadDir(-1)
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
