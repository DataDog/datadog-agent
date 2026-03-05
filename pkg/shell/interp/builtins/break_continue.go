// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"strconv"
)

func builtinBreak(_ context.Context, call *CallContext, args []string) Result {
	return loopControl(call, "break", args)
}

func builtinContinue(_ context.Context, call *CallContext, args []string) Result {
	return loopControl(call, "continue", args)
}

func loopControl(call *CallContext, name string, args []string) Result {
	if !call.InLoop {
		call.Errf("%s is only useful in a loop\n", name)
		return Result{}
	}

	n := 1
	switch len(args) {
	case 0:
	case 1:
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			call.Errf("usage: %s [n]\n", name)
			return Result{Code: 2}
		}
		n = parsed
	default:
		call.Errf("usage: %s [n]\n", name)
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
