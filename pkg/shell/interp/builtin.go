// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// builtins is the set of commands implemented directly in the interpreter.
// All commands are builtins — the interpreter never executes host binaries.
var builtins = map[string]bool{
	"echo":     true,
	"true":     true,
	"false":    true,
	"test":     true,
	"[":        true,
	"break":    true,
	"continue": true,
	"exit":     true,
	"cd":       true,
	"pwd":      true,
	"ls":       true,
	"head":     true,
	"tail":     true,
	"find":     true,
	"grep":     true,
	"wc":       true,
	"sort":     true,
	"uniq":     true,
}

// builtin executes a builtin command. Returns (true, err) if the command is
// a builtin, (false, nil) if it is not.
func (r *Runner) builtin(_ context.Context, name string, args []string) (bool, error) {
	if !builtins[name] {
		return false, nil
	}

	var err error
	switch name {
	case "echo":
		err = r.builtinEcho(args)
	case "true":
		r.exitCode = 0
	case "false":
		r.exitCode = 1
	case "test":
		err = r.builtinTest(args)
	case "[":
		err = r.builtinBracket(args)
	case "break":
		err = r.builtinBreak()
	case "continue":
		err = r.builtinContinue()
	case "exit":
		err = r.builtinExit(args)
	case "cd":
		err = r.builtinCd(args)
	case "pwd":
		err = r.builtinPwd(args)
	case "ls":
		err = r.builtinLs(args)
	case "head":
		err = r.builtinHead(args)
	case "tail":
		err = r.builtinTail(args)
	case "find":
		err = r.builtinFind(args)
	case "grep":
		err = r.builtinGrep(args)
	case "wc":
		err = r.builtinWc(args)
	case "sort":
		err = r.builtinSort(args)
	case "uniq":
		err = r.builtinUniq(args)
	}

	return true, err
}

func (r *Runner) builtinEcho(args []string) error {
	newline := true
	interpretEscapes := false
	i := 0

	// Parse flags
	for i < len(args) {
		switch args[i] {
		case "-n":
			newline = false
			i++
		case "-e":
			interpretEscapes = true
			i++
		case "-E":
			interpretEscapes = false
			i++
		default:
			goto printArgs
		}
	}

printArgs:
	output := strings.Join(args[i:], " ")
	if interpretEscapes {
		output = interpretEchoEscapes(output)
	}
	if newline {
		output += "\n"
	}
	_, err := fmt.Fprint(r.stdout, output)
	if err == nil {
		r.exitCode = 0
	}
	return nil // echo never fails from the interpreter's perspective
}

func interpretEchoEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case 'a':
				b.WriteByte('\a')
				i++
			case 'b':
				b.WriteByte('\b')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 'v':
				b.WriteByte('\v')
				i++
			case 'c':
				// \c = stop output
				return b.String()
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (r *Runner) builtinBracket(args []string) error {
	if len(args) == 0 || args[len(args)-1] != "]" {
		return fmt.Errorf("[: missing closing ]")
	}
	return r.builtinTest(args[:len(args)-1])
}

func (r *Runner) builtinTest(args []string) error {
	result := evalTest(args, r.dir)
	if result {
		r.exitCode = 0
	} else {
		r.exitCode = 1
	}
	return nil
}

// evalTest evaluates a test expression and returns true/false.
func evalTest(args []string, dir string) bool {
	if len(args) == 0 {
		return false
	}

	// Handle negation: ! expr
	if args[0] == "!" {
		return !evalTest(args[1:], dir)
	}

	// Unary file tests: -e/-f/-d/-s file
	if len(args) == 2 {
		path := args[1]
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		switch args[0] {
		case "-e":
			_, err := os.Stat(path)
			return err == nil
		case "-f":
			info, err := os.Stat(path)
			return err == nil && info.Mode().IsRegular()
		case "-d":
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		case "-s":
			info, err := os.Stat(path)
			return err == nil && info.Size() > 0
		case "-z":
			return args[1] == ""
		case "-n":
			return args[1] != ""
		}
	}

	// Single arg: test string (true if non-empty)
	if len(args) == 1 {
		return args[0] != ""
	}

	// Binary operators: string = string, string != string, int -eq int, etc.
	if len(args) == 3 {
		left, op, right := args[0], args[1], args[2]
		switch op {
		case "=":
			return left == right
		case "!=":
			return left != right
		case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
			l, err1 := strconv.Atoi(left)
			r, err2 := strconv.Atoi(right)
			if err1 != nil || err2 != nil {
				return false
			}
			switch op {
			case "-eq":
				return l == r
			case "-ne":
				return l != r
			case "-lt":
				return l < r
			case "-le":
				return l <= r
			case "-gt":
				return l > r
			case "-ge":
				return l >= r
			}
		}
	}

	return false
}

func (r *Runner) builtinBreak() error {
	if r.loopDepth == 0 {
		return fmt.Errorf("break: not in a loop")
	}
	return errBreak
}

func (r *Runner) builtinContinue() error {
	if r.loopDepth == 0 {
		return fmt.Errorf("continue: not in a loop")
	}
	return errContinue
}

func (r *Runner) builtinExit(args []string) error {
	code := r.exitCode
	if len(args) > 0 {
		var err error
		code, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("exit: invalid exit code %q", args[0])
		}
	}
	return &exitError{code: code}
}

func (r *Runner) builtinCd(args []string) error {
	var target string

	// Skip flags (-L, -P)
	i := 0
	for i < len(args) {
		if args[i] == "-L" || args[i] == "-P" {
			i++
			continue
		}
		break
	}

	if i < len(args) {
		target = args[i]
	} else {
		target = os.Getenv("HOME")
		if target == "" {
			return fmt.Errorf("cd: HOME not set")
		}
	}

	// Resolve relative to current dir.
	if !filepath.IsAbs(target) {
		target = filepath.Join(r.dir, target)
	}

	// Validate the target exists and is a directory.
	info, err := os.Stat(target)
	if err != nil {
		r.exitCode = 1
		return fmt.Errorf("cd: %w", err)
	}
	if !info.IsDir() {
		r.exitCode = 1
		return fmt.Errorf("cd: %s: Not a directory", target)
	}

	r.dir = target
	r.exitCode = 0
	return nil
}

func (r *Runner) builtinPwd(args []string) error {
	// Skip flags (-L, -P)
	for _, a := range args {
		if a != "-L" && a != "-P" {
			break
		}
	}

	fmt.Fprintln(r.stdout, r.dir)
	r.exitCode = 0
	return nil
}
