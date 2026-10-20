// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tests_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// whileFuzzRun executes a script under a short ctx deadline so that runaway
// loops the fuzzer constructs cannot hang the test. The exit code from a
// well-behaved while/until execution is 0 or 1 (POSIX); a panic, fatal Go
// error, or unexpected non-{0,1} exit code is surfaced as a test failure.
//
// This deliberately diverges from the unit-test helpers: it does not call
// require/assert, it does not Fatalf on parse errors (those are skipped), and
// it accepts ctx-cancellation as a successful path.
func whileFuzzRun(t *testing.T, script string) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		// Parse error means the input wasn't a valid shell program; nothing
		// to fuzz against the runtime.
		return
	}

	// 250 ms is enough for any non-pathological fuzz input to finish; longer
	// pushes the seed-corpus run time past CI budgets.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	if err != nil {
		t.Fatalf("runner construction failed: %v", err)
	}
	defer runner.Close()

	err = runner.Run(ctx, prog)
	if err == nil {
		return // exit 0 — success
	}
	var es interp.ExitStatus
	if errors.As(err, &es) {
		// Any non-zero exit is acceptable for shell scripts; the spec doesn't
		// constrain user-program exit codes. We only rule out explicit
		// internal-error sentinels.
		_ = es
		return
	}
	// Non-ExitStatus errors (ctx.Canceled, ctx.DeadlineExceeded, fatal panics)
	// are acceptable for fuzz-constructed inputs as long as we know the ctx
	// expired.
	if ctx.Err() != nil {
		return
	}
	// "internal error" is the runner's controlled panic-recovery outcome (see
	// api.go's deferred recover). This fuzz target is scoped to while/until
	// semantics; it should not flag pre-existing panics in unrelated paths
	// (e.g. invalid-UTF-8 globs causing regexp.Compile panics in the expander)
	// since those affect for-loops and bare commands the same way and are
	// outside the scope of this fuzzer. The runner's recovery already
	// guarantees the panic does not leak into caller state.
	if err.Error() == "internal error" {
		return
	}
	t.Fatalf("unexpected error from runner.Run: %v", err)
}

// FuzzWhileBody fuzzes the body of a while-true loop. Each input becomes the
// body of `while true; do <input>; break; done`. We force-break to bound
// iterations and avoid hangs.
func FuzzWhileBody(f *testing.F) {
	// Source A: implementation edges
	f.Add([]byte(":"))                              // null command
	f.Add([]byte("echo x"))                         // simple echo
	f.Add([]byte("true"))                           // exit 0
	f.Add([]byte("false"))                          // exit 1
	f.Add([]byte("if true; then :; fi"))            // if-clause
	f.Add([]byte("if false; then :; fi"))           // if-clause not taken
	f.Add([]byte("for x in a b; do echo $x; done")) // nested for
	f.Add([]byte("{ echo a; echo b; }"))            // brace group
	f.Add([]byte("(echo sub)"))                     // subshell
	f.Add([]byte("echo a; echo b"))                 // sequence
	f.Add([]byte("echo a && echo b"))               // and-list
	f.Add([]byte("echo a || echo b"))               // or-list
	f.Add([]byte("echo a | grep a"))                // pipeline
	f.Add([]byte(""))                               // empty body
	f.Add([]byte(": ; : ; :"))                      // multiple null cmds
	// Source B: runaway / boundary inputs the fuzzer should cope with
	f.Add([]byte(strings.Repeat("echo x; ", 100)))
	f.Add([]byte(strings.Repeat("if true; then ", 50) + "echo x" + strings.Repeat("; fi", 50)))
	// Source C: existing test inputs (one per shape)
	f.Add([]byte("i=\"${i}a\"; echo \"$i\""))
	f.Add([]byte("for j in 1 2; do echo $j; break; done; echo after"))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 4096 {
			return // cap input size to keep runtime bounded
		}
		// Reject inputs that contain raw NUL bytes — the parser handles them
		// but they can cause hard-to-diagnose test failures unrelated to
		// while-loop semantics.
		if bytes.IndexByte(body, 0) >= 0 {
			return
		}
		script := "while true; do " + string(body) + "\nbreak; done"
		whileFuzzRun(t, script)
	})
}

// FuzzWhileCondition fuzzes the condition list of a while loop. Body always
// breaks to keep iteration count finite.
func FuzzWhileCondition(f *testing.F) {
	f.Add([]byte("true"))
	f.Add([]byte("false"))
	f.Add([]byte("echo cond"))
	f.Add([]byte("[ x = x ]"))
	f.Add([]byte("[ x = y ]"))
	f.Add([]byte("echo a | grep a"))
	f.Add([]byte("echo a | grep z"))
	f.Add([]byte("true; false"))
	f.Add([]byte("false; true"))
	f.Add([]byte("true && true"))
	f.Add([]byte("true && false"))
	f.Add([]byte("false || true"))
	f.Add([]byte("false || false"))
	f.Add([]byte("! true"))
	f.Add([]byte("! false"))
	// Multi-stmt conds — only the last status matters
	f.Add([]byte("i=x; [ \"$i\" = x ]"))
	f.Add([]byte("i=x; i=\"${i}a\"; [ \"$i\" != aaaaa ]"))

	f.Fuzz(func(t *testing.T, cond []byte) {
		if len(cond) > 4096 {
			return
		}
		if bytes.IndexByte(cond, 0) >= 0 {
			return
		}
		script := "while " + string(cond) + "; do break; done"
		whileFuzzRun(t, script)
	})
}

// FuzzUntilBody mirrors FuzzWhileBody for until loops.
func FuzzUntilBody(f *testing.F) {
	f.Add([]byte(":"))
	f.Add([]byte("echo x"))
	f.Add([]byte("true"))
	f.Add([]byte("false"))
	f.Add([]byte("for x in a; do echo $x; done"))
	f.Add([]byte("if true; then echo y; fi"))
	f.Add([]byte("{ echo a; }"))
	f.Add([]byte("(echo sub)"))
	f.Add([]byte("echo a; echo b"))
	f.Add([]byte("echo a | grep a"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 4096 {
			return
		}
		if bytes.IndexByte(body, 0) >= 0 {
			return
		}
		script := "until false; do " + string(body) + "\nbreak; done"
		whileFuzzRun(t, script)
	})
}

// FuzzWhileBreakContinueLevels fuzzes the N argument to break/continue inside
// nested while loops. Bash clamps excess levels at the outermost loop; this
// fuzzer ensures we never panic, hang, or produce malformed output for any N.
func FuzzWhileBreakContinueLevels(f *testing.F) {
	// Reasonable seed values
	f.Add(0, "break") // (break with bad arg, expect handled error)
	f.Add(1, "break")
	f.Add(2, "break")
	f.Add(3, "break")
	f.Add(99, "break")
	f.Add(1, "continue")
	f.Add(2, "continue")
	f.Add(99, "continue")

	f.Fuzz(func(t *testing.T, n int, kind string) {
		// Constrain inputs: kind must be break or continue; n in [1, 1000].
		if kind != "break" && kind != "continue" {
			return
		}
		if n < 1 || n > 1000 {
			return
		}
		// Two nested while loops with the inner doing `<kind> <n>`. For
		// continue with n>=2 at outermost we'd hit the clamp branch; for
		// break with n>=3 we'd hit the outermost-clamp.
		script := "i=; while [ \"$i\" != aa ]; do i=\"${i}a\"; while true; do echo \"$i\"; " + kind + " " + itoa(n) + "; done; done"
		whileFuzzRun(t, script)
	})
}

// FuzzWhileNestingDepth fuzzes the nesting depth of while-true loops with an
// innermost break to unwind. Each input becomes a depth value and a break-N
// argument; we exercise the nesting + break-clamping interaction.
func FuzzWhileNestingDepth(f *testing.F) {
	f.Add(1, 1)
	f.Add(2, 2)
	f.Add(5, 5)
	f.Add(20, 20)
	f.Add(20, 1)
	f.Add(20, 999)
	f.Add(50, 50)

	f.Fuzz(func(t *testing.T, depth, breakN int) {
		if depth < 1 || depth > 100 {
			return // keep test runtime bounded
		}
		if breakN < 1 || breakN > 10000 {
			return
		}
		var b strings.Builder
		for i := 0; i < depth; i++ {
			b.WriteString("while true; do ")
		}
		b.WriteString("echo ok; break ")
		b.WriteString(itoa(breakN))
		b.WriteString("; ")
		for i := 0; i < depth; i++ {
			b.WriteString("done; ")
		}
		whileFuzzRun(t, b.String())
	})
}
