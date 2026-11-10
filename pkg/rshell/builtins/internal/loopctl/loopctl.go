// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package loopctl bounds command iteration and checks cancellation.
package loopctl

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// unwindAllLoops is used as a break/continue depth that comfortably exceeds
// any loop nesting depth the interpreter could actually reach before its own
// recursion/stack limits kick in first, so the clamp-at-outermost logic in
// interp/runner_exec.go fully unwinds every enclosing loop. It is a plain
// literal (rather than math.MaxInt) to avoid depending on the "math" package,
// and is small enough to avoid any risk of overflow when decremented once
// per enclosing loop.
const unwindAllLoops = 1 << 30

// LoopControl implements the shared logic for the break and continue builtins.
func LoopControl(callCtx *builtins.CallContext, name string, args []string) builtins.Result {
	if !callCtx.InLoop {
		callCtx.Errf("%s is only useful in a loop\n", name)
		return builtins.Result{}
	}

	n := 1
	switch len(args) {
	case 0:
	case 1:
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			callCtx.Errf("%s: %s: numeric argument required\n", name, args[0])
			return builtins.Result{Code: 128, Exiting: true}
		}
		if parsed < 1 {
			callCtx.Errf("%s: %s: loop count out of range\n", name, args[0])
			// Bash unwinds every enclosing loop (not just the innermost one)
			// when the count is out of range — for BOTH break and continue.
			// This differs from a count that merely exceeds the nesting
			// depth (e.g. "continue 100" in two loops), which clamps to the
			// outermost loop and keeps iterating there; an out-of-range
			// count terminates all enclosing loops outright, exactly like
			// "break" with an effectively infinite count. Use BreakN (not
			// ContinueN) with a very large value so the shared
			// clamp-at-outermost logic in interp/runner_exec.go fully
			// unwinds the loop stack instead of resuming iteration anywhere.
			return builtins.Result{Code: 1, BreakN: unwindAllLoops}
		}
		n = parsed
	default:
		callCtx.Errf("%s: too many arguments\n", name)
		return builtins.Result{Code: 1, Exiting: true}
	}

	var r builtins.Result
	if name == "break" {
		r.BreakN = n
	} else {
		r.ContinueN = n
	}
	return r
}
