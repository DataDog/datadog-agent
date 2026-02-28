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
	"strings"
)

// builtins is the set of commands implemented directly in the interpreter.
var builtins = map[string]bool{
	"echo":     true,
	"true":     true,
	"false":    true,
	"break":    true,
	"continue": true,
	"cd":       true,
	"pwd":      true,
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
	case "break":
		err = r.builtinBreak()
	case "continue":
		err = r.builtinContinue()
	case "cd":
		err = r.builtinCd(args)
	case "pwd":
		err = r.builtinPwd(args)
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
