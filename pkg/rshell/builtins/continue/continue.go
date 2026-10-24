// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package continuecmd implements the continue builtin command.
//
// continue — resume the next iteration of a for, while, or until loop
//
// Usage: continue [N]
//
// Resume the next iteration of the innermost enclosing loop. If N is
// specified, resume the Nth enclosing loop.
//
// Exit codes:
//
//	0  Iteration resumed successfully.
//	1  Not inside a loop, or invalid argument.
package continuecmd

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/loopctl"
)

const helpText = "continue: continue [n]\n" +
	"    Resume for, while, or until loops.\n" +
	"    \n" +
	"    Resumes the next iteration of the enclosing FOR, WHILE or UNTIL loop.\n" +
	"    If N is specified, resumes the Nth enclosing loop.\n" +
	"    \n" +
	"    Exit Status:\n" +
	"    The exit status is 0 unless N is not greater than or equal to 1."

// Cmd is the continue builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "continue",
	Description: "continue a loop iteration",
	MakeFlags:   builtins.NoFlags(run),
}

func run(_ context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	if len(args) > 0 && args[0] == "--help" {
		callCtx.Outf("%s\n", helpText)
		return builtins.Result{Code: 2}
	}
	return loopctl.LoopControl(callCtx, "continue", args)
}
