// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// IsBuiltin returns true if the given word is a shell builtin.
func IsBuiltin(name string) bool {
	switch name {
	case "true", "false", "exit", "echo", "break", "continue", "pwd", "cd", "test", "[":
		return true
	}
	return false
}

// TODO: atoi is duplicated in the expand package.

// atoi is like [strconv.ParseInt](s, 10, 64), but it ignores errors and trims whitespace.
func atoi(s string) int64 {
	s = strings.TrimSpace(s)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

type errBuiltinExitStatus exitStatus

func (e errBuiltinExitStatus) Error() string {
	return fmt.Sprintf("builtin exit status %d", e.code)
}

// Builtin allows [ExecHandlerFunc] implementations to execute any builtin,
// which can be useful for an exec handler to wrap or combine builtin calls.
//
// Note that a non-nil error may be returned in cases where the builtin
// alters the control flow of the runner, even if the builtin did not fail.
// For example, this is the case with `exit 0` or `return`.
func (hc HandlerContext) Builtin(ctx context.Context, args []string) error {
	if hc.kind != handlerKindExec {
		return fmt.Errorf("HandlerContext.Builtin can only be called via an ExecHandlerFunc")
	}
	exit := hc.runner.builtin(ctx, hc.Pos, args[0], args[1:])
	if exit != (exitStatus{}) {
		return errBuiltinExitStatus(exit)
	}
	return nil
}

func (r *Runner) builtin(ctx context.Context, pos syntax.Pos, name string, args []string) (exit exitStatus) {
	failf := func(code uint8, format string, args ...any) exitStatus {
		r.errf(format, args...)
		exit.code = code
		return exit
	}
	switch name {
	case "true":
	case "false":
		exit.code = 1
	case "exit":
		switch len(args) {
		case 0:
			exit = r.lastExit
		case 1:
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return failf(2, "invalid exit status code: %q\n", args[0])
			}
			exit.code = uint8(n)
		default:
			return failf(1, "exit cannot take multiple arguments\n")
		}
		exit.exiting = true
	case "echo":
		newline, doExpand := true, false
	echoOpts:
		for len(args) > 0 {
			switch args[0] {
			case "-n":
				newline = false
			case "-e":
				doExpand = true
			case "-E": // default
			default:
				break echoOpts
			}
			args = args[1:]
		}
		for i, arg := range args {
			if i > 0 {
				r.out(" ")
			}
			if doExpand {
				arg, _, _ = expand.Format(r.ecfg, arg, nil)
			}
			r.out(arg)
		}
		if newline {
			r.out("\n")
		}
	case "break", "continue":
		if !r.inLoop {
			return failf(0, "%s is only useful in a loop\n", name)
		}
		enclosing := &r.breakEnclosing
		if name == "continue" {
			enclosing = &r.contnEnclosing
		}
		switch len(args) {
		case 0:
			*enclosing = 1
		case 1:
			if n, err := strconv.Atoi(args[0]); err == nil {
				*enclosing = n
				break
			}
			fallthrough
		default:
			return failf(2, "usage: %s [n]\n", name)
		}
	case "pwd":
		evalSymlinks := false
		for len(args) > 0 {
			switch args[0] {
			case "-L":
				evalSymlinks = false
			case "-P":
				evalSymlinks = true
			default:
				return failf(2, "invalid option: %q\n", args[0])
			}
			args = args[1:]
		}
		pwd := r.envGet("PWD")
		if evalSymlinks {
			var err error
			pwd, err = filepath.EvalSymlinks(pwd)
			if err != nil {
				exit.fatal(err) // perhaps overly dramatic?
				return exit
			}
		}
		r.outf("%s\n", pwd)
	case "cd":
		var path string
		switch len(args) {
		case 0:
			path = r.envGet("HOME")
		case 1:
			path = args[0]

			// replicate the commonly implemented behavior of `cd -`
			// ref: https://www.man7.org/linux/man-pages/man1/cd.1p.html#OPERANDS
			if path == "-" {
				path = r.envGet("OLDPWD")
				r.outf("%s\n", path)
			}
		default:
			return failf(2, "usage: cd [dir]\n")
		}
		exit.code = r.changeDir(ctx, path)
	case "[":
		if len(args) == 0 || args[len(args)-1] != "]" {
			return failf(2, "%v: [: missing matching ]\n", pos)
		}
		args = args[:len(args)-1]
		fallthrough
	case "test":
		parseErr := false
		p := testParser{
			rem: args,
			err: func(err error) {
				r.errf("%v: %v\n", pos, err)
				parseErr = true
			},
		}
		p.next()
		expr := p.classicTest("[", false)
		if parseErr {
			exit.code = 2
			return exit
		}
		exit.oneIf(r.bashTest(ctx, expr, true) == "")
	default:
		return failf(2, "%s: unimplemented builtin\n", name)
	}
	return exit
}

func (r *Runner) changeDir(ctx context.Context, path string) uint8 {
	path = cmp.Or(path, ".")
	path = r.absPath(path)
	info, err := r.stat(ctx, path)
	if err != nil || !info.IsDir() {
		return 1
	}
	if r.access(ctx, path, access_X_OK) != nil {
		return 1
	}
	r.Dir = path
	r.setVarString("OLDPWD", r.envGet("PWD"))
	r.setVarString("PWD", path)
	return 0
}

func absPath(dir, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return filepath.Clean(path) // TODO: this clean is likely unnecessary
}

func (r *Runner) absPath(path string) string {
	return absPath(r.Dir, path)
}

// flagParser is used to parse builtin flags.
//
// It's similar to the getopts implementation, but with some key differences.
// First, the API is designed for Go loops, making it easier to use directly.
// Second, it doesn't require the awkward ":ab" syntax that getopts uses.
// Third, it supports "-a" flags as well as "+a".
type flagParser struct {
	current   string
	remaining []string
}

func (p *flagParser) more() bool {
	if p.current != "" {
		// We're still parsing part of "-ab".
		return true
	}
	if len(p.remaining) == 0 {
		// Nothing left.
		p.remaining = nil
		return false
	}
	arg := p.remaining[0]
	if arg == "--" {
		// We explicitly stop parsing flags.
		p.remaining = p.remaining[1:]
		return false
	}
	if len(arg) == 0 || (arg[0] != '-' && arg[0] != '+') {
		// The next argument is not a flag.
		return false
	}
	// More flags to come.
	return true
}

func (p *flagParser) flag() string {
	arg := p.current
	if arg == "" {
		arg = p.remaining[0]
		p.remaining = p.remaining[1:]
	} else {
		p.current = ""
	}
	if len(arg) > 2 {
		// We have "-ab", so return "-a" and keep "-b".
		p.current = arg[:1] + arg[2:]
		arg = arg[:2]
	}
	return arg
}

func (p *flagParser) value() string {
	if len(p.remaining) == 0 {
		return ""
	}
	arg := p.remaining[0]
	p.remaining = p.remaining[1:]
	return arg
}

func (p *flagParser) args() []string { return p.remaining }
