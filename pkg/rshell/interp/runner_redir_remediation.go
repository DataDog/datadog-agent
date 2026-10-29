// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"

	"mvdan.cc/sh/v3/syntax"
)

// redirectFilePerm is the permission bits used when creating redirect target
// files. 0666 lets the process umask determine the final mode (open(2) applies
// mode & ~umask), matching bash's >FILE behaviour and Sandbox.Truncate.
// Hardcoding 0644 here would instead bake in the result for umask 022 only,
// producing a non-group-writable file on hosts with a 002 umask.
// On Windows there is no umask; Go maps the mode to the read-only attribute
// (perm&0200), which both 0644 and 0666 set, so this value is inert there.
const redirectFilePerm os.FileMode = 0666

// statFileMode returns the fs.FileMode for path via the sandbox.
// When r.sandbox is nil, it returns os.ErrNotExist so the caller skips the
// type-check — a nil sandbox routes all opens through sandbox.Open(nil),
// which returns ErrPermission immediately, so a FIFO can never actually block.
func (r *Runner) statFileMode(path string) (fs.FileMode, error) {
	if r.sandbox == nil {
		return 0, os.ErrNotExist
	}
	info, err := r.sandbox.Stat(path, r.Dir)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

// rejectNonRegularRedirectTarget prevents opening a non-regular file (FIFO,
// socket, device) as a write redirect target. Opening a FIFO with O_WRONLY
// blocks until a reader connects, which would hang the script before context
// cancellation fires. Sandbox.Stat is openat-based and never blocks.
//
// /dev/null is handled by the io.Discard fast path and never reaches here.
// ENOENT is ignored — O_CREATE will create a regular file; other open
// failures surface from the subsequent Open call. When r.sandbox is nil,
// the guard is skipped.
//
// There is a TOCTOU window between Stat and Open; it is not a sandbox-escape
// risk because the sandbox enforces path containment atomically via openat.
func (r *Runner) rejectNonRegularRedirectTarget(path string) error {
	mode, err := r.statFileMode(path)
	if err != nil {
		return nil // ENOENT or other: let Open surface the real error
	}
	if mode&fs.ModeType == 0 {
		return nil // regular file
	}
	werr := fmt.Errorf("open %s: not a regular file", path)
	r.errf("%v\n", werr)
	return werr
}

// openWriteRedirect opens arg for writing and assigns it to *orig.
// Used by >, >|, and >> in remediation mode.
func (r *Runner) openWriteRedirect(ctx context.Context, op syntax.RedirOperator, arg string, orig *io.Writer) (io.Closer, error) {
	if err := r.rejectNonRegularRedirectTarget(arg); err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if op == syntax.AppOut {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := r.open(ctx, arg, flags, redirectFilePerm, true)
	if err != nil {
		return nil, err
	}
	*orig = f
	return f, nil
}

// openWriteAllRedirect opens arg for writing and assigns it to both stdout and
// stderr. Used by &> and &>> in remediation mode.
func (r *Runner) openWriteAllRedirect(ctx context.Context, op syntax.RedirOperator, arg string) (io.Closer, error) {
	if err := r.rejectNonRegularRedirectTarget(arg); err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if op == syntax.AppAll {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := r.open(ctx, arg, flags, redirectFilePerm, true)
	if err != nil {
		return nil, err
	}
	r.stdout = f
	r.stderr = f
	return f, nil
}
