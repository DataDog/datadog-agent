// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// writeOutcomeWaitTimeout bounds acquisition, writing, and outcome resolution
// together. The caller's earlier deadline and explicit cancellation still win.
const writeOutcomeWaitTimeout = 30 * time.Second

// writeRegularFile preserves sandbox capabilities while resolving abandoned
// writes before telling the builtin whether restoration is safe.
func (r *Runner) writeRegularFile(ctx context.Context, path, dir string, data []byte, expectedIdentity fs.FileInfo) (bool, error) {
	return writeWithOutcome(ctx, func(writeCtx context.Context) (bool, error) {
		return r.sandbox.WriteRegularFile(writeCtx, path, dir, data, expectedIdentity)
	})
}

// Reserve up to one second (at most half the remaining budget) for a writer
// to report its outcome after its own deadline. Using the caller's deadline
// for both phases would leave no time to resolve a deadline-driven abandonment.
func writeWithOutcome(ctx context.Context, write func(context.Context) (bool, error)) (bool, error) {
	now := time.Now()
	deadline := writeOutcomeDeadline(ctx, now)
	outcomeCtx, cancelOutcome := context.WithDeadline(ctx, deadline)
	defer cancelOutcome()
	reserve := min(time.Second, deadline.Sub(now)/2)
	writeCtx, cancelWrite := context.WithDeadline(outcomeCtx, deadline.Add(-reserve))
	defer cancelWrite()

	mutated, err := write(writeCtx)
	if err != nil && allowedpaths.IsWriteOutcomeUnknown(err) {
		if resolvedMutated, resolvedErr, resolved := allowedpaths.WaitForWriteOutcome(outcomeCtx, err); resolved {
			return resolvedMutated, resolvedErr
		}
		err = fmt.Errorf("%w: %w", builtins.ErrWriteOutcomeUnknown, err)
	}
	return mutated, err
}

// writeOutcomeDeadline keeps both phases inside one budget, never extending
// the caller's deadline.
func writeOutcomeDeadline(ctx context.Context, now time.Time) time.Time {
	deadline := now.Add(writeOutcomeWaitTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return deadline
}

// removeWithBudget charges successful removals against the shared run-wide
// limit. Reserving before unlink and refunding failures keeps concurrent callers
// within the cap without charging for nonexistent or inaccessible paths.
func (r *Runner) removeWithBudget(dir, path string) error {
	counter := r.fileRemovalCount
	if counter != nil {
		if counter.Add(1) > builtins.MaxFileRemovalsPerRun {
			counter.Add(-1)
			return fmt.Errorf("%w: limit is %d files per run, across all commands, loops, and subshells",
				builtins.ErrRemoveBudgetExceeded, builtins.MaxFileRemovalsPerRun)
		}
	}
	if err := r.sandbox.Remove(path, dir); err != nil {
		if counter != nil {
			counter.Add(-1)
		}
		return err
	}
	return nil
}

func allowedPathsList(sb *allowedpaths.Sandbox) []builtins.AllowedPath {
	if sb == nil {
		return nil
	}
	paths := sb.PathAccesses()
	out := make([]builtins.AllowedPath, len(paths))
	for i, path := range paths {
		access := builtins.AllowedPathReadOnly
		if path.ReadWrite {
			access = builtins.AllowedPathReadWrite
		}
		out[i] = builtins.AllowedPath{
			Path:   path.Path,
			Access: access,
		}
	}
	return out
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) {
	if r.stop(ctx) {
		return
	}
	r.exit = exitStatus{}
	r.stmtSync(ctx, st)
	r.lastExit = r.exit
}

func (r *Runner) stmtSync(ctx context.Context, st *syntax.Stmt) {
	oldIn, oldOut, oldErr := r.stdin, r.stdout, r.stderr
	var closers []io.Closer
	defer func() {
		r.stdin, r.stdout, r.stderr = oldIn, oldOut, oldErr
		for i := len(closers) - 1; i >= 0; i-- {
			_ = closers[i].Close()
		}
	}()

	if call, ok := st.Cmd.(*syntax.CallExpr); ok {
		r.callExpr(ctx, call, st.Redirs, &closers)
	} else {
		r.applyRedirects(ctx, st.Redirs, &closers)
		if r.exit.ok() && st.Cmd != nil {
			r.cmd(ctx, st.Cmd)
		}
	}

	if st.Negated && !r.exit.exiting {
		wasOk := r.exit.ok()
		r.exit = exitStatus{}
		r.exit.oneIf(wasOk)
	}
}

func (r *Runner) applyRedirects(ctx context.Context, redirs []*syntax.Redirect, closers *[]io.Closer) {
	for _, rd := range redirs {
		cls, err := r.redir(ctx, rd)
		if err != nil {
			r.exit.code = 1
			break
		}
		if cls != nil {
			*closers = append(*closers, cls)
		}
	}
}

func (r *Runner) callExpr(ctx context.Context, cm *syntax.CallExpr, redirs []*syntax.Redirect, closers *[]io.Closer) {
	r.lastExpandExit = exitStatus{}
	collector := r.newFieldCollector(MaxExpandedArgumentsPerCommand + 2)
	nextArg := 0
	// Expand only enough words to identify the command. A denied command must
	// not trigger expansion or command substitutions in the remaining words.
	for nextArg < len(cm.Args) && len(collector.fields) == 0 {
		collector.add(cm.Args[nextArg])
		nextArg++
	}
	// sudo is a marker rather than the dispatched command, so resolve one more
	// field before applying the command and elevation policies.
	for nextArg < len(cm.Args) && len(collector.fields) == 1 && collector.fields[0] == "sudo" {
		collector.add(cm.Args[nextArg])
		nextArg++
	}
	fields := collector.fields
	if !r.exit.ok() {
		return
	}
	if len(fields) > 0 {
		prefixFields := 1
		if fields[0] == "sudo" && len(fields) > 1 {
			prefixFields = 2
		}
		if !collector.setCommandPrefixFields(prefixFields) {
			return
		}
	}
	if len(fields) == 0 {
		r.applyRedirects(ctx, redirs, closers)
		if r.exit.ok() {
			for _, as := range cm.Assigns {
				prev := r.lookupVar(as.Name.Value)
				prev.Local = false

				vr := r.assignVal(prev, as, "")
				if !r.exit.ok() {
					break
				}
				r.setVarWithIndex(prev, as.Name.Value, as.Index, vr)
			}
		}
		// If interpreting the last expansion like $(foo) failed,
		// and the expansion and assignments otherwise succeeded,
		// we need to surface that last exit code.
		if r.exit.ok() {
			r.exit = r.lastExpandExit
		}
		return
	}
	type restoreVar struct {
		name string
		vr   expand.Variable
	}
	type inlineAssignment struct {
		name string
		prev expand.Variable
		vr   expand.Variable
	}
	var restores []restoreVar
	defer func() {
		// cd intentionally writes $PWD and $OLDPWD as part of its semantics.
		isCd := fields[0] == "cd" && r.exit.ok()
		for _, restore := range restores {
			if isCd && (restore.name == "PWD" || restore.name == "OLDPWD") {
				continue
			}
			r.setVarRestore(restore.name, restore.vr)
		}
	}()

	r.call(ctx, cm.Args[0].Pos(), fields, func() ([]string, bool) {
		if !collector.add(cm.Args[nextArg:]...) {
			return nil, false
		}
		fields = collector.fields

		assignments := make([]inlineAssignment, 0, len(cm.Assigns))
		func() {
			// Earlier assignments are visible while expanding later values,
			// but redirects expand against the original environment.
			previousEnv := r.writeEnv
			r.writeEnv = newOverlayEnviron(previousEnv, false)
			defer func() { r.writeEnv = previousEnv }()

			for _, as := range cm.Assigns {
				name := as.Name.Value
				prev := r.lookupVar(name)

				vr := r.assignVal(prev, as, "")
				if !r.exit.ok() {
					return
				}
				vr.Exported = true
				assignments = append(assignments, inlineAssignment{name, prev, vr})
				r.setVar(name, vr)
			}
		}()
		if !r.exit.ok() {
			return nil, false
		}

		r.applyRedirects(ctx, redirs, closers)
		if !r.exit.ok() {
			return nil, false
		}

		seenRestore := map[string]bool{}
		for _, assignment := range assignments {
			if !seenRestore[assignment.name] {
				restores = append(restores, restoreVar{assignment.name, assignment.prev})
				seenRestore[assignment.name] = true
			}
			r.setVar(assignment.name, assignment.vr)
		}
		return fields, r.exit.ok()
	})
}

func (r *Runner) cmd(ctx context.Context, cm syntax.Command) {
	if r.stop(ctx) {
		return
	}

	switch cm := cm.(type) {
	case *syntax.Subshell:
		r2 := r.subshell(false)
		// A pipeline inside (…) should get its own pipeline span, so
		// clear the flag that suppresses nested pipeline spans.
		r2.inPipeline = false
		r2.stmts(ctx, cm.Stmts)
		r.exit = r2.exit
		if !r.exit.limitExit {
			r.exit.exiting = false
		}
		r.totalCount += r2.totalCount
		r.dispatchedCount += r2.dispatchedCount
		r.unallowedCount += r2.unallowedCount
		r.unknownCount += r2.unknownCount
	case *syntax.Block:
		r.stmts(ctx, cm.Stmts)
	case *syntax.CallExpr:
		var closers []io.Closer
		r.callExpr(ctx, cm, nil, &closers)
	case *syntax.BinaryCmd:
		switch cm.Op {
		case syntax.AndStmt, syntax.OrStmt:
			r.stmt(ctx, cm.X)
			if r.breakEnclosing > 0 || r.contnEnclosing > 0 || r.exit.exiting {
				break
			}
			if r.exit.ok() == (cm.Op == syntax.AndStmt) {
				r.stmt(ctx, cm.Y)
			}
		case syntax.Pipe:
			if !r.inPipeline {
				var span rshellTelemetrySpan
				span, ctx = startTelemetrySpan(ctx, "control_flow")
				span.SetResourceName("pipeline")
				span.SetTag("rshell.pipeline.stage_count", countPipelineStages(cm))
				defer func() {
					span.SetTag("rshell.pipeline.exit_code", int(r.exit.code))
					span.Finish(nil)
				}()
			}
			pr, pw, err := os.Pipe()
			if err != nil {
				r.exit.fatal(err) // not being able to create a pipe is rare but critical
				return
			}
			// Wrap stderr in a synchronized writer so both sides of the
			// pipe can write to it concurrently without a data race.
			safeStderr := &syncWriter{w: r.stderr}
			rLeft := r.subshell(true)
			rLeft.stdout = pw
			rLeft.stderr = safeStderr
			rLeft.inPipeline = true
			rLeft.inPipelineStage = true
			// Pipeline stages inherit the parent's loop context only when the
			// stage is a simple command or another pipeline. Bash silently
			// no-ops a bare `break`/`continue` invoked as an entire pipeline
			// stage, but when the stage is a compound command (`{...}`,
			// `(...)`, an if chain, etc.) bash prints the "only useful in
			// a loop" diagnostic AND keeps executing the rest of the stage.
			// Inheriting inLoop unconditionally would suppress the
			// diagnostic for compound stages and — worse — let the
			// stage-internal break/continue counter abort the rest of the
			// stage's statements, dropping commands that bash still runs.
			// A nested pipeline must inherit so the recursive case applies
			// the same per-stage rule to its own leaves; otherwise non-
			// rightmost stages of 3+ stage pipelines (`break | cat | cat`)
			// would lose loop context and emit spurious diagnostics.
			if pipelineStageInheritsInLoop(cm.X) {
				rLeft.inLoop = r.inLoop
			}
			rRight := r.subshell(true)
			rRight.stdin = pr
			rRight.stderr = safeStderr
			rRight.inPipeline = true
			rRight.inPipelineStage = true
			if pipelineStageInheritsInLoop(cm.Y) {
				rRight.inLoop = r.inLoop
			}
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						panicOut := io.Writer(io.Discard)
						if rLeft.stderr != nil {
							panicOut = rLeft.stderr
						}
						func() {
							defer func() { _ = recover() }()
							fmt.Fprintf(panicOut, "rshell: internal panic: %v\n", rec)
						}()
						rLeft.exit.fatal(errors.New("internal error"))
					}
					pw.Close()
					wg.Done()
				}()
				rLeft.stmt(ctx, cm.X)
				rLeft.exit.exiting = false
			}()
			rRight.stmt(ctx, cm.Y)
			r.exit = rRight.exit
			if !r.exit.limitExit {
				r.exit.exiting = false
			}
			pr.Close()
			wg.Wait()
			// Roll each pipeline stage's per-run counters up to the
			// parent so the run-span totals reflect commands dispatched,
			// blocked, or unknown across every stage.
			r.totalCount += rLeft.totalCount + rRight.totalCount
			r.dispatchedCount += rLeft.dispatchedCount + rRight.dispatchedCount
			r.unallowedCount += rLeft.unallowedCount + rRight.unallowedCount
			r.unknownCount += rLeft.unknownCount + rRight.unknownCount
			if rLeft.exit.fatalExit {
				r.exit.fatal(rLeft.exit.err)
			}
			if rLeft.exit.limitExit {
				r.exit.code = 1
				r.exit.exiting = true
				r.exit.limitExit = true
			}
		}
	case *syntax.IfClause:
		r.execIfChain(ctx, cm)
	case *syntax.ForClause:
		span, forCtx := startTelemetrySpan(ctx, "control_flow")
		span.SetResourceName("for")
		iterationCount := 0
		brokeEarly := false
		var varName string
		defer func() {
			span.SetTag("rshell.for.variable_name", varName)
			span.SetTag("rshell.for.iteration_count", iterationCount)
			span.SetTag("rshell.for.broke_early", brokeEarly)
			span.Finish(nil)
		}()
		switch y := cm.Loop.(type) {
		case *syntax.WordIter:
			varName = y.Name.Value
			items := r.Params // for i; do ...

			inToken := y.InPos.IsValid()
			if inToken {
				items = r.fields(y.Items...) // for i in ...; do ...
			}

			for _, field := range items {
				if err := forCtx.Err(); err != nil {
					r.exit.fatal(err)
					break
				}
				r.setVarString(varName, field)
				iterSpan, iterCtx := startTelemetrySpan(forCtx, "control_flow")
				iterSpan.SetResourceName("for.iteration")
				iterSpan.SetTag("rshell.for.iteration.index", iterationCount)
				broken := r.loopStmtsBroken(iterCtx, cm.Do)
				iterSpan.Finish(nil)
				iterationCount++
				if broken {
					// Excess continue at outermost loop: clamp and keep iterating
					// (bash treats "continue 99" in a single loop like "continue 1").
					if r.contnEnclosing > 0 && !r.inLoop {
						r.contnEnclosing = 0
						continue
					}
					brokeEarly = true
					break
				}
			}
			// Clamp excess break/continue levels at the outermost loop.
			// Bash discards excess levels (e.g. "break 99" with 1 loop).
			if !r.inLoop {
				r.breakEnclosing = 0
				r.contnEnclosing = 0
			}
		default:
			r.exit.fatal(fmt.Errorf("unsupported loop type: %T", cm.Loop))
		}
	case *syntax.WhileClause:
		r.execWhileClause(ctx, cm)
	default:
		r.exit.fatal(fmt.Errorf("unsupported command node: %T", cm))
	}
}

// pipelineStageInheritsInLoop reports whether a pipeline stage should inherit
// the parent's loop context. Bash silently no-ops a bare `break`/`continue`
// invoked as the entirety of a pipeline stage, but prints the "only useful in
// a loop" diagnostic when the stage is compound (block, subshell, if chain,
// loop) — so loop context is propagated for simple commands only.
//
// A nested pipeline (e.g. the `a | b` inside `a | b | c`) must also inherit
// the loop context: the recursive Pipe case re-runs this check on each leaf
// stage, so without propagation through intermediate pipeline nodes any non-
// rightmost stage of a 3+ stage pipeline would lose its loop context and
// `break`/`continue` would emit spurious diagnostics.
func pipelineStageInheritsInLoop(st *syntax.Stmt) bool {
	if st == nil {
		return false
	}
	switch c := st.Cmd.(type) {
	case *syntax.CallExpr:
		return true
	case *syntax.BinaryCmd:
		return c.Op == syntax.Pipe
	}
	return false
}

// execWhileClause runs a while or until loop. Both share the same AST node;
// cm.Until inverts the condition's exit-status check (`while` runs the body
// while the condition's last command exits 0; `until` runs the body while it
// exits non-zero).
//
// Per POSIX 1003.1-2024 §2.9.4.1/§2.9.4.2, the loop's exit status is the exit
// status of the last command of the last body iteration, or 0 if the body
// never executed.
//
// Termination is bounded by the runner's existing safety machinery: every
// r.stmt() invocation calls r.stop(ctx), which short-circuits on shell exit
// or context cancellation. We also explicitly check loopCtx.Err() at the top
// of each iteration so a cancelled context exits the loop with a fatal error
// even if the body and cond happen to be fast no-ops.
//
// Per-iteration spans are deliberately omitted (unlike for `for`): while/until
// can iterate unboundedly and emitting one span per iteration would be a
// memory cliff on long-running loops. Iteration count and broke-early are
// reported on the outer span instead.
func (r *Runner) execWhileClause(ctx context.Context, cm *syntax.WhileClause) {
	kind := "while"
	if cm.Until {
		kind = "until"
	}
	// Resource name encodes the loop kind (while/until); no separate kind tag
	// is needed.
	span, loopCtx := startTelemetrySpan(ctx, "control_flow")
	span.SetResourceName(kind)
	iterationCount := 0
	brokeEarly := false
	defer func() {
		span.SetTag("rshell.while.iteration_count", iterationCount)
		span.SetTag("rshell.while.broke_early", brokeEarly)
		span.Finish(nil)
	}()

	// The condition list is part of the loop's lexical scope, so `break` and
	// `continue` invoked inside it are valid (they should not error out as
	// "only useful in a loop"). Mark the runner as in-loop for the duration
	// of the entire while/until evaluation; loopStmtsBroken redundantly re-
	// sets this flag for the body, which is harmless.
	oldInLoop := r.inLoop
	r.inLoop = true
	defer func() { r.inLoop = oldInLoop }()

	var lastBody exitStatus
	// condBroke is set when the loop terminates because of a `break` invoked
	// inside the condition list, OR because `continue` invoked in the cond
	// short-circuits the cond list with status 0 and the cond-status check
	// then says "exit loop" (e.g. `until ...; continue` — until exits when
	// cond is 0). In those cases the loop's exit status must be the
	// builtin's exit status (typically 0), NOT the previous body's
	// last-command status — this matches bash. Without the flag, we would
	// overwrite the cond's status with stale lastBody at loop exit.
	condBroke := false
	for {
		if err := loopCtx.Err(); err != nil {
			r.exit.fatal(err)
			break
		}
		// Evaluate the condition list. Per POSIX, only the trailing exit
		// status decides whether to enter the body.
		r.stmts(loopCtx, cm.Cond)
		if r.exit.exiting || r.exit.fatalExit {
			break
		}

		// `break` invoked inside the cond list short-circuits the rest of
		// the cond and exits the loop regardless of cond's last status.
		// Preserve break's exit status (typically 0) as the loop's exit.
		if r.breakEnclosing > 0 {
			r.breakEnclosing--
			brokeEarly = true
			condBroke = true
			break
		}

		// `continue` invoked inside the cond list. Order matters here vs.
		// the cond-status check: when `continue` is invoked in an `until`
		// cond, continue's status (0) means until's cond-status check says
		// "exit loop", and bash then exits the loop rather than re-
		// evaluating. We mirror that by deferring to the cond-status check
		// after consuming nesting levels.
		if r.contnEnclosing > 0 {
			r.contnEnclosing--
			if r.contnEnclosing > 0 {
				// continue targets a loop further out.
				if !oldInLoop {
					// outermost: clamp excess (treat as continue 1).
					r.contnEnclosing = 0
				} else {
					// nested: propagate to outer regardless of cond
					// status. The outer loop will see contnEnclosing>0
					// and continue its own iteration.
					brokeEarly = true
					break
				}
			}
			// continue 1 (or clamped excess at outermost). Cond status
			// decides: if cond says exit loop (e.g. until + status 0),
			// exit and preserve continue's status. Otherwise re-evaluate
			// cond, skipping the body for this iteration.
			if r.exit.ok() == cm.Until {
				condBroke = true
				break
			}
			continue
		}

		// while: run body when cond.ok(); until: run body when !cond.ok().
		// Equivalently, exit the loop when ok() == cm.Until.
		if r.exit.ok() == cm.Until {
			break
		}
		// Reset the cond's exit so the body sees a clean status (mirrors how
		// r.stmt() resets r.exit before each statement).
		r.exit = exitStatus{}

		broken := r.loopStmtsBroken(loopCtx, cm.Do)
		iterationCount++
		// Capture the body's last-command exit; per POSIX this becomes the
		// loop's exit status if no further iteration runs.
		lastBody = r.exit

		if broken {
			// Excess continue at the outermost loop: clamp and keep iterating
			// (bash treats "continue 99" in a single loop like "continue 1").
			// oldInLoop is the in-loop flag from BEFORE this loop started, so
			// it tells us whether we're nested inside another loop.
			if r.contnEnclosing > 0 && !oldInLoop {
				r.contnEnclosing = 0
				continue
			}
			brokeEarly = true
			break
		}
	}
	// Clamp excess break/continue levels at the outermost loop, matching
	// bash's behaviour of discarding excess levels (e.g. "break 99" in a
	// single loop).
	if !oldInLoop {
		r.breakEnclosing = 0
		r.contnEnclosing = 0
	}
	// Loop's exit status (POSIX 2.9.4): exit status of the last command of the
	// last body iteration, or 0 if the body never ran. If the loop is exiting
	// via `exit` or a fatal error, we leave r.exit alone so the exit
	// status/state propagates upward. If the loop exited via a break in the
	// condition list, r.exit currently holds the break builtin's exit status
	// — leave it alone so the cond-break status (matching bash) is preserved
	// rather than overwritten by the stale previous-body status.
	if r.exit.exiting || r.exit.fatalExit {
		return
	}
	if condBroke {
		return
	}
	if iterationCount > 0 {
		r.exit = lastBody
	} else {
		r.exit = exitStatus{}
	}
}

func (r *Runner) stmts(ctx context.Context, stmts []*syntax.Stmt) {
	for _, stmt := range stmts {
		r.stmt(ctx, stmt)
		if r.exit.exiting || r.breakEnclosing > 0 || r.contnEnclosing > 0 {
			return
		}
	}
}

func (r *Runner) loopStmtsBroken(ctx context.Context, stmts []*syntax.Stmt) bool {
	oldInLoop := r.inLoop
	r.inLoop = true
	defer func() { r.inLoop = oldInLoop }()
	for _, stmt := range stmts {
		r.stmt(ctx, stmt)
		if r.contnEnclosing > 0 {
			r.contnEnclosing--
			return r.contnEnclosing > 0
		}
		if r.breakEnclosing > 0 {
			r.breakEnclosing--
			return true
		}
	}
	return false
}

// commandFlags extracts the flag tokens (arguments beginning with "-") from
// a command's arguments, for use as span telemetry. Only the flag name is
// kept, never a value:
//   - A value passed as a separate argument ("-r secret", "--file secret")
//     is never captured, since it doesn't itself start with "-".
//   - A value glued to a long flag with "=" ("--file=secret") is truncated
//     at "=".
//   - A value glued directly to a short flag with no separator at all
//     ("-nsecret", "-n5") has no delimiter to strip, so the token is
//     truncated to just the flag letter ("-n"). This also means a combined
//     boolean cluster like "-la" is recorded as only its first flag ("-l"),
//     since it's indistinguishable from a short flag plus a glued value at
//     this generic, per-builtin-schema-unaware layer — completeness of the
//     flag list is sacrificed to guarantee no value ever leaks.
//
// Scanning stops at a literal "--" end-of-flags separator, so nothing after
// it (even if flag-shaped) is captured. The lone "-" token (conventionally
// stdin/stdout, not a flag) is skipped.
func commandFlags(args []string) []string {
	var flags []string
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			continue
		}
		if eq := strings.IndexByte(arg, '='); eq >= 0 {
			arg = arg[:eq]
		}
		if !strings.HasPrefix(arg, "--") && len(arg) > 2 {
			arg = arg[:2]
		}
		flags = append(flags, arg)
	}
	return flags
}

// remediationOnlyRefusal reports whether dispatching the named builtin must be
// refused because it is registered as RemediationOnly while the shell runs in
// read-only mode, and returns the stderr text to write when it must.
//
// Builtins carry their own equivalent check; this dispatch-level gate is
// defence in depth that applies uniformly to every registered command,
// including ones added later that forget to gate themselves. The message comes
// from the command's metadata so the refusal text stays identical to the one
// the builtin would have printed.
func remediationOnlyRefusal(name string, remediationMode bool) (string, bool) {
	if remediationMode {
		return "", false
	}
	meta, ok := builtins.Meta(name)
	if !ok || !meta.RemediationOnly {
		return "", false
	}
	msg := meta.RemediationDeniedMessage
	if msg == "" {
		msg = builtins.DefaultRemediationDeniedMessage(name)
	}
	return msg, true
}

func (r *Runner) call(ctx context.Context, pos syntax.Pos, args []string, setup func() ([]string, bool)) {
	elevated := false
	if args[0] == "sudo" {
		if len(args) < 2 {
			r.errf("rshell: sudo: command is required\n")
			r.exit.code = 2
			return
		}
		elevated = true
		args = args[1:]
	}
	name := args[0]
	r.totalCount++

	// Evaluate both policy checks upfront so the span tags reflect the
	// independent facts about the command name regardless of which gate
	// short-circuits dispatch.
	isAllowed := r.allowAllCommands || r.allowedCommands[name]
	fn, isKnown := builtins.Lookup(name)

	span, ctx := startTelemetrySpan(ctx, "command")
	span.SetResourceName(name)
	span.SetTag("rshell.command.name", name)
	span.SetTag("rshell.command.is_allowed", isAllowed)
	span.SetTag("rshell.command.is_known", isKnown)
	setArgAttrs := func(args []string) {
		span.SetTag("rshell.command.argc", len(args)-1)
		if flags := commandFlags(args[1:]); len(flags) > 0 {
			// Padded with a leading and trailing comma so a query for one exact
			// flag (e.g. `*,-n,*`) can't false-positive match a longer flag that
			// merely contains the same substring (e.g. "-name").
			span.SetTag("rshell.command.flags", ","+strings.Join(flags, ",")+",")
		}
	}
	// has_stdin_pipe / has_output_redirect reflect whether the command's
	// stdin/stdout were reassigned from the Runner's originals — true for
	// both pipeline stages and file redirects.
	setIOAttrs := func() {
		span.SetTag("rshell.command.has_stdin_pipe", r.stdin != r.runStdin)
		span.SetTag("rshell.command.has_output_redirect", r.stdout != r.runStdout)
	}
	setIOAttrs()
	defer func() {
		setArgAttrs(args)
		span.SetTag("rshell.command.exit_code", int(r.exit.code))
		span.Finish(nil)
	}()

	if r.stop(ctx) {
		return
	}

	// Increment independently — is_allowed and is_known are orthogonal
	// facts about the command name, so a command that is both blocked and
	// missing from the registry bumps both counters. Mirrors the semantics
	// of the per-command rshell.command.is_allowed / is_known tags.
	if !isAllowed {
		r.unallowedCount++
	}
	if !isKnown {
		r.unknownCount++
	}

	if !isAllowed {
		r.errf("rshell: %s: command not allowed\n", name)
		if r.allowedCommands["help"] {
			r.errf("Run 'help' to see allowed commands.\n")
		}
		r.exit.code = 127
		return
	}
	if elevated && (r.elevate == nil || !r.elevatableCommands[name]) {
		r.errf("rshell: sudo: %s: elevation not allowed\n", name)
		r.exit.code = 126
		return
	}
	// Pipeline stages run concurrently and elevation changes the effective
	// UID of the whole process, so an elevated stage would also elevate its
	// siblings. inPipelineStage survives (…) subshells, unlike inPipeline.
	if elevated && r.inPipelineStage {
		r.errf("rshell: sudo: elevated commands are not allowed in pipelines\n")
		r.exit.code = 126
		return
	}

	if isKnown {
		// Enforce builtins.Command.RemediationOnly at dispatch, before the
		// handler runs and therefore before flag parsing, so --help is
		// refused exactly like any other invocation. Builtins repeat this
		// check themselves; this gate is the layer that makes the flag
		// load-bearing for a future builtin that forgets to.
		if msg, denied := remediationOnlyRefusal(name, r.remediationMode); denied {
			r.errf("%s", msg)
			r.exit.code = 1
			return
		}
	}

	if setup != nil {
		prepared, ok := setup()
		if !ok {
			setIOAttrs()
			return
		}
		args = prepared
		if elevated {
			args = args[1:]
		}
	}
	setIOAttrs()

	if isKnown {
		r.dispatchedCount++
		envEach := func(fn func(name, value string) bool) {
			r.writeEnv.Each(func(name string, vr expand.Variable) bool {
				if !vr.IsSet() || !vr.Exported {
					return true
				}
				return fn(name, vr.Str)
			})
		}
		var runCmdWithStdin func(context.Context, string, string, []string, io.Reader) (uint8, error)
		runCmdWithStdin = func(ctx context.Context, dir string, cmdName string, cmdArgs []string, childStdin io.Reader) (uint8, error) {
			if !r.allowAllCommands && !r.allowedCommands[cmdName] {
				return 127, fmt.Errorf("rshell: %s: command not allowed", cmdName)
			}
			cmdFn, ok := builtins.Lookup(cmdName)
			if !ok {
				return 127, fmt.Errorf("rshell: %s: unknown command", cmdName)
			}
			// Same remediation gate as the top-level dispatch above, applied
			// to grandchildren spawned by find -exec / -execdir / xargs.
			// Write to the same stderr the builtin's own check would have
			// used and return its exit code, so the refusal is byte-for-byte
			// what it was before the gate existed.
			if msg, denied := remediationOnlyRefusal(cmdName, r.remediationMode); denied {
				r.errf("%s", msg)
				return 1, nil
			}
			child := &builtins.CallContext{
				Stdout:  r.stdout,
				Stderr:  r.stderr,
				WorkDir: func() string { return dir },
				Env:     envEach,
				HostPrefix: func() string {
					// Return the sandbox's normalized prefix (filepath.Clean'd
					// in SetHostPrefix) rather than the raw user-supplied
					// value. A caller-provided trailing slash or "."/".."
					// segment would otherwise break prefix-matching in
					// builtins that consume this value.
					if r.sandbox != nil {
						return r.sandbox.HostPrefix()
					}
					return r.hostPrefix
				},
				CanonicalizeRootPrefix: func(absPath string) string {
					if r.sandbox == nil {
						return absPath
					}
					return r.sandbox.CanonicalizeRootPrefix(absPath)
				},
				OpenFile: func(ctx context.Context, path string, flags int, mode os.FileMode) (io.ReadWriteCloser, error) {
					f, err := r.sandbox.Open(path, dir, flags, mode)
					if err != nil {
						return nil, err
					}
					return allowedpaths.WithContextClose(ctx, f), nil
				},
				OpenRegularFile: func(ctx context.Context, path string) (io.ReadCloser, error) {
					f, err := r.sandbox.OpenRegular(path, dir)
					if err != nil {
						return nil, err
					}
					return allowedpaths.WithContextClose(ctx, f), nil
				},
				ReadDir: func(_ context.Context, path string) ([]fs.DirEntry, error) {
					return r.sandbox.ReadDir(path, dir)
				},
				OpenDir: func(_ context.Context, path string) (fs.ReadDirFile, error) {
					return r.sandbox.OpenDir(path, dir)
				},
				IsDirEmpty: func(_ context.Context, path string) (bool, error) {
					return r.sandbox.IsDirEmpty(path, dir)
				},
				ReadDirLimited: func(_ context.Context, path string, offset, maxRead int) ([]fs.DirEntry, bool, error) {
					return r.sandbox.ReadDirLimited(path, dir, offset, maxRead)
				},
				StatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
					return r.sandbox.Stat(path, dir)
				},
				FileSystemStat: func(_ context.Context, path string) (builtins.FileSystemInfo, error) {
					info, err := r.sandbox.StatFS(path, dir)
					return builtins.FileSystemInfo(info), err
				},
				LstatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
					return r.sandbox.Lstat(path, dir)
				},
				ReadlinkFile: func(_ context.Context, path string) (string, error) {
					return r.sandbox.Readlink(path, dir)
				},
				AccessFile: func(_ context.Context, path string, mode uint32) error {
					return r.sandbox.Access(path, dir, mode)
				},
				PortableErr: allowedpaths.PortableErrMsg,
				Now:         r.startTime,
				FileIdentity: func(path string, info fs.FileInfo) (builtins.FileID, bool) {
					absPath := path
					if !filepath.IsAbs(absPath) {
						absPath = filepath.Join(dir, absPath)
					}
					dev, ino, ok := allowedpaths.FileIdentity(absPath, info, r.sandbox)
					if !ok {
						return builtins.FileID{}, false
					}
					return builtins.FileID{Dev: dev, Ino: ino}, true
				},
				CommandAllowed: func(n string) bool {
					return r.allowAllCommands || r.allowedCommands[n]
				},
				ElevatableCommandsList:    r.elevatableCommandsList,
				AuthorizeSystemd:          r.authorizeSystemd,
				AuthorizeSystemServices:   r.authorizeSystemServices,
				ReadableSystemServices:    r.readableSystemServices,
				AllowedSystemServicesList: r.allowedSystemServicesList,
				AllowedPathsList: func() []builtins.AllowedPath {
					return allowedPathsList(r.sandbox)
				},
				// ChangeDir is intentionally nil for RunCommand children
				// (find -exec, find -execdir, xargs). bash forks a child
				// process for each invocation, so cd inside such a child
				// can never propagate to the parent shell. We model the
				// same isolation by making cd unavailable in this path —
				// the cd handler returns "cd: not supported in this
				// runner" rather than silently mutating the top-level
				// r.Dir, which would have leaked the child's directory
				// change back into the caller (the bug Codex flagged).
				LookupEnvVar: r.lookupEnvVar,
				RunCommand: func(ctx context.Context, dir string, name string, args []string) (uint8, error) {
					// Inherit the parent's overridden stdin so grandchildren
					// dispatched via RunCommand (the no-stdin variant) stay
					// isolated from the top-level r.stdin. When the parent
					// has no override (childStdin == nil), this is the same
					// fallback as before — the grandchild's CallContext
					// picks up r.stdin via the default branch below.
					return runCmdWithStdin(ctx, dir, name, args, childStdin)
				},
				RunCommandWithStdin: runCmdWithStdin,
				// Intentionally not exposing SetVar / GetVar in the
				// child CallContext used for find -exec / -execdir
				// grandchildren. find treats each invocation as a
				// separate command (bash forks and execs a new
				// process), so any environment mutation must not
				// leak back to the calling shell. State-mutating
				// builtins like read detect the absent SetVar and
				// refuse to run with "variable access is not
				// available", which is the closest analogue to bash's
				// "exec read fails because read is a builtin, not an
				// executable on PATH" behaviour.
				Proc:            r.proc,
				Systemd:         r.systemd,
				RemediationMode: r.remediationMode,
			}
			if r.remediationMode && r.sandbox != nil {
				child.Truncate = func(_ context.Context, path string, size int64, create bool) error {
					return r.sandbox.Truncate(path, dir, size, create)
				}
				child.TruncateToZeroIfAtLeast = func(_ context.Context, path string, minSize int64, dryRun bool) (int64, bool, error) {
					return r.sandbox.TruncateToZeroIfAtLeast(path, dir, minSize, dryRun)
				}
				child.Remove = func(_ context.Context, path string) error {
					return r.removeWithBudget(dir, path)
				}
				child.WriteRegularFile = func(ctx context.Context, path string, data []byte, expectedIdentity fs.FileInfo) (bool, error) {
					return r.writeRegularFile(ctx, path, dir, data, expectedIdentity)
				}
			}
			if childStdin != nil {
				child.Stdin = childStdin
			} else if r.stdin != nil {
				child.Stdin = r.stdin
			}
			result := cmdFn(ctx, child, cmdArgs)
			return result.Code, nil
		}
		runCmd := func(ctx context.Context, dir string, cmdName string, cmdArgs []string) (uint8, error) {
			return runCmdWithStdin(ctx, dir, cmdName, cmdArgs, nil)
		}
		call := &builtins.CallContext{
			Stdout:       r.stdout,
			Stderr:       r.stderr,
			InLoop:       r.inLoop,
			LastExitCode: r.lastExit.code,
			Env:          envEach,
			WorkDir: func() string {
				return HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir
			},
			HostPrefix: func() string {
				// Return the sandbox's normalized prefix (filepath.Clean'd
				// in SetHostPrefix) rather than the raw user-supplied
				// value. A caller-provided trailing slash or "."/".."
				// segment would otherwise break prefix-matching in
				// builtins that consume this value.
				if r.sandbox != nil {
					return r.sandbox.HostPrefix()
				}
				return r.hostPrefix
			},
			CanonicalizeRootPrefix: func(absPath string) string {
				if r.sandbox == nil {
					return absPath
				}
				return r.sandbox.CanonicalizeRootPrefix(absPath)
			},
			OpenFile: func(ctx context.Context, path string, flags int, mode os.FileMode) (io.ReadWriteCloser, error) {
				f, err := r.open(ctx, path, flags, mode, false)
				if err != nil {
					return nil, err
				}
				return allowedpaths.WithContextClose(ctx, f), nil
			},
			OpenRegularFile: func(ctx context.Context, path string) (io.ReadCloser, error) {
				f, err := r.sandbox.OpenRegular(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
				if err != nil {
					return nil, err
				}
				return allowedpaths.WithContextClose(ctx, f), nil
			},
			ReadDir: func(ctx context.Context, path string) ([]fs.DirEntry, error) {
				return r.sandbox.ReadDir(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			OpenDir: func(ctx context.Context, path string) (fs.ReadDirFile, error) {
				return r.sandbox.OpenDir(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			IsDirEmpty: func(ctx context.Context, path string) (bool, error) {
				return r.sandbox.IsDirEmpty(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			ReadDirLimited: func(ctx context.Context, path string, offset, maxRead int) ([]fs.DirEntry, bool, error) {
				return r.sandbox.ReadDirLimited(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir, offset, maxRead)
			},
			StatFile: func(ctx context.Context, path string) (fs.FileInfo, error) {
				return r.sandbox.Stat(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			FileSystemStat: func(ctx context.Context, path string) (builtins.FileSystemInfo, error) {
				info, err := r.sandbox.StatFS(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
				return builtins.FileSystemInfo(info), err
			},
			LstatFile: func(ctx context.Context, path string) (fs.FileInfo, error) {
				return r.sandbox.Lstat(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			ReadlinkFile: func(ctx context.Context, path string) (string, error) {
				return r.sandbox.Readlink(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir)
			},
			AccessFile: func(ctx context.Context, path string, mode uint32) error {
				return r.sandbox.Access(path, HandlerCtx(r.handlerCtx(ctx, todoPos)).Dir, mode)
			},
			PortableErr: allowedpaths.PortableErrMsg,
			Now:         r.startTime,
			FileIdentity: func(path string, info fs.FileInfo) (builtins.FileID, bool) {
				absPath := path
				if !filepath.IsAbs(absPath) {
					absPath = filepath.Join(r.Dir, absPath)
				}
				dev, ino, ok := allowedpaths.FileIdentity(absPath, info, r.sandbox)
				if !ok {
					return builtins.FileID{}, false
				}
				return builtins.FileID{Dev: dev, Ino: ino}, true
			},
			CommandAllowed: func(cmdName string) bool {
				return r.allowAllCommands || r.allowedCommands[cmdName]
			},
			ElevatableCommandsList:    r.elevatableCommandsList,
			AuthorizeSystemd:          r.authorizeSystemd,
			AuthorizeSystemServices:   r.authorizeSystemServices,
			ReadableSystemServices:    r.readableSystemServices,
			AllowedSystemServicesList: r.allowedSystemServicesList,
			AllowedPathsList: func() []builtins.AllowedPath {
				return allowedPathsList(r.sandbox)
			},
			ChangeDir:           r.changeDir,
			LookupEnvVar:        r.lookupEnvVar,
			RunCommand:          runCmd,
			RunCommandWithStdin: runCmdWithStdin,
			SetVar: func(name, value string) error {
				if len(value) > MaxVarBytes {
					return fmt.Errorf("%s: value too large (limit %d bytes)", name, MaxVarBytes)
				}
				err := r.setVarErr(name, expand.Variable{Set: true, Kind: expand.String, Str: value})
				if err == nil {
					return nil
				}
				// Total-storage exhaustion is script-aborting (matches the
				// AST setVar behaviour in interp/vars.go). Translate the
				// internal sentinel to the public builtins.ErrVarStorageExceeded
				// so state-mutating builtins can surface Result.Exiting=true
				// without needing access to the private interp type.
				var storageErr *errTotalVarStorageExceeded
				if errors.As(err, &storageErr) {
					return fmt.Errorf("%s: %w", err, builtins.ErrVarStorageExceeded)
				}
				return err
			},
			GetVar: func(name string) (string, bool) {
				vr := r.writeEnv.Get(name)
				return vr.Str, vr.IsSet()
			},
			Proc:            r.proc,
			Systemd:         r.systemd,
			RemediationMode: r.remediationMode,
		}
		if r.remediationMode && r.sandbox != nil {
			call.Truncate = func(_ context.Context, path string, size int64, create bool) error {
				return r.sandbox.Truncate(path, r.Dir, size, create)
			}
			call.TruncateToZeroIfAtLeast = func(_ context.Context, path string, minSize int64, dryRun bool) (int64, bool, error) {
				return r.sandbox.TruncateToZeroIfAtLeast(path, r.Dir, minSize, dryRun)
			}
			call.Remove = func(_ context.Context, path string) error {
				return r.removeWithBudget(r.Dir, path)
			}
			call.WriteRegularFile = func(ctx context.Context, path string, data []byte, expectedIdentity fs.FileInfo) (bool, error) {
				return r.writeRegularFile(ctx, path, r.Dir, data, expectedIdentity)
			}
		}
		if r.stdin != nil { // do not assign a typed nil into the io.Reader interface
			call.Stdin = r.stdin
		}
		var result builtins.Result
		if elevated {
			if err := r.elevate(ctx, name, func() { result = fn(ctx, call, args[1:]) }); err != nil {
				r.exit.fatal(fmt.Errorf("elevating %s: %w", name, err))
				return
			}
		} else {
			result = fn(ctx, call, args[1:])
		}
		r.exit.code = result.Code
		r.exit.exiting = result.Exiting
		r.breakEnclosing = result.BreakN
		r.contnEnclosing = result.ContinueN
		// If the run-level context was cancelled while the builtin was
		// blocked (MaxExecutionTime, CLI --timeout, parent cancellation),
		// surface that error to Run() so callers can distinguish a
		// timeout-driven termination from an ordinary failing exit code.
		// Without this, a long-blocking builtin that happens to be the
		// last command in a script would leave Run() returning the
		// builtin's status (e.g. `read -t`'s 142) instead of the
		// underlying context.DeadlineExceeded that cmd/rshell main checks
		// for to print "execution timed out".
		if err := ctx.Err(); err != nil {
			r.exit.fatal(err)
		}
		return
	}
	// Allowed but not known: the default execHandler (noExecHandler) will
	// reject with exit 127. unknownCount was already incremented above.
	r.exec(ctx, pos, args)
}

func (r *Runner) exec(ctx context.Context, pos syntax.Pos, args []string) {
	r.exit.fromHandlerError(r.execHandler(r.handlerCtx(ctx, pos), args))
}

// execIfChain runs an if/elif/else chain iteratively (rather than recursing
// through cmd() on each elif as the AST's Else pointer suggests) so the whole
// chain is covered by a single rshell.if span. The parser encodes "else" as a
// trailing *IfClause with no ThenPos set and an empty Cond.
func (r *Runner) execIfChain(ctx context.Context, cm *syntax.IfClause) {
	span, ctx := startTelemetrySpan(ctx, "control_flow")
	span.SetResourceName("if")
	branchCount := 0
	for cur := cm; cur != nil; cur = cur.Else {
		branchCount++
	}
	branchTaken := -2 // -2 = none matched, -1 = else, >=0 = if/elif index
	defer func() {
		span.SetTag("rshell.if.branch_taken", branchTaken)
		span.SetTag("rshell.if.branch_count", branchCount)
		span.Finish(nil)
	}()

	idx := 0
	for cur := cm; cur != nil; cur = cur.Else {
		// A trailing "else" branch has no "then" token.
		if !cur.ThenPos.IsValid() {
			branchTaken = -1
			r.stmts(ctx, cur.Then)
			return
		}
		r.stmts(ctx, cur.Cond)
		if r.exit.exiting || r.breakEnclosing > 0 || r.contnEnclosing > 0 {
			return
		}
		if r.exit.ok() {
			branchTaken = idx
			r.stmts(ctx, cur.Then)
			return
		}
		r.exit = exitStatus{}
		idx++
	}
}

// countPipelineStages walks a left-recursive Pipe-BinaryCmd tree (e.g. a|b|c
// is BinaryCmd(Pipe, BinaryCmd(Pipe, a, b), c)) and returns the total number
// of stages.
func countPipelineStages(cm *syntax.BinaryCmd) int {
	n := 1 // Y is one stage
	if inner, ok := cm.X.Cmd.(*syntax.BinaryCmd); ok && inner.Op == syntax.Pipe {
		n += countPipelineStages(inner)
	} else {
		n++
	}
	return n
}

// syncWriter wraps an io.Writer with a mutex so concurrent writes are safe.
// Used to protect stderr when both sides of a pipe write to it.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}
