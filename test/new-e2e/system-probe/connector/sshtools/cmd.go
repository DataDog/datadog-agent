// Copyright (C) 2017 ScyllaDB

// Package sshtools allows execution of SSH commands easily
package sshtools

import (
	"context"
	"fmt"
	"io"
)

// Cmd represents a remote command being prepared or run.
type Cmd struct {
	// Command is the command to run remotely. This is executed as if
	// it were a shell command, so you are expected to do any shell escaping
	// necessary.
	Command string

	// Env specifies the environment of the process.
	// Each entry is of the form "key=value".
	Env []string

	// Stdin specifies the process's standard input. If Stdin is nil,
	// the process reads from an empty bytes.Buffer.
	Stdin io.Reader

	// Stdout and Stderr represent the process's standard output and error.
	//
	// If either is nil, it will be set to ioutil.Discard.
	Stdout io.Writer
	Stderr io.Writer

	// Internal fields
	ctx    context.Context
	closer io.Closer

	exitStatus int
	err        error
	exitCh     chan struct{} // protects exitStatus and err
}

// Init must be called by the Communicator before executing the command.
func (c *Cmd) init(ctx context.Context, closer io.Closer) {
	c.ctx = ctx
	c.closer = closer
	c.exitCh = make(chan struct{})
}

// setExitStatus stores the exit status of the remote command as well as any
// communicator related error. SetExitStatus then unblocks any pending calls
// to Wait.
// This should only be called by communicators executing the remote.Cmd.
func (c *Cmd) setExitStatus(status int, err error) {
	c.exitStatus = status
	c.err = err

	close(c.exitCh)
}

// Wait waits for the remote command completion or cancellation.
// Wait may return an error from the communicator, or an ExitError if the
// process exits with a non-zero exit status.
func (c *Cmd) Wait() error {
	select {
	case <-c.ctx.Done():
		c.closer.Close()
		return c.ctx.Err()
	case <-c.exitCh:
		// continue
	}

	if c.err != nil || c.exitStatus != 0 {
		return &ExitError{
			Command:    c.Command,
			ExitStatus: c.exitStatus,
			Err:        c.err,
		}
	}

	return nil
}

// ExitError is returned by Wait to indicate an error while executing the remote
// command, or a non-zero exit status.
type ExitError struct {
	Command    string
	ExitStatus int
	Err        error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("error executing %q: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("%q exit status: %d", e.Command, e.ExitStatus)
}
