// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package exit implements the exit builtin command.
//
// exit — cause the shell to exit
//
// Usage: exit [N]
//
// Exit the shell with status N. If N is omitted, the exit status is
// that of the last command executed. If N is not a valid integer, the
// shell prints an error and exits with status 2 (matching bash behavior).
//
// Exit codes:
//
//	N    The supplied exit status (truncated to uint8).
//	2    Invalid (non-numeric) argument (shell exits).
//	1    Too many arguments (shell exits).
package exit

import (
	"context"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const helpText = "exit: exit [n]\n" +
	"    Exit the shell.\n" +
	"    \n" +
	"    Exits the shell with a status of N.  If N is omitted, the exit status\n" +
	"    is that of the last command executed."

// Cmd is the exit builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "exit",
	Description: "exit the shell",
	MakeFlags:   builtins.NoFlags(run),
}

func run(_ context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	if len(args) > 0 && args[0] == "--help" {
		callCtx.Outf("%s\n", helpText)
		return builtins.Result{Code: 2}
	}
	var r builtins.Result
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	switch len(args) {
	case 0:
		r.Code = callCtx.LastExitCode
	case 1:
		n, err := strconv.Atoi(args[0])
		if err != nil {
			callCtx.Errf("%s: numeric argument required\n", args[0])
			r.Code = 2
			// In bash, exit with a non-numeric arg prints an error and
			// exits the shell with code 2.
			r.Exiting = true
			return r
		}
		r.Code = uint8(n & 0xff) // bash wraps exit codes mod 256
	default:
		callCtx.Errf("too many arguments\n")
		r.Code = 1
		// In bash, exit with too many args still terminates the shell.
		r.Exiting = true
		return r
	}
	r.Exiting = true
	return r
}
