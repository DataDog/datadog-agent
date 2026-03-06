// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
// It prints "<cmd>: not found" to stderr and returns exit code 127,
// without ever searching PATH or executing host binaries.
func noExecHandler() ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		fmt.Fprintf(hc.Stderr, "%s: not found\n", args[0])
		return ExitStatus(127)
	}
}

func checkStat(dir, file string, checkExec bool) (string, error) {
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	m := info.Mode()
	if m.IsDir() {
		return "", fmt.Errorf("is a directory")
	}
	if checkExec && runtime.GOOS != "windows" && m&0o111 == 0 {
		return "", fmt.Errorf("permission denied")
	}
	return file, nil
}

// findExecutable returns the path to an existing executable file.
func findExecutable(dir, file string) (string, error) {
	return checkStat(dir, file, true)
}

// LookPathDir is similar to [os/exec.LookPath], with the difference that it uses the
// provided environment. env is used to fetch relevant environment variables
// such as PWD and PATH.
//
// If no error is returned, the returned path must be valid.
func LookPathDir(cwd string, env expand.Environ, file string) (string, error) {
	return lookPathDir(cwd, env, file, findExecutable)
}

// findAny defines a function to pass to [lookPathDir].
type findAny = func(dir string, file string) (string, error)

func lookPathDir(cwd string, env expand.Environ, file string, find findAny) (string, error) {
	if find == nil {
		panic("no find function found")
	}

	pathList := filepath.SplitList(env.Get("PATH").String())
	if len(pathList) == 0 {
		pathList = []string{""}
	}
	chars := `/`
	if runtime.GOOS == "windows" {
		chars = `:\/`
	}
	if strings.ContainsAny(file, chars) {
		return find(cwd, file)
	}
	for _, elem := range pathList {
		var path string
		switch elem {
		case "", ".":
			// otherwise "foo" won't be "./foo"
			path = "." + string(filepath.Separator) + file
		default:
			path = filepath.Join(elem, file)
		}
		if f, err := find(cwd, path); err == nil {
			return f, nil
		}
	}
	return "", fmt.Errorf("%q: executable file not found in $PATH", file)
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

