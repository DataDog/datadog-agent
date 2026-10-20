// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interp implements a restricted shell interpreter designed for
// safe, sandboxed execution. It supports a subset of Bash syntax with
// many features intentionally blocked (see [validateNode]).
//
// The interpreter behaves like a non-interactive shell. External command
// execution and filesystem access are denied by default and must be
// explicitly enabled via [RunnerOption] functions.
package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	internalsystemd "github.com/DataDog/datadog-agent/pkg/rshell/internal/systemd"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
)

// runnerConfig holds the immutable configuration of a [Runner].
// These fields are set during construction ([New]) and first [Runner.Reset],
// and never change afterwards.
type runnerConfig struct {
	// Env specifies the initial environment for the interpreter, which must
	// not be nil. It can only be set via [Env].
	Env expand.Environ

	// execHandler is responsible for executing programs. It must not be nil.
	execHandler ExecHandlerFunc

	// openHandler is a function responsible for opening files. It must not be nil.
	openHandler OpenHandlerFunc

	// readDirHandler is a function responsible for reading directories during
	// glob expansion. It must be non-nil.
	readDirHandler ReadDirHandlerFunc

	// sandbox restricts file/directory access to allowed directories.
	// nil (default) blocks all file access; populate via AllowedPaths option.
	sandbox *allowedpaths.Sandbox

	// sandboxWarnings holds diagnostic messages emitted while applying runner
	// options. Flushed to warningsWriter after all options are applied and
	// defaults are set, so the output target is independent of option ordering.
	// Retained after flush so callers can retrieve them via [Runner.Warnings].
	sandboxWarnings []byte

	// warningsWriter is the destination for runner warning messages.
	// Defaults to r.stderr when unset; callers can route warnings to a separate
	// sink (e.g. a dedicated buffer or logger) via [WarningsWriter] so they are
	// not conflated with command stderr.
	warningsWriter io.Writer

	// hostPrefix is stored here so HostPrefix can be applied before or
	// after AllowedPaths. Applied to the sandbox in New() after all
	// options are processed.
	hostPrefix string

	// allowedCommands is the set of command names (builtins or external) that
	// the interpreter is permitted to execute. If nil and allowAllCommands is
	// false, no commands are allowed.
	allowedCommands map[string]bool

	// allowAllCommands bypasses the allowedCommands check and permits any
	// command. Intended for testing convenience.
	allowAllCommands bool

	// allowedSystemServices maps exact systemd units to their permitted actions. It
	// is independent of allowAllCommands and
	// defaults to denying every systemd operation.
	allowedSystemServices systemdGrants

	// systemdTarget identifies the local or mounted host used by systemd-aware
	// builtins. It is resolved once during construction and shared by
	// subshells.
	systemdTarget           internalsystemd.Target
	systemdTargetConfigured bool
	systemd                 *builtins.SystemdServices

	// maxExecutionTime bounds the duration of each Run call. Zero disables
	// the limit. When non-zero, Run derives a child context with this timeout.
	maxExecutionTime time.Duration

	// procPath is the path to the proc filesystem used by proc-aware builtins
	// such as ps and pmap.
	// Defaults to "/proc" when empty.
	procPath string

	// commandText is the raw script text reported on the "run" telemetry
	// span. Set via [Script]; has no effect on parsing or execution.
	commandText string

	// disableDetailedTelemetry suppresses the rshell.run.command and
	// rshell.run.options.* tags on the "run" span. Set via
	// [DisableDetailedTelemetry].
	disableDetailedTelemetry bool

	// invokedViaCLI marks the "run" span as coming from the cmd/rshell CLI
	// rather than an embedding Go program. Set via [InvokedViaCLI].
	invokedViaCLI bool

	// remediationMode enables remediation-only capabilities, including file-target
	// output redirections within AllowedPaths and the restricted systemctl builtin.
	remediationMode bool

	// elevate runs one explicitly marked command inside a temporary privilege
	// window. nil (the default) means the sudo marker is unavailable.
	elevate            ElevateFunc
	elevatableCommands map[string]bool

	// proc is the ProcProvider constructed from procPath, created once in
	// New() and shared across subshells via runnerConfig value copy.
	proc *builtins.ProcProvider

	// usedNew is set by New() and checked in Reset() to ensure a Runner
	// was properly constructed rather than zero-initialized.
	usedNew bool

	// origDir, origParams, and origStd* preserve the initial values
	// set during construction so that [Runner.Reset] can restore them.
	origDir    string
	origParams []string
	origStdin  *os.File
	origStdout io.Writer
	origStderr io.Writer
}

// runnerState holds the per-execution mutable state of a [Runner].
// [Runner.Reset] reinitializes this entire struct from [runnerConfig].
type runnerState struct {
	// writeEnv overlays [runnerConfig.Env] so that we can write environment
	// variables as an overlay.
	writeEnv expand.WriteEnviron

	// Dir specifies the working directory of the command, which must be an
	// absolute path.
	Dir string

	// Params are the current shell parameters, e.g. from running a shell
	// file. Note: positional parameter expansion ($@, $*, $1, etc.) is
	// blocked by the AST validator in this restricted interpreter.
	Params []string

	stdin  *os.File // e.g. the read end of a pipe
	stdout io.Writer
	stderr io.Writer

	// runStdin / runStdout are the baselines captured at the start of Run()
	// after any Run-level stdout wrapping. Telemetry uses them to decide
	// whether a command's stdin/stdout was reassigned by a pipe or redirect.
	// Propagated to subshells via the runnerState copy in subshell().
	runStdin  *os.File
	runStdout io.Writer

	// inPipeline is set to true on subshells created for pipeline stages and
	// inherited by further subshells spawned inside them. It suppresses the
	// nested rshell.pipeline spans that would otherwise fire when the
	// pipeline implementation recurses through stmt() to handle N-stage
	// pipelines (a|b|c is BinaryCmd(Pipe, BinaryCmd(Pipe, a, b), c)). Reset
	// to false when entering a syntax.Subshell so a pipeline inside (…) gets
	// its own span.
	inPipeline bool

	// inPipelineStage is set on subshells created for pipeline stages and
	// inherited by every runner spawned beneath them. Unlike inPipeline it is
	// never reset when entering a syntax.Subshell: a (…) inside a stage still
	// runs concurrently with the sibling stages, so it is the flag the
	// elevation guard relies on. It must not be repurposed for telemetry.
	inPipelineStage bool

	// totalCount / dispatchedCount / unallowedCount / unknownCount tally
	// the call() invocations this run observed: how many command
	// dispatches were attempted in total, how many ran through a
	// builtin, how many were blocked by AllowedCommands, and how many
	// were not in the builtin registry. The unallowed/unknown pair are
	// independent facts about the command name — a command that is both
	// blocked and unknown bumps both counters, matching the semantics of
	// the per-command rshell.command.is_allowed / is_known tags.
	// totalCount counts each call() exactly once regardless of outcome,
	// so total_count = dispatched_count + (unallowed-only) + (unknown-only)
	// + (unallowed AND unknown). Summed up from subshells when a
	// pipeline or (…) completes.
	totalCount      int
	dispatchedCount int
	unallowedCount  int
	unknownCount    int

	ecfg *expand.Config
	ectx context.Context // just so that subshell can use it again

	// didReset remembers whether the runner has ever been reset. This is
	// used so that Reset is automatically called when running any program
	// or node for the first time on a Runner.
	didReset bool

	filename string // only if Node was a File

	// >0 to break or continue out of N enclosing loops
	breakEnclosing, contnEnclosing int

	inLoop bool

	// The current and last exit statuses. They can only be different if
	// the interpreter is in the middle of running a statement. In that
	// scenario, 'exit' is the status for the current statement being run,
	// and 'lastExit' corresponds to the previous statement that was run.
	exit     exitStatus
	lastExit exitStatus

	lastExpandExit exitStatus // used to surface exit statuses while expanding fields

	// startTime is captured once at the beginning of Run() and passed to
	// all builtin invocations so they share a consistent time reference.
	startTime time.Time

	// globReadDirCount tracks the total number of ReadDirForGlob calls
	// across the entire Run() invocation. It is shared with subshells
	// (including concurrent pipe subshells) via pointer, and must be
	// accessed atomically.
	globReadDirCount *atomic.Int64

	// fileRemovalCount tracks the total number of files successfully removed
	// through the builtin Remove capability across the entire Run()
	// invocation. Like globReadDirCount it is shared with subshells
	// (including concurrent pipe subshells) via pointer and must be accessed
	// atomically, so a `for` loop, a subshell, or an `xargs -n1 rm` pipeline
	// cannot each start a fresh budget.
	fileRemovalCount *atomic.Int64

	// expansionByteCount tracks expanded argument, assignment, redirect, and
	// heredoc bytes across the entire Run invocation. It is shared with
	// subshells and pipeline stages so nested execution cannot reset the budget.
	expansionByteCount *atomic.Int64
}

// A Runner interprets shell programs. It can be reused, but it is not safe for
// concurrent use. Use [New] to build a new Runner.
//
// Runner's exported fields are meant to be configured via [RunnerOption];
// once a Runner has been created, the fields should be treated as read-only.
type Runner struct {
	runnerConfig
	runnerState
}

// ElevateFunc temporarily raises privileges while run executes. Implementations
// must restore privileges before returning, including when run fails.
type ElevateFunc func(ctx context.Context, command string, run func()) error

// exitStatus holds the state of the shell after running one command.
// Beyond the exit status code, it also holds whether the shell should return or exit,
// as well as any Go error values that should be given back to the user.
type exitStatus struct {
	// code is the exit status code.
	code uint8

	exiting   bool // whether the current shell is exiting
	fatalExit bool // whether the current shell is exiting due to a fatal error; err below must not be nil
	limitExit bool // whether a resource limit must abort the entire script

	// err is a fatal error if fatal is true, or a non-fatal custom error from a handler.
	// Used so that running a single statement with a custom handler
	// which returns a non-fatal Go error, such as a Go error wrapping [NewExitStatus],
	// can be returned by [Runner.Run] without being lost entirely.
	err error
}

func (e *exitStatus) ok() bool { return e.code == 0 }

func (e *exitStatus) oneIf(b bool) {
	if b {
		e.code = 1
	} else {
		e.code = 0
	}
}

func (e *exitStatus) fatal(err error) {
	if !e.fatalExit && err != nil {
		e.exiting = true
		e.fatalExit = true
		e.err = err
		if e.code == 0 {
			e.code = 1
		}
	}
}

func (e *exitStatus) fromHandlerError(err error) {
	if err != nil {
		var es ExitStatus
		if errors.As(err, &es) {
			e.err = err
			e.code = uint8(es)
		} else {
			e.fatal(err) // handler's custom fatal error
		}
	} else {
		e.code = 0
	}
}

// New creates a new Runner, applying a number of options. If applying any of
// the options results in an error, it is returned.
//
// Any unset options fall back to their defaults. For example, not supplying the
// environment defaults to an empty environment (no host env inherited), and not
// supplying the standard output writer means that the output will be discarded.
func New(opts ...RunnerOption) (*Runner, error) {
	registerBuiltins()
	r := &Runner{
		runnerConfig: runnerConfig{usedNew: true},
	}
	for _, opt := range opts {
		if err := opt(r); err != nil {
			_ = r.Close()
			return nil, err
		}
	}

	// Default to an empty environment to avoid propagating parent env vars.
	if r.Env == nil {
		r.Env = expand.ListEnviron()
	}
	if r.Dir == "" {
		if paths := r.sandbox.Paths(); len(paths) > 0 {
			r.Dir = paths[0]
		} else {
			dir, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("could not get current dir: %w", err)
			}
			r.Dir = dir
		}
	}
	if r.stdout == nil || r.stderr == nil {
		StdIO(r.stdin, r.stdout, r.stderr)(r)
	}
	// Default warnings to r.stderr so today's behaviour is
	// preserved for callers who do not opt in to a dedicated sink.
	if r.warningsWriter == nil {
		r.warningsWriter = r.stderr
	}
	// Apply host prefix if set, now that both HostPrefix and AllowedPaths
	// have been processed regardless of option ordering.
	if r.hostPrefix != "" && r.sandbox != nil {
		r.sandbox.SetHostPrefix(r.hostPrefix)
	}
	// In remediation mode, unlock write opens on the sandbox. The sandbox
	// defaults to read-only (defense-in-depth); SetWritable opts it in only
	// when the caller has explicitly requested remediation mode.
	if r.remediationMode && r.sandbox != nil {
		r.sandbox.SetWritable()
	}
	// Flush buffered warnings now that the warnings sink is guaranteed to be
	// set. The buffer is retained so callers can retrieve it via [Runner.Warnings].
	if len(r.sandboxWarnings) > 0 {
		r.warningsWriter.Write(r.sandboxWarnings)
	}
	if !r.systemdTargetConfigured {
		r.systemdTarget = internalsystemd.LocalTarget()
	}
	systemdClient := internalsystemd.NewClient(r.systemdTarget)
	r.systemd = &builtins.SystemdServices{
		Journal:        systemdClient,
		JournalStorage: systemdClient,
		JournalCleaner: systemdClient,
		JournalRotator: systemdClient,
		ServiceState:   systemdClient,
		ServiceControl: systemdClient,
	}
	r.proc = builtins.NewProcProvider(r.procPath)
	return r, nil
}

// RunnerOption can be passed to [New] to alter a [Runner]'s behaviour.
type RunnerOption func(*Runner) error

func stdinFile(ctx context.Context, r io.Reader) (*os.File, error) {
	switch r := r.(type) {
	case *os.File:
		return r, nil
	case nil:
		return nil, nil
	default:
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		go func() {
			defer pw.Close()
			buf := make([]byte, 32*1024)
			for {
				if ctx.Err() != nil {
					return
				}
				// Note: r.Read may block past ctx cancellation if the underlying
				// reader does not respect deadlines. For the StdIO path
				// (context.Background()), the goroutine is bounded by the
				// reader reaching EOF or pw.Write failing once the runner
				// closes the pipe read-end. For the runner_redir.go path
				// (execution-scoped context), a slow reader may keep this
				// goroutine alive briefly after the script is cancelled; the
				// ctx.Err() check at the top of the loop bounds the delay to
				// at most one additional Read call.
				n, err := r.Read(buf)
				if n > 0 {
					if _, werr := pw.Write(buf[:n]); werr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
		return pr, nil
	}
}

// Env sets the initial environment for the interpreter. Each pair must be in
// "KEY=value" format. If this option is not used, the interpreter starts with
// an empty environment (no host environment variables are inherited).
func Env(pairs ...string) RunnerOption {
	return func(r *Runner) error {
		r.Env = expand.ListEnviron(pairs...)
		return nil
	}
}

// StdIO configures an interpreter's standard input, standard output, and
// standard error. If out or err are nil, they default to a writer that discards
// the output.
//
// Note that providing a non-nil standard input other than [*os.File] will require
// an [os.Pipe] and spawning a goroutine to copy into it,
// as an [os.File] is the only way to share a reader with subprocesses.
// This may cause the interpreter to consume the entire reader.
// See [os/exec.Cmd.Stdin].
//
// When providing an [*os.File] as standard input, consider using an [os.Pipe]
// as it has the best chance to support cancellable reads via [os.File.SetReadDeadline],
// so that cancelling the runner's context can stop a blocked standard input read.
//
// When a non-[*os.File] reader is provided, the background copy goroutine uses
// [context.Background] because [StdIO] is a [RunnerOption] executed before any
// [Runner.Run] call, so no run-scoped context is available at this point.
// The goroutine terminates as soon as the reader returns [io.EOF] or the pipe's
// write end is closed, so it is bounded by the reader's lifetime. Callers that
// require context-cancellable stdin should provide an [*os.File] (e.g. via
// [os.Pipe]) directly, or use a redirect inside the script.
// If the provided reader's Read blocks indefinitely (for example a [net.Conn]
// without a deadline), the goroutine may outlive the script; callers in that
// situation should wrap the reader with a deadline-aware adapter before passing
// it to StdIO.
func StdIO(in io.Reader, out, err io.Writer) RunnerOption {
	return func(r *Runner) error {
		stdin, _err := stdinFile(context.Background(), in)
		if _err != nil {
			return _err
		}
		r.stdin = stdin
		if out == nil {
			out = io.Discard
		}
		r.stdout = out
		if err == nil {
			err = io.Discard
		}
		r.stderr = err
		return nil
	}
}

// MaxExecutionTime bounds the total execution time of each [Runner.Run] call.
//
// When d is zero, no timeout is applied. Negative values are rejected.
//
// The timeout is applied per Run call rather than when the Runner is created,
// so reusing a Runner across multiple runs yields a fresh deadline each time.
func MaxExecutionTime(d time.Duration) RunnerOption {
	return func(r *Runner) error {
		if d < 0 {
			return fmt.Errorf("MaxExecutionTime: duration must be >= 0")
		}
		r.maxExecutionTime = d
		return nil
	}
}

// Reset returns a runner to its initial state, right before the first call to
// Run or Reset.
//
// Typically, this function only needs to be called if a runner is reused to run
// multiple programs non-incrementally. Not calling Reset between each run will
// mean that the shell state will be kept, including variables, options, and the
// current directory.
func (r *Runner) Reset() {
	if !r.usedNew {
		r.exit.fatal(fmt.Errorf("use interp.New to construct a Runner"))
		return
	}
	if !r.didReset {
		r.origDir = r.Dir
		r.origParams = r.Params
		r.origStdin = r.stdin
		r.origStdout = r.stdout
		r.origStderr = r.stderr

		// Install sandbox-backed handlers. AllowedPaths opens os.Root handles
		// eagerly during construction, so there is no filesystem race here.
		// Default: block all file access (nil sandbox).
		if r.openHandler == nil {
			r.openHandler = func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
				return r.sandbox.Open(path, HandlerCtx(ctx).Dir, flag, perm)
			}
			r.readDirHandler = func(ctx context.Context, path string) ([]os.DirEntry, error) {
				return r.sandbox.ReadDirForGlob(path, HandlerCtx(ctx).Dir)
			}
			r.execHandler = noExecHandler(r.allowAllCommands || r.allowedCommands["help"])
		}
		if r.execHandler == nil {
			r.execHandler = noExecHandler(r.allowAllCommands || r.allowedCommands["help"])
		}
	}
	// Reset only the mutable state; config is preserved.
	// startTime is intentionally zeroed here by the struct literal; it will
	// be set again by Run() before any builtin is invoked.
	r.runnerState = runnerState{
		Dir:    r.origDir,
		Params: r.origParams,
		stdin:  r.origStdin,
		stdout: r.origStdout,
		stderr: r.origStderr,
	}
	r.writeEnv = &overlayEnviron{parent: r.Env}
	r.setVarString("PWD", r.Dir)
	r.setVarString("RSHELL_VERSION", version.Version)
	// IFS is intentionally mutable: scripts may set it to customise field splitting,
	// which is standard POSIX behaviour. Callers that provide a custom ExecHandler
	// should be aware that a script can set IFS to a non-whitespace value (e.g.
	// IFS=/) to manipulate how unquoted variable expansions are split before being
	// passed to executed commands (argument smuggling). The default noExecHandler
	// blocks all external execution, limiting the practical impact of this vector.
	r.setVarString("IFS", " \t\n")
	r.setVarString("OPTIND", "1")
	if r.sandbox != nil {
		r.setVarString("ALLOWED_PATHS", strings.Join(r.sandbox.Paths(), string(filepath.ListSeparator)))
	}

	// Reset the total-bytes counter so that the interpreter's own initial
	// variable assignments (PWD, IFS, OPTIND, ALLOWED_PATHS above) do not
	// count against the user-visible MaxTotalVarsBytes cap. Those values are
	// small and bounded; only the variables that a script itself creates or
	// modifies should count. ALLOWED_PATHS is operator-configured and
	// typically a few hundred bytes, so this is safe.
	if ov, ok := r.writeEnv.(*overlayEnviron); ok {
		ov.totalBytes = 0
	}

	r.didReset = true
}

// ErrOutputLimitExceeded is returned by Run when a script produces more stdout
// than maxStdoutBytes. Partial output up to the limit is still delivered to the
// caller's writer. Use errors.Is to check for this condition.
var ErrOutputLimitExceeded = errors.New(fmt.Sprintf(
	"stdout limit exceeded: script produced more than %d MiB of output",
	maxStdoutBytes/(1024*1024),
))

// ErrStderrLimitExceeded is returned by Run when a script produces more stderr
// than maxStderrBytes. Partial output up to the limit is still delivered to the
// caller's writer. When both stdout and stderr caps fire in the same run, Run
// returns a bothLimitsExceededError whose Is method matches either sentinel,
// so callers can write errors.Is(err, ErrOutputLimitExceeded) or
// errors.Is(err, ErrStderrLimitExceeded) without caring which fired.
var ErrStderrLimitExceeded = errors.New(fmt.Sprintf(
	"stderr limit exceeded: script produced more than %d MiB of output",
	maxStderrBytes/(1024*1024),
))

// bothLimitsExceededError is returned by Run when a single invocation exceeds
// both the stdout and stderr caps. Its Is method satisfies errors.Is for
// ErrOutputLimitExceeded and ErrStderrLimitExceeded so callers can detect
// either condition without distinguishing the both-streams case.
type bothLimitsExceededError struct{}

func (bothLimitsExceededError) Error() string {
	return fmt.Sprintf("stdout and stderr limit exceeded: script produced more than %d MiB of output on each stream",
		maxStdoutBytes/(1024*1024))
}

func (bothLimitsExceededError) Is(target error) bool {
	return target == ErrOutputLimitExceeded || target == ErrStderrLimitExceeded
}

// ExitStatus is a non-zero status code resulting from running a shell node.
type ExitStatus uint8

func (s ExitStatus) Error() string { return fmt.Sprintf("exit status %d", s) }

// Run interprets a node, which can be a [*File], [*Stmt], or [Command]. If a non-nil
// error is returned, it will typically contain a command's exit status, which
// can be retrieved with [errors.As] and [ExitStatus].
//
// Run can be called multiple times synchronously to interpret programs
// incrementally. To reuse a [Runner] without keeping the internal shell state,
// call Reset.
func (r *Runner) Run(ctx context.Context, node syntax.Node) (retErr error) {
	span, ctx := startTelemetrySpan(ctx, "run")
	span.SetTag("rshell.version", version.Version)
	span.SetTag("rshell.run.invoked_via_cli", r.invokedViaCLI)
	if !r.disableDetailedTelemetry {
		span.SetTag("rshell.run.command", scrubCommandText(r.commandText))
		r.setRunOptionTags(span)
	}
	defer func() {
		span.SetTag("rshell.run.exit_code", int(r.exit.code))
		span.SetTag("rshell.run.commands.total", r.totalCount)
		span.SetTag("rshell.run.commands.dispatched", r.dispatchedCount)
		span.SetTag("rshell.run.commands.unallowed", r.unallowedCount)
		span.SetTag("rshell.run.commands.unknown", r.unknownCount)
		outcome := classifyRunOutcome(retErr)
		span.SetTag("rshell.run.outcome", outcome)
		// The run span reports whether the shell interpreter did its job —
		// any script completion (even with a non-zero last command or an
		// explicit `exit N`) is success from that point of view. Only
		// abnormal terminations (timeout, canceled, stdout cap hit,
		// internal fatal) flag the span as errored. Consumers distinguish
		// zero vs non-zero exits via rshell.run.exit_code, and alert on
		// policy rejections via rshell.run.unallowed_count > 0.
		if outcome == "success" {
			span.Finish(nil)
		} else {
			span.Finish(retErr)
		}
	}()

	defer func() {
		if rec := recover(); rec != nil {
			panicOut := io.Writer(io.Discard)
			if r != nil && r.stderr != nil {
				panicOut = r.stderr
			}
			func() {
				defer func() { recover() }()
				fmt.Fprintf(panicOut, "rshell: internal panic: %v\n", rec)
			}()
			retErr = fmt.Errorf("internal error")
		}
	}()
	if r.maxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxExecutionTime)
		defer cancel()
	}
	if !r.didReset {
		r.Reset()
		if r.exit.fatalExit {
			return r.exit.err
		}
	}
	// Wrap stdout and stderr with per-stream caps. Bytes beyond the limit are
	// silently discarded so that builtins never see a write error mid-execution,
	// but the exceeded flag is set so Run() can surface a well-defined error to
	// the caller after the script finishes. Restore the original writers on
	// return so that repeated Run() calls without Reset() do not double-wrap.
	prevStdout := r.stdout
	stdoutCap := &limitWriter{w: prevStdout, limit: maxStdoutBytes}
	r.stdout = stdoutCap
	defer func() { r.stdout = prevStdout }()
	prevStderr := r.stderr
	stderrCap := &limitWriter{w: prevStderr, limit: maxStderrBytes}
	r.stderr = stderrCap
	defer func() { r.stderr = prevStderr }()
	// Capture the stdin/stdout baseline at Run() start (after wrapping) so
	// per-command telemetry can detect pipe and redirect reassignments
	// without tripping on the Run-level limitWriter wrap.
	r.runStdin = r.stdin
	r.runStdout = r.stdout
	r.startTime = time.Now()
	r.globReadDirCount = &atomic.Int64{}
	r.fileRemovalCount = &atomic.Int64{}
	r.expansionByteCount = &atomic.Int64{}
	r.fillExpandConfig(ctx)
	if err := validateNode(node, r.remediationMode); err != nil {
		fmt.Fprintln(r.stderr, err)
		return ExitStatus(2)
	}
	r.exit = exitStatus{}
	r.filename = ""
	switch node := node.(type) {
	case *syntax.File:
		r.filename = node.Name
		r.stmts(ctx, node.Stmts)
	case *syntax.Stmt:
		r.stmt(ctx, node)
	case syntax.Command:
		r.cmd(ctx, node)
	default:
		return fmt.Errorf("node can only be File, Stmt, or Command: %T", node)
	}
	// Return the first of: a fatal/handler error, output cap exceeded, or the exit code.
	// Fatal errors take precedence over the cap sentinels so that cancellation
	// and handler failures are not masked when a cap is also hit. When both
	// stdout and stderr caps fire, return bothLimitsExceededError so errors.Is
	// matches either sentinel.
	if err := r.exit.err; err != nil {
		return err
	}
	stdoutEx := stdoutCap.isExceeded()
	stderrEx := stderrCap.isExceeded()
	switch {
	case stdoutEx && stderrEx:
		return bothLimitsExceededError{}
	case stdoutEx:
		return ErrOutputLimitExceeded
	case stderrEx:
		return ErrStderrLimitExceeded
	}
	if code := r.exit.code; code != 0 {
		return ExitStatus(code)
	}
	return nil
}

// setRunOptionTags records the effective [RunnerOption] configuration of r
// on the "run" span.
func (r *Runner) setRunOptionTags(span rshellTelemetrySpan) {
	mode := ModeReadOnly
	if r.remediationMode {
		mode = ModeRemediation
	}
	span.SetTag("rshell.run.options.mode", string(mode))
	span.SetTag("rshell.run.options.max_execution_time", r.maxExecutionTime.String())

	procPath := r.procPath
	if procPath == "" {
		procPath = "/proc"
	}
	span.SetTag("rshell.run.options.proc_path", procPath)
	span.SetTag("rshell.run.options.host_prefix", r.hostPrefix)

	pathEntries := make([]string, 0, len(r.sandbox.PathAccesses()))
	for _, access := range r.sandbox.PathAccesses() {
		suffix := "ro"
		if access.ReadWrite {
			suffix = "rw"
		}
		pathEntries = append(pathEntries, fmt.Sprintf("%s:%s", access.Path, suffix))
	}
	span.SetTag("rshell.run.options.allowed_paths", strings.Join(pathEntries, ","))

	span.SetTag("rshell.run.options.allow_all_commands", r.allowAllCommands)
	allowedCommands := make([]string, 0, len(r.allowedCommands))
	for name := range r.allowedCommands {
		allowedCommands = append(allowedCommands, name)
	}
	sort.Strings(allowedCommands)
	span.SetTag("rshell.run.options.allowed_commands", strings.Join(allowedCommands, ","))
	span.SetTag("rshell.run.options.elevatable_commands", strings.Join(r.elevatableCommandsList(), ","))

	allowedServices := r.allowedSystemServicesList()
	serviceEntries := make([]string, 0, len(allowedServices))
	for _, op := range allowedServices {
		serviceEntries = append(serviceEntries, fmt.Sprintf("%s:%s", op.Service, op.Action))
	}
	span.SetTag("rshell.run.options.allowed_system_services", strings.Join(serviceEntries, ","))

	span.SetTag("rshell.run.options.systemd_target_configured", r.systemdTargetConfigured)
}

// MaxScriptBytes is the maximum allowed byte length of a shell script passed
// to [ParseScript]. Scripts larger than this are rejected before parsing to
// prevent the parser from allocating unbounded memory. Unlike other per-input
// limits (variables, command substitution, per-line builtins) this cap is
// enforced at the API boundary rather than inside the interpreter, because the
// interpreter only receives the pre-parsed AST.
const MaxScriptBytes = 5 * 1024 * 1024 // 5 MiB

// ParseScript parses script as a shell program and returns the resulting AST.
// It enforces [MaxScriptBytes]: if len(script) exceeds that limit the call
// returns an error immediately, before the parser allocates any memory.
//
// name is an optional filename used in parse-error messages (pass "" if
// there is no associated file).
//
// Library callers should use ParseScript rather than calling the underlying
// syntax parser directly so that the size limit is consistently enforced.
func ParseScript(script, name string) (*syntax.File, error) {
	if len(script) > MaxScriptBytes {
		return nil, fmt.Errorf("script size %d bytes exceeds maximum of %d bytes (%d MiB); split the script into smaller pieces",
			len(script), MaxScriptBytes, MaxScriptBytes/(1024*1024))
	}
	return syntax.NewParser().Parse(strings.NewReader(script), name)
}

// Close releases resources held by the Runner, such as os.Root file descriptors
// opened by AllowedPaths. It is safe to call Close multiple times.
func (r *Runner) Close() error {
	return r.sandbox.Close()
}

// WarningsWriter routes diagnostic messages emitted while applying runner
// options (such as skipped [AllowedPaths] and [AllowedSystemServices] entries)
// to the given writer instead of the runner's stderr.
//
// Diagnostics are buffered during option processing and flushed once
// during [New], after all other options have been applied. They are not
// written again on subsequent runs.
//
// When unset, warnings fall back to the runner's stderr writer (whatever was
// supplied via [StdIO]), matching the historical behaviour. Callers that
// inspect stderr to detect command failure should pass a dedicated writer
// here so sandbox diagnostics are not conflated with command output.
//
// Passing [io.Discard] suppresses the streaming flush entirely; the messages
// remain accessible via [Runner.Warnings] regardless.
func WarningsWriter(w io.Writer) RunnerOption {
	return func(r *Runner) error {
		if w == nil {
			return fmt.Errorf("WarningsWriter: writer must not be nil")
		}
		r.warningsWriter = w
		return nil
	}
}

// Warnings returns the diagnostic messages collected during runner construction,
// one entry per warning. The slice is empty when no warnings were emitted.
//
// Callers that integrate rshell as a library can use this to surface
// configuration problems in their own structured output (e.g. a "warnings"
// field in an API response) without parsing them out of stderr.
func (r *Runner) Warnings() []string {
	if len(r.sandboxWarnings) == 0 {
		return nil
	}
	s := string(r.sandboxWarnings)
	// Each warning is terminated with "\n". Strip a single trailing newline
	// before splitting so the result does not end in a stray empty string.
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

// AllowedPaths restricts file and directory access to the specified directories.
// Paths must be absolute directories that exist. A path may end with :ro or :rw
// to indicate read-only or read-write permissions, respectively; paths without
// a suffix default to read-only. The suffix is stripped before validation. When
// set, only files within these directories can be opened (for reading or
// writing), read, or executed.
//
// The sandbox itself permits both read and write opens through os.Root;
// whether a particular shell feature (a builtin, a redirection, etc.)
// actually performs writes is a separate, layered decision. The validate
// pass currently blocks file-target output redirections (>, >>) at parse
// time, so the user-visible surface remains read-only until those layers
// opt in.
//
// When not set (default), all file access is blocked.
// An empty slice also blocks all file access.
func AllowedPaths(paths []string) RunnerOption {
	return func(r *Runner) error {
		sb, warnings, err := allowedpaths.New(paths)
		if err != nil {
			return err
		}
		r.sandbox = sb
		r.sandboxWarnings = append(r.sandboxWarnings, warnings...)
		return nil
	}
}

// HostPrefix enables container symlink resolution and sets the mount prefix
// used to translate host-absolute symlink targets. When set, symlink targets
// resolved during cross-root fallback are prepended with this prefix.
// Can be applied before or after AllowedPaths.
func HostPrefix(prefix string) RunnerOption {
	return func(r *Runner) error {
		r.hostPrefix = prefix
		return nil
	}
}

// AllowedCommands restricts command execution to the specified command names.
// Names must use the "rshell:" namespace prefix (e.g. "rshell:cat",
// "rshell:find"). Names without a colon separator or with an unknown namespace
// are rejected. The bare command name (after the prefix) is stored internally
// and matched exactly against the command name (args[0]) at execution time.
//
// Only commands whose name appears in the list may be executed; all others are
// rejected with "<cmd>: command not allowed".
//
// After prefix stripping, path-containing names (e.g. "rshell:/bin/bash")
// will not match bare command names and vice versa. Empty strings and empty
// command names are rejected.
//
// When not set (default), no commands are allowed.
func AllowedCommands(names []string) RunnerOption {
	return func(r *Runner) error {
		allowed, err := parseAllowedCommands(names)
		if err != nil {
			return err
		}
		r.allowedCommands = allowed
		return nil
	}
}

// parseAllowedCommands validates names without constructing a Runner.
func parseAllowedCommands(names []string) (map[string]bool, error) {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if n == "" {
			return nil, fmt.Errorf("AllowedCommands: empty command name")
		}
		idx := strings.Index(n, ":")
		if idx < 0 {
			return nil, fmt.Errorf("AllowedCommands: %q missing namespace prefix (expected \"rshell:<command>\")", n)
		}
		ns := n[:idx]
		cmd := n[idx+1:]
		if strings.Index(cmd, ":") >= 0 {
			return nil, fmt.Errorf("AllowedCommands: %q contains multiple colons; expected format \"rshell:<command>\"", n)
		}
		if ns != "rshell" {
			return nil, fmt.Errorf("AllowedCommands: %q has unknown namespace %q (only \"rshell\" is supported)", n, ns)
		}
		if cmd == "" {
			return nil, fmt.Errorf("AllowedCommands: %q has empty command name", n)
		}
		m[cmd] = true
	}
	return m, nil
}

// SelectiveElevation enables the "sudo <command>" marker for an explicit
// namespaced command allowlist. It does not add commands to AllowedCommands.
func SelectiveElevation(names []string, elevate ElevateFunc) RunnerOption {
	return func(r *Runner) error {
		if elevate == nil {
			return fmt.Errorf("SelectiveElevation: elevate callback is required")
		}
		m := make(map[string]bool, len(names))
		for _, n := range names {
			separator := strings.Index(n, ":")
			if separator != len("rshell") || strings.Index(n[separator+1:], ":") >= 0 || len(n) == len("rshell:") {
				return fmt.Errorf("SelectiveElevation: invalid command %q", n)
			}
			m[n[separator+1:]] = true
		}
		r.elevate = elevate
		r.elevatableCommands = m
		return nil
	}
}

func (r *Runner) elevatableCommandsList() []string {
	commands := make([]string, 0, len(r.elevatableCommands))
	for command := range r.elevatableCommands {
		if r.allowAllCommands || r.allowedCommands[command] {
			commands = append(commands, command)
		}
	}
	sort.Strings(commands)
	return commands
}

// allowAllCommandsOpt is a convenience for tests within the interp package.
func allowAllCommandsOpt() RunnerOption {
	return func(r *Runner) error {
		r.allowAllCommands = true
		return nil
	}
}

// ProcPath sets the path to the proc filesystem used by proc-aware builtins
// such as ps, pmap, and lsof.
// When not set (default), proc-aware builtins use "/proc". This option has no
// effect on non-Linux platforms.
//
// Note: bare ps (session mode) uses the host process's PID to walk the PPID
// chain. If path points to a proc filesystem from a different PID namespace,
// the host PID will likely not be found there and session output will be empty.
// ps -e and ps -p work correctly against any proc tree.
func ProcPath(path string) RunnerOption {
	return func(r *Runner) error {
		r.procPath = path
		return nil
	}
}

// Script attaches the raw script text to report on the "run" telemetry span.
// It has no effect on parsing or execution; omit it to keep the raw script
// out of telemetry.
func Script(text string) RunnerOption {
	return func(r *Runner) error {
		r.commandText = text
		return nil
	}
}

// DisableDetailedTelemetry suppresses the rshell.run.command and
// rshell.run.options.* tags on the "run" span, for when that data is too
// sensitive to forward to the telemetry backend.
func DisableDetailedTelemetry() RunnerOption {
	return func(r *Runner) error {
		r.disableDetailedTelemetry = true
		return nil
	}
}

// InvokedViaCLI marks this Runner as constructed by the cmd/rshell CLI
// rather than embedded directly by another Go program.
func InvokedViaCLI() RunnerOption {
	return func(r *Runner) error {
		r.invokedViaCLI = true
		return nil
	}
}

// Mode controls the execution mode of a Runner.
type Mode string

const (
	// ModeReadOnly is the default mode: write operations and remediation-only
	// builtins are blocked.
	ModeReadOnly Mode = "read-only"
	// ModeRemediation enables remediation-only capabilities. Filesystem writes
	// remain restricted by AllowedPaths; other capabilities retain their own
	// exact authorization policies.
	ModeRemediation Mode = "remediation"
)

// WithMode sets the execution mode of the runner. Use [ModeReadOnly] (the default)
// to block writes and remediation-only builtins, or [ModeRemediation] to enable
// capabilities such as file-target output redirections (>, >>, 2>, &>, &>>)
// within the configured [AllowedPaths] and restricted systemctl operations.
// Passing an unrecognised mode value returns an error.
func WithMode(m Mode) RunnerOption {
	return func(r *Runner) error {
		switch m {
		case ModeReadOnly:
			r.remediationMode = false
		case ModeRemediation:
			r.remediationMode = true
		default:
			return fmt.Errorf("WithMode: unrecognised execution mode %q (want %q or %q)", m, ModeReadOnly, ModeRemediation)
		}
		return nil
	}
}

// subshell creates a child Runner that inherits the parent's state.
// If background is false, the child shares the parent's environment overlay
// without copying, which is more efficient but must not be used concurrently.
func (r *Runner) subshell(background bool) *Runner {
	if !r.didReset {
		r.Reset()
	}
	r2 := &Runner{
		runnerConfig: r.runnerConfig,
		runnerState: runnerState{
			Dir:                r.Dir,
			Params:             r.Params,
			stdin:              r.stdin,
			stdout:             r.stdout,
			stderr:             r.stderr,
			runStdin:           r.runStdin,
			runStdout:          r.runStdout,
			inPipeline:         r.inPipeline,
			inPipelineStage:    r.inPipelineStage,
			filename:           r.filename,
			exit:               r.exit,
			lastExit:           r.lastExit,
			startTime:          r.startTime,
			globReadDirCount:   r.globReadDirCount,
			fileRemovalCount:   r.fileRemovalCount,
			expansionByteCount: r.expansionByteCount,
		},
	}
	r2.writeEnv = newOverlayEnviron(r.writeEnv, background)
	r2.fillExpandConfig(r.ectx)
	r2.didReset = true
	return r2
}

// classifyRunOutcome maps the error Run() is about to return to a stable,
// low-cardinality enum tag on the run span. The outcome answers "did the
// interpreter do its job", not "did the script exit zero" — any script
// completion (including a failing last command or an explicit exit N) is
// "success". Consumers that care about the exit code read
// rshell.run.exit_code; those that care about policy rejections read
// rshell.run.unallowed_count / rshell.run.unknown_count.
func classifyRunOutcome(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, ErrOutputLimitExceeded), errors.Is(err, ErrStderrLimitExceeded):
		return "output_limit"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	var status ExitStatus
	if errors.As(err, &status) {
		return "success"
	}
	return "fatal"
}
