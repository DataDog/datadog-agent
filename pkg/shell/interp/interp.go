// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interp provides a restricted shell interpreter that directly executes
// a small subset of POSIX shell, eliminating the TOCTOU gap of verify-then-exec.
// Only compound_list, &&/||, pipes, and for-in loops are supported.
package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Sentinel errors for loop control flow.
var (
	errBreak    = errors.New("break")
	errContinue = errors.New("continue")
)

// Runner interprets a restricted subset of shell scripts.
type Runner struct {
	vars      map[string]string // only for-loop variables
	exitCode  int
	dir       string
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	env       []string
	loopDepth int
}

// Option configures a Runner.
type Option func(*Runner)

// WithStdin sets the stdin reader.
func WithStdin(r io.Reader) Option {
	return func(run *Runner) { run.stdin = r }
}

// WithStdout sets the stdout writer.
func WithStdout(w io.Writer) Option {
	return func(run *Runner) { run.stdout = w }
}

// WithStderr sets the stderr writer.
func WithStderr(w io.Writer) Option {
	return func(run *Runner) { run.stderr = w }
}

// WithEnv sets the environment for child processes.
func WithEnv(env []string) Option {
	return func(run *Runner) { run.env = env }
}

// WithDir sets the working directory.
func WithDir(dir string) Option {
	return func(run *Runner) { run.dir = dir }
}

// New creates a Runner with the given options.
func New(opts ...Option) *Runner {
	r := &Runner{
		vars:   make(map[string]string),
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, o := range opts {
		o(r)
	}
	if r.dir == "" {
		r.dir, _ = os.Getwd()
	}
	return r
}

// ExitCode returns the exit code of the last command.
func (r *Runner) ExitCode() int {
	return r.exitCode
}

// Run parses and executes a shell script string.
func (r *Runner) Run(ctx context.Context, script string) error {
	parser := syntax.NewParser(syntax.KeepComments(false))
	f, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	return r.run(ctx, f)
}

// run executes a parsed file (compound_list of statements).
func (r *Runner) run(ctx context.Context, f *syntax.File) error {
	for _, stmt := range f.Stmts {
		if err := r.stmt(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// stmt executes a single statement.
func (r *Runner) stmt(ctx context.Context, s *syntax.Stmt) error {
	if s == nil {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if s.Background {
		return fmt.Errorf("background execution (&) is not supported")
	}
	if s.Coprocess {
		return fmt.Errorf("coprocess is not supported")
	}
	if len(s.Redirs) > 0 {
		return fmt.Errorf("redirections are not supported")
	}

	if s.Cmd == nil {
		return nil
	}

	err := r.cmd(ctx, s.Cmd)

	// Handle negation (!)
	if s.Negated {
		if err == nil {
			if r.exitCode == 0 {
				r.exitCode = 1
			} else {
				r.exitCode = 0
			}
		}
	}

	return err
}

// cmd dispatches on command type. Only CallExpr, BinaryCmd, and ForClause are allowed.
func (r *Runner) cmd(ctx context.Context, c syntax.Command) error {
	switch x := c.(type) {
	case *syntax.CallExpr:
		return r.callExpr(ctx, x)
	case *syntax.BinaryCmd:
		return r.binaryCmd(ctx, x)
	case *syntax.ForClause:
		return r.forClause(ctx, x)
	case *syntax.IfClause:
		return fmt.Errorf("if statements are not supported")
	case *syntax.WhileClause:
		return fmt.Errorf("while/until loops are not supported")
	case *syntax.CaseClause:
		return fmt.Errorf("case statements are not supported")
	case *syntax.Block:
		return fmt.Errorf("block commands ({ }) are not supported")
	case *syntax.Subshell:
		return fmt.Errorf("subshells are not supported")
	case *syntax.FuncDecl:
		return fmt.Errorf("function declarations are not supported")
	case *syntax.ArithmCmd:
		return fmt.Errorf("arithmetic commands (( )) are not supported")
	case *syntax.TestClause:
		return fmt.Errorf("test commands [[ ]] are not supported")
	case *syntax.DeclClause:
		return fmt.Errorf("declaration commands (%s) are not supported", x.Variant.Value)
	case *syntax.LetClause:
		return fmt.Errorf("let command is not supported")
	case *syntax.TimeClause:
		return fmt.Errorf("time command is not supported")
	case *syntax.CoprocClause:
		return fmt.Errorf("coproc is not supported")
	default:
		return fmt.Errorf("unsupported command type: %T", c)
	}
}

// callExpr handles simple commands (command + arguments).
func (r *Runner) callExpr(ctx context.Context, c *syntax.CallExpr) error {
	if c == nil {
		return nil
	}

	// Reject all variable assignments (standalone or prefix).
	if len(c.Assigns) > 0 {
		return fmt.Errorf("variable assignment is not supported")
	}

	// No args means empty statement.
	if len(c.Args) == 0 {
		return nil
	}

	// The command name must be a literal string in the AST.
	cmdName, ok := literalWordValue(c.Args[0])
	if !ok {
		return fmt.Errorf("command name must be a literal string (dynamic command names are not allowed)")
	}

	// Expand all argument words (command name + args).
	args, err := r.expandFields(c.Args[1:])
	if err != nil {
		return err
	}

	// Try builtin first.
	if isBuiltin, bErr := r.builtin(ctx, cmdName, args); isBuiltin {
		return bErr
	}

	// External command.
	return r.execCommand(ctx, cmdName, args)
}

// binaryCmd handles &&, ||, and |.
func (r *Runner) binaryCmd(ctx context.Context, c *syntax.BinaryCmd) error {
	if c == nil {
		return nil
	}

	switch c.Op {
	case syntax.AndStmt: // &&
		if err := r.stmt(ctx, c.X); err != nil {
			return err
		}
		if r.exitCode != 0 {
			return nil
		}
		return r.stmt(ctx, c.Y)

	case syntax.OrStmt: // ||
		if err := r.stmt(ctx, c.X); err != nil {
			return err
		}
		if r.exitCode == 0 {
			return nil
		}
		return r.stmt(ctx, c.Y)

	case syntax.Pipe: // |
		return r.pipe(ctx, c.X, c.Y)

	case syntax.PipeAll: // |&
		return fmt.Errorf("pipe with stderr (|&) is not supported")

	default:
		return fmt.Errorf("unsupported binary operator: %v", c.Op)
	}
}

// forClause handles for-in loops.
func (r *Runner) forClause(ctx context.Context, c *syntax.ForClause) error {
	if c == nil {
		return nil
	}

	if c.Select {
		return fmt.Errorf("select statement is not supported")
	}

	loop, ok := c.Loop.(*syntax.WordIter)
	if !ok {
		return fmt.Errorf("only for-in loops are supported (no C-style for loops)")
	}

	// Expand the iteration items.
	items, err := r.expandFields(loop.Items)
	if err != nil {
		return err
	}

	varName := loop.Name.Value

	r.loopDepth++
	defer func() { r.loopDepth-- }()

	for _, item := range items {
		r.vars[varName] = item

		bodyErr := r.runStmts(ctx, c.Do)
		if bodyErr != nil {
			if errors.Is(bodyErr, errBreak) {
				break
			}
			if errors.Is(bodyErr, errContinue) {
				continue
			}
			return bodyErr
		}
	}

	// Clean up the loop variable.
	delete(r.vars, varName)

	return nil
}

// runStmts executes a list of statements sequentially.
func (r *Runner) runStmts(ctx context.Context, stmts []*syntax.Stmt) error {
	for _, s := range stmts {
		if err := r.stmt(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// pipe connects the stdout of the left command to the stdin of the right command.
func (r *Runner) pipe(ctx context.Context, x, y *syntax.Stmt) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	// Run left side with stdout redirected to pipe writer.
	leftRunner := r.clone()
	leftRunner.stdout = pw

	// Run right side with stdin from pipe reader.
	rightRunner := r.clone()
	rightRunner.stdin = pr

	errc := make(chan error, 1)
	go func() {
		errc <- leftRunner.stmt(ctx, x)
		pw.Close()
	}()

	rightErr := rightRunner.stmt(ctx, y)
	pr.Close()

	leftErr := <-errc

	// Propagate the right side's exit code (POSIX convention: pipefail=off).
	r.exitCode = rightRunner.exitCode

	if rightErr != nil {
		return rightErr
	}
	return leftErr
}

// clone creates a shallow copy of the runner for use in pipes.
func (r *Runner) clone() *Runner {
	vars := make(map[string]string, len(r.vars))
	for k, v := range r.vars {
		vars[k] = v
	}
	return &Runner{
		vars:      vars,
		exitCode:  r.exitCode,
		dir:       r.dir,
		stdin:     r.stdin,
		stdout:    r.stdout,
		stderr:    r.stderr,
		env:       r.env,
		loopDepth: r.loopDepth,
	}
}
