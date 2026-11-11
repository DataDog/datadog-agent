// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package breakcmd implements the break builtin command.
//
// break — exit from a for, while, or until loop
//
// Usage: break [N]
//
// Exit from the innermost enclosing loop. If N is specified,
// break out of N enclosing loops.
//
// Exit codes:
//
//	0  Loop exited successfully.
//	1  Not inside a loop, or invalid argument.
package breakcmd

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/loopctl"
)

const helpText = "break: break [n]\n" +
	"    Exit for, while, or until loops.\n" +
	"    \n" +
	"    Exit a FOR, WHILE or UNTIL loop.  If N is specified, break N enclosing\n" +
	"    loops.\n" +
	"    \n" +
	"    Exit Status:\n" +
	"    The exit status is 0 unless N is not greater than or equal to 1."

// Cmd is the break builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "break",
	Description: "exit from a loop",
	MakeFlags:   builtins.NoFlags(run),
}

func run(_ context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	if len(args) > 0 && args[0] == "--help" {
		callCtx.Outf("%s\n", helpText)
		return builtins.Result{Code: 2}
	}
	return loopctl.LoopControl(callCtx, "break", args)
}
