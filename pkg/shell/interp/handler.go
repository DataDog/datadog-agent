// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// HandlerCtx returns HandlerContext value stored in ctx.
// It panics if ctx has no HandlerContext stored.
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

// defaultOpenHandler returns the [OpenHandlerFunc] used by default.
// It uses [os.OpenFile] to open files.
//
// On Windows, /dev/null is transparently mapped to NUL (the Windows
// equivalent) so that shell scripts using /dev/null work cross-platform.
func defaultOpenHandler() OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		mc := HandlerCtx(ctx)
		if runtime.GOOS == "windows" && path == "/dev/null" {
			// /dev/null is always safe to open: it returns EOF on read
			// and discards writes. Map it to NUL, the Windows equivalent.
			path = "NUL"
			// Work around https://go.dev/issue/71752, where Go 1.24 started giving
			// "Invalid handle" errors when opening "NUL" with O_TRUNC.
			// TODO: hopefully remove this in the future once the bug is fixed.
			flag &^= os.O_TRUNC
		} else if path != "" && !filepath.IsAbs(path) {
			path = filepath.Join(mc.Dir, path)
		}
		return os.OpenFile(path, flag, perm)
	}
}

// ReadDirHandlerFunc is a handler which reads directories. It is called during
// shell globbing, if enabled.
type ReadDirHandlerFunc func(ctx context.Context, path string) ([]fs.DirEntry, error)

// defaultReadDirHandler returns the [ReadDirHandlerFunc] used by default.
// It uses [os.ReadDir].
func defaultReadDirHandler() ReadDirHandlerFunc {
	return func(ctx context.Context, path string) ([]fs.DirEntry, error) {
		return os.ReadDir(path)
	}
}
