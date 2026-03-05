// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

	// TODO(v4): use an os.File for stdin below directly.

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
// Other exit statuses can be set by returning or wrapping a [NewExitStatus] error,
// and such an error is returned via the API if it is the last statement executed.
// Any other error will halt the [Runner] and will be returned via the API.
type ExecHandlerFunc func(ctx context.Context, args []string) error

// NoExecHandler returns an [ExecHandlerFunc] that rejects all commands.
// It prints "<cmd>: not found" to stderr and returns exit code 127,
// without ever searching PATH or executing host binaries.
func NoExecHandler() ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		fmt.Fprintf(hc.Stderr, "%s: not found\n", args[0])
		return ExitStatus(127)
	}
}

// DefaultExecHandler returns the [ExecHandlerFunc] used by default.
// It finds binaries in PATH and executes them.
// When context is cancelled, an interrupt signal is sent to running processes.
// killTimeout is a duration to wait before sending the kill signal.
// A negative value means that a kill signal will be sent immediately.
//
// On Windows, the kill signal is always sent immediately,
// because Go doesn't currently support sending Interrupt on Windows.
// [Runner] defaults to a killTimeout of 2 seconds.
func DefaultExecHandler(killTimeout time.Duration) ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		path, err := LookPathDir(hc.Dir, hc.Env, args[0])
		if err != nil {
			fmt.Fprintln(hc.Stderr, err)
			return ExitStatus(127)
		}
		cmd := exec.Cmd{
			Path:   path,
			Args:   args,
			Env:    execEnv(hc.Env),
			Dir:    hc.Dir,
			Stdin:  hc.Stdin,
			Stdout: hc.Stdout,
			Stderr: hc.Stderr,
		}

		err = cmd.Start()
		if err == nil {
			stopf := context.AfterFunc(ctx, func() {
				if killTimeout <= 0 || runtime.GOOS == "windows" {
					_ = cmd.Process.Signal(os.Kill)
					return
				}
				_ = cmd.Process.Signal(os.Interrupt)
				// TODO: don't sleep in this goroutine if the program
				// stops itself with the interrupt above.
				time.Sleep(killTimeout)
				_ = cmd.Process.Signal(os.Kill)
			})
			defer stopf()

			err = cmd.Wait()
		}

		switch err := err.(type) {
		case *exec.ExitError:
			// Windows and Plan9 do not have support for [syscall.WaitStatus]
			// with methods like Signaled and Signal, so for those, [waitStatus] is a no-op.
			// Note: [waitStatus] is an alias [syscall.WaitStatus]
			if status, ok := err.Sys().(waitStatus); ok && status.Signaled() {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return ExitStatus(128 + status.Signal())
			}
			return ExitStatus(err.ExitCode())
		case *exec.Error:
			// did not start
			fmt.Fprintf(hc.Stderr, "%v\n", err)
			return ExitStatus(127)
		default:
			return err
		}
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
	if checkExec && m&0o111 == 0 {
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
	if strings.ContainsAny(file, `/`) {
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
// such as in redirects, except for named pipes created by process substitutions.
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

// TODO: paths passed to [OpenHandlerFunc] should be cleaned.

// DefaultOpenHandler returns the [OpenHandlerFunc] used by default.
// It uses [os.OpenFile] to open files.
//
// For the sake of portability, /dev/null opens NUL on Windows.
func DefaultOpenHandler() OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		mc := HandlerCtx(ctx)
		if runtime.GOOS == "windows" && path == "/dev/null" {
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

// ReadDirHandlerFunc2 is a handler which reads directories. It is called during
// shell globbing, if enabled.
type ReadDirHandlerFunc2 func(ctx context.Context, path string) ([]fs.DirEntry, error)

// DefaultReadDirHandler2 returns the [ReadDirHandlerFunc2] used by default.
// It uses [os.ReadDir].
func DefaultReadDirHandler2() ReadDirHandlerFunc2 {
	return func(ctx context.Context, path string) ([]fs.DirEntry, error) {
		return os.ReadDir(path)
	}
}

