// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"io"
	"os"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
)

var todoPos syntax.Pos // for handlerCtx callers where we don't yet have a position

func (r *Runner) handlerCtx(ctx context.Context, pos syntax.Pos) context.Context {
	hc := HandlerContext{
		Env:    &overlayEnviron{parent: r.writeEnv},
		Dir:    r.Dir,
		Pos:    pos,
		Stdout: r.stdout,
		Stderr: r.stderr,
	}
	if r.stdin != nil { // do not leave hc.Stdin as a typed nil
		hc.Stdin = r.stdin
	}
	return context.WithValue(ctx, handlerCtxKey{}, hc)
}

func (r *Runner) errf(format string, a ...any) {
	fmt.Fprintf(r.stderr, format, a...)
}

func (r *Runner) stop(ctx context.Context) bool {
	if r.exit.exiting {
		return true
	}
	if err := ctx.Err(); err != nil {
		r.exit.fatal(err)
		return true
	}
	return false
}

func (r *Runner) open(ctx context.Context, path string, flags int, mode os.FileMode, print bool) (io.ReadWriteCloser, error) {
	f, err := r.openHandler(r.handlerCtx(ctx, todoPos), path, flags, mode)
	// TODO: support wrapped PathError returned from openHandler.
	switch err.(type) {
	case nil:
		return f, nil
	case *os.PathError:
		err = allowedpaths.PortablePathError(err)
		if print {
			r.errf("%v\n", err)
		}
	default: // handler's custom fatal error
		r.exit.fatal(err)
	}
	return nil, err
}
