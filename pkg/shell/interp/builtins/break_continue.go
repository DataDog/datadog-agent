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
			callCtx.Errf("%s: %s: numeric argument required\n", name, args[0])
			// In bash, invalid args still break the loop.
			return Result{Code: 2, BreakN: 1}
		}
		if parsed < 1 {
			callCtx.Errf("%s: %s: loop count out of range\n", name, args[0])
			return Result{Code: 1, BreakN: 1}
		}
		n = parsed
	default:
		callCtx.Errf("usage: %s [n]\n", name)
		// In bash, invalid args still break the loop.
		return Result{Code: 2, BreakN: 1}
	}

	var r Result
	if name == "break" {
		r.BreakN = n
	} else {
		r.ContinueN = n
	}
	return r
}
