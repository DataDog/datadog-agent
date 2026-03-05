// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import "context"

func builtinEcho(_ context.Context, callCtx *CallContext, args []string) Result {
	for i, arg := range args {
		if i > 0 {
			callCtx.Out(" ")
		}
		callCtx.Out(arg)
	}
	callCtx.Out("\n")
	return Result{}
}
