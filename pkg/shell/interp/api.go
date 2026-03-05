// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

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

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// A Runner interprets shell programs. It can be reused, but it is not safe for
// concurrent use. Use [New] to build a new Runner.
//
// Runner's exported fields are meant to be configured via [RunnerOption];
// once a Runner has been created, the fields should be treated as read-only.
type Runner struct {
	// Env specifies the initial environment for the interpreter, which must
	// not be nil. It can only be set via [Env].
	Env expand.Environ

	// writeEnv overlays [Runner.Env] so that we can write environment variables
	// as an overlay.
	writeEnv expand.WriteEnviron

	// Dir specifies the working directory of the command, which must be an
	// absolute path.
	Dir string

	// Params are the current shell parameters, e.g. from running a shell
	// file. Note: positional parameter expansion ($@, $*, $1, etc.) is
	// blocked by the AST validator in this restricted interpreter.
	Params []string

	// execHandler is responsible for executing programs. It must not be nil.
	execHandler ExecHandlerFunc

	// openHandler is a function responsible for opening files. It must not be nil.
	openHandler OpenHandlerFunc

	// readDirHandler is a function responsible for reading directories during
	// glob expansion. It must be non-nil.
	readDirHandler ReadDirHandlerFunc

	stdin  *os.File // e.g. the read end of a pipe
	stdout io.Writer
	stderr io.Writer

	ecfg *expand.Config
	ectx context.Context // just so that subshell can use it again

	// didReset remembers whether the runner has ever been reset. This is
	// used so that Reset is automatically called when running any program
	// or node for the first time on a Runner.
	didReset bool

	usedNew bool

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

	// allowedPaths restricts file/directory access to these directories.
	// Empty (default) blocks all file access; populate via AllowedPaths option.
	allowedPaths []string
	// roots holds opened os.Root instances, one per allowedPaths entry.
	roots []*os.Root

	origDir    string
	origParams []string
	origStdin  *os.File
	origStdout io.Writer
	origStderr io.Writer

}

// exitStatus holds the state of the shell after running one command.
// Beyond the exit status code, it also holds whether the shell should return or exit,
// as well as any Go error values that should be given back to the user.
type exitStatus struct {
	// code is the exit status code.
	code uint8

	exiting bool // whether the current shell is exiting
	fatalExit bool // whether the current shell is exiting due to a fatal error; err below must not be nil

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
	r := &Runner{
		usedNew:        true,
		openHandler:    defaultOpenHandler(),
		readDirHandler: defaultReadDirHandler(),
	}
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	// Set the default fallbacks, if necessary.
	// Default to an empty environment to avoid propagating parent env vars.
	if r.Env == nil {
		r.Env = expand.ListEnviron()
	}
	if r.Dir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("could not get current dir: %w", err)
		}
		r.Dir = dir
	}
	if r.stdout == nil || r.stderr == nil {
		StdIO(r.stdin, r.stdout, r.stderr)(r)
	}
	return r, nil
}

// RunnerOption can be passed to [New] to alter a [Runner]'s behaviour.
type RunnerOption func(*Runner) error

func stdinFile(r io.Reader) (*os.File, error) {
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
			io.Copy(pw, r)
			pw.Close()
		}()
		return pr, nil
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
func StdIO(in io.Reader, out, err io.Writer) RunnerOption {
	return func(r *Runner) error {
		stdin, _err := stdinFile(in)
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

// Reset returns a runner to its initial state, right before the first call to
// Run or Reset.
//
// Typically, this function only needs to be called if a runner is reused to run
// multiple programs non-incrementally. Not calling Reset between each run will
// mean that the shell state will be kept, including variables, options, and the
// current directory.
func (r *Runner) Reset() {
	if !r.usedNew {
		panic("use interp.New to construct a Runner")
	}
	if !r.didReset {
		r.origDir = r.Dir
		r.origParams = r.Params
		r.origStdin = r.stdin
		r.origStdout = r.stdout
		r.origStderr = r.stderr

		if r.execHandler == nil {
			r.execHandler = noExecHandler()
		}
		// Open os.Root handles and wrap handlers for path restriction.
		// Default: block all file access (empty allowedPaths).
		if r.roots == nil {
			r.roots = make([]*os.Root, len(r.allowedPaths))
			for i, p := range r.allowedPaths {
				root, err := os.OpenRoot(p)
				if err != nil {
					for _, prev := range r.roots[:i] {
						prev.Close()
					}
					r.exit.fatal(fmt.Errorf("AllowedPaths: cannot open root %q: %w", p, err))
					return
				}
				r.roots[i] = root
			}
			r.openHandler = wrapOpenHandler(r.roots, r.allowedPaths)
			r.readDirHandler = wrapReadDirHandler(r.roots, r.allowedPaths)
			r.execHandler = wrapExecHandler(r.roots, r.allowedPaths, r.execHandler)
		}
	}
	// reset the internal state
	*r = Runner{
		Env:             r.Env,
		execHandler:     r.execHandler,
		openHandler:     r.openHandler,
		readDirHandler:  r.readDirHandler,

		allowedPaths: r.allowedPaths,
		roots:        r.roots,

		// These can be set by functions like [Dir] or [Params], but
		// builtins can overwrite them; reset the fields to whatever the
		// constructor set up.
		Dir:    r.origDir,
		Params: r.origParams,
		stdin:  r.origStdin,
		stdout: r.origStdout,
		stderr: r.origStderr,

		origDir:    r.origDir,
		origParams: r.origParams,
		origStdin:  r.origStdin,
		origStdout: r.origStdout,
		origStderr: r.origStderr,

		usedNew: r.usedNew,
	}
	r.writeEnv = &overlayEnviron{parent: r.Env}
	r.setVarString("PWD", r.Dir)
	r.setVarString("IFS", " \t\n")
	r.setVarString("OPTIND", "1")

	r.didReset = true
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
func (r *Runner) Run(ctx context.Context, node syntax.Node) error {
	if !r.didReset {
		r.Reset()
	}
	r.fillExpandConfig(ctx)
	if err := validateNode(node); err != nil {
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
	// Return the first of: a fatal error, a non-fatal handler error, or the exit code.
	if err := r.exit.err; err != nil {
		return err
	}
	if code := r.exit.code; code != 0 {
		return ExitStatus(code)
	}
	return nil
}

// Close releases resources held by the Runner, such as os.Root file descriptors
// opened by AllowedPaths. It is safe to call Close multiple times.
func (r *Runner) Close() error {
	for _, root := range r.roots {
		root.Close()
	}
	r.roots = nil
	return nil
}

// subshell creates a child Runner that inherits the parent's state.
// If background is false, the child shares the parent's environment overlay
// without copying, which is more efficient but must not be used concurrently.
func (r *Runner) subshell(background bool) *Runner {
	if !r.didReset {
		r.Reset()
	}
	// Keep in sync with the Runner type. Manually copy fields, to not copy
	// sensitive ones, and to do deep copies of slices.
	r2 := &Runner{
		Dir:             r.Dir,
		Params:          r.Params,
		execHandler:     r.execHandler,
		openHandler:     r.openHandler,
		readDirHandler:  r.readDirHandler,

		allowedPaths: r.allowedPaths,
		roots:        r.roots, // safe: os.Root is goroutine-safe

		stdin:          r.stdin,
		stdout:         r.stdout,
		stderr:         r.stderr,
		filename:       r.filename,
		usedNew:        r.usedNew,
		exit:           r.exit,
		lastExit:       r.lastExit,

	}
	r2.writeEnv = newOverlayEnviron(r.writeEnv, background)
	r2.fillExpandConfig(r.ectx)
	r2.didReset = true
	return r2
}
