// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/expand"
)

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
// It prints "<cmd>: command not found" to stderr and returns exit code 127,
// without ever searching PATH or executing host binaries.
func noExecHandler() ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		fmt.Fprintf(hc.Stderr, "%s: command not found\n", args[0])
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

// ExecLookPathDir is similar to [os/exec.LookPath], with the difference that it uses the
// provided environment. env is used to fetch relevant environment variables
// such as PWD and PATH.
//
// If no error is returned, the returned path must be valid.
func ExecLookPathDir(cwd string, env expand.Environ, file string) (string, error) {
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
