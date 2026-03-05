// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"strconv"
)

func builtinBreak(_ context.Context, callCtx *CallContext, args []string) Result {
	return loopControl(callCtx, "break", args)
}

func builtinContinue(_ context.Context, callCtx *CallContext, args []string) Result {
	return loopControl(callCtx, "continue", args)
}

func loopControl(callCtx *CallContext, name string, args []string) Result {
	if !callCtx.InLoop {
		callCtx.Errf("%s is only useful in a loop\n", name)
		return Result{}
	}

	n := 1
	switch len(args) {
	case 0:
	case 1:
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			callCtx.Errf("usage: %s [n]\n", name)
			return Result{Code: 2}
		}
		n = parsed
	default:
		callCtx.Errf("usage: %s [n]\n", name)
		return Result{Code: 2}
	}

	var r Result
	if name == "break" {
		r.BreakN = n
	} else {
		r.ContinueN = n
	}
	return r
}
