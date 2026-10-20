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

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// HandlerCtx returns HandlerContext value stored in ctx.
// It panics if ctx has no HandlerContext stored; this indicates a
// programming error by the caller (e.g. passing a context that did not
// originate from the interpreter).
func HandlerCtx(ctx context.Context) HandlerContext {
	hc, ok := ctx.Value(handlerCtxKey{}).(HandlerContext)
	if !ok {
		panic("interp.HandlerCtx: no HandlerContext in ctx")
	}
	return hc
}

type handlerCtxKey struct{}

// HandlerContext is the data passed to all the handler functions via [context.WithValue].
// It contains some of the current state of the [Runner].
type HandlerContext struct {
	// Env is a read-only version of the interpreter's environment,
	// including environment variables and global variables.
	Env expand.Environ

	// Dir is the interpreter's current directory.
	Dir string

	// Pos is the source position which relates to the operation,
	// such as a [syntax.CallExpr] when calling an [ExecHandlerFunc].
	// It may be invalid if the operation has no relevant position information.
	Pos syntax.Pos

	// Stdin is the interpreter's current standard input reader.
	// It is always an [*os.File], but the type here remains an [io.Reader]
	// due to backwards compatibility.
	Stdin io.Reader
	// Stdout is the interpreter's current standard output writer.
	Stdout io.Writer
	// Stderr is the interpreter's current standard error writer.
	Stderr io.Writer
}

// OpenHandlerFunc is a handler which opens files.
// It is called for all files that are opened directly by the shell,
// such as in redirects.
// Files opened by executed programs are not included.
//
// The path parameter may be relative to the current directory,
// which can be fetched via [HandlerCtx].
//
// Use a return error of type [*os.PathError] to have the error printed to
// stderr and the exit status set to 1.
// Any other error will halt the [Runner] and will be returned via the API.
//
// Note that implementations which do not return [os.File] will cause
// extra files and goroutines for input redirections; see [StdIO].
type OpenHandlerFunc func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error)

// ReadDirHandlerFunc is a handler which reads directories. It is called during
// shell globbing, if enabled.
type ReadDirHandlerFunc func(ctx context.Context, path string) ([]fs.DirEntry, error)

// ExecHandlerFunc is a handler which executes simple commands.
// It is called for all [syntax.CallExpr] nodes
// where the first argument is not a builtin.
//
// Returning a nil error means a zero exit status.
// Other exit statuses can be set by returning or wrapping an [ExitStatus] error,
// and such an error is returned via the API if it is the last statement executed.
// Any other error will halt the [Runner] and will be returned via the API.
type ExecHandlerFunc func(ctx context.Context, args []string) error

// noExecHandler returns an [ExecHandlerFunc] that rejects all commands.
// It prints "rshell: <cmd>: unknown command" to stderr and returns exit
// code 127, without ever searching PATH or executing host binaries.
// When helpAvailable is true, a hint directing the user to 'help' is appended.
func noExecHandler(helpAvailable bool) ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("exec handler called with no arguments")
		}
		hc := HandlerCtx(ctx)
		fmt.Fprintf(hc.Stderr, "rshell: %s: unknown command\n", args[0])
		if helpAvailable {
			fmt.Fprintf(hc.Stderr, "Run 'help' to see available commands.\n")
		}
		return ExitStatus(127)
	}
}
