// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"unicode"
	"unicode/utf8"
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
}

// Lookup returns the handler for a builtin command.
func Lookup(name string) (HandlerFunc, bool) {
	fn, ok := registry[name]
	return fn, ok
}

// FormatOSError extracts the underlying syscall error from a wrapped error
// (e.g., *os.PathError) and capitalizes the first letter to match the
// format used by coreutils and bash. Non-syscall errors (such as path
// escape or custom permission errors) are returned unchanged.
func FormatOSError(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if _, ok := pathErr.Err.(syscall.Errno); ok {
			msg := pathErr.Err.Error()
			r, size := utf8.DecodeRuneInString(msg)
			if size > 0 && unicode.IsLower(r) {
				msg = string(unicode.ToUpper(r)) + msg[size:]
			}
			return msg
		}
	}
	return err.Error()
}

// IsSyscallPathError reports whether err is an *os.PathError wrapping
// a syscall.Errno. This is used to decide whether to reformat the
// error message (e.g., for redirects) or keep the original.
func IsSyscallPathError(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		_, ok := pathErr.Err.(syscall.Errno)
		return ok
	}
	return false
}
