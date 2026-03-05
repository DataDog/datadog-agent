// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import "context"

func builtinEcho(_ context.Context, call *CallContext, args []string) Result {
	for i, arg := range args {
		if i > 0 {
			call.Out(" ")
		}
		call.Out(arg)
	}
	call.Out("\n")
	return Result{}
}
