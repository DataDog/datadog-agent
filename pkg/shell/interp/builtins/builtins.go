// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"fmt"
	"io"
	"os"
)

// HandlerFunc is the signature for a builtin command implementation.
type HandlerFunc func(ctx context.Context, callCtx *CallContext, args []string) Result

// CallContext provides the capabilities available to builtin commands.
// It is created by the Runner for each builtin invocation.
type CallContext struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  *os.File

	// InLoop is true when the builtin runs inside a for loop.
	InLoop bool

	// LastExitCode is the exit code from the previous command.
	LastExitCode uint8

	// OpenFile opens a file within the shell's path restrictions.
	OpenFile func(ctx context.Context, path string, flags int, mode os.FileMode) (io.ReadWriteCloser, error)
}

// Out writes a string to stdout.
func (c *CallContext) Out(s string) {
	io.WriteString(c.Stdout, s)
}

// Outf writes a formatted string to stdout.
func (c *CallContext) Outf(format string, a ...any) {
	fmt.Fprintf(c.Stdout, format, a...)
}

// Errf writes a formatted string to stderr.
func (c *CallContext) Errf(format string, a ...any) {
	fmt.Fprintf(c.Stderr, format, a...)
}

// Result captures the outcome of executing a builtin command.
type Result struct {
	// Code is the exit status code.
	Code uint8

	// Exiting signals that the shell should exit (set by the "exit" builtin).
	Exiting bool

	// BreakN > 0 means break out of N enclosing loops.
	BreakN int

	// ContinueN > 0 means continue from N enclosing loops.
	ContinueN int
}

var registry = map[string]HandlerFunc{
	"true":     builtinTrue,
	"false":    builtinFalse,
	"echo":     builtinEcho,
	"cat":      builtinCat,
	"exit":     builtinExit,
	"break":    builtinBreak,
	"continue": builtinContinue,
	"tail":     builtinTail,
	"grep":     builtinGrep,
}

// Lookup returns the handler for a builtin command.
func Lookup(name string) (HandlerFunc, bool) {
	fn, ok := registry[name]
	return fn, ok
}

