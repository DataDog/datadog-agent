// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"strconv"
)

func builtinExit(_ context.Context, callCtx *CallContext, args []string) Result {
	var r Result
	switch len(args) {
	case 0:
		r.Code = callCtx.LastExitCode
	case 1:
		n, err := strconv.Atoi(args[0])
		if err != nil {
			callCtx.Errf("invalid exit status code: %q\n", args[0])
			r.Code = 2
			// In sh, exit with invalid args still terminates the shell.
			r.Exiting = true
			return r
		}
		r.Code = uint8(n)
	default:
		callCtx.Errf("exit cannot take multiple arguments\n")
		r.Code = 1
		// In sh, exit with too many args still terminates the shell.
		r.Exiting = true
		return r
	}
	r.Exiting = true
	return r
}
