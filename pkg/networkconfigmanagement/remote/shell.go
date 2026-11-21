// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"context"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// sshShell implements Shell over an SSH session with a PTY.
//
// The ssh library copies the session's output into the shell (via Write) on
// its own goroutine, so output is always drained even when nobody is reading;
// this keeps the channel's flow-control window open and lets ReadUntil honor
// its context.
//
// Only writeLoop touches stdin. The ssh library does not allow closing a
// channel's input while a write to it is in progress, and a Send abandoned on
// ctx can leave its write running, so writes and the final close are
// serialized on that one goroutine.
type sshShell struct {
	session *ssh.Session
	stdin   io.WriteCloser

	writes    chan writeReq // to writeLoop; unbuffered
	inputDone chan struct{} // closed once input is closed
	inputOnce sync.Once

	mu     sync.Mutex
	buf    []byte
	notify chan struct{} // signaled (non-blocking) whenever buf grows

	done      chan struct{} // closed once the session has exited
	closedErr error         // why the session exited; set before done is closed

	closeOnce sync.Once
}

var _ Shell = (*sshShell)(nil)

var errInputClosed = errors.New("shell input is closed")

// writeReq asks writeLoop to write data; the result is sent on done, which
// must be buffered so writeLoop never waits on an abandoned Send.
type writeReq struct {
	data string
	done chan error
}

// StartShell opens an interactive shell with a PTY on the device. If ctx ends
// before setup completes, any session already opened is closed and the ctx
// error is returned.
func (c *SSHConnection) StartShell(ctx context.Context) (Shell, types.TypedError) {
	if err := ctx.Err(); err != nil {
		return nil, types.WrapErrorf(types.ErrCannotConnect, "unable to start shell on %q: %w", c.ipAddress, err)
	}

	type result struct {
		sh  Shell
		err types.TypedError
	}
	var (
		mu        sync.Mutex
		session   *ssh.Session
		abandoned bool // set once StartShell has given up on ctx
	)
	resCh := make(chan result, 1) // buffered so the goroutine never blocks
	go func() {
		s, err := c.client.NewSession()
		if err != nil {
			resCh <- result{err: types.WrapErrorf(types.ErrCannotConnect, "unable to open session to %q: %w", c.ipAddress, err)}
			return
		}
		mu.Lock()
		if abandoned {
			mu.Unlock()
			_ = s.Close()
			return
		}
		session = s
		mu.Unlock()
		sh, err := startShell(s)
		if err != nil {
			_ = s.Close()
			resCh <- result{err: types.WrapErrorf(types.ErrCannotConnect, "unable to start shell on %q: %w", c.ipAddress, err)}
			return
		}
		resCh <- result{sh: sh}
	}()

	select {
	case r := <-resCh:
		return r.sh, r.err
	case <-ctx.Done():
	}
	// Closing the session unblocks any pending setup request, and also tears
	// down a shell whose setup finished concurrently with cancellation.
	mu.Lock()
	abandoned = true
	if session != nil {
		_ = session.Close()
	}
	mu.Unlock()
	return nil, types.WrapErrorf(types.ErrCannotConnect, "unable to start shell on %q: %w", c.ipAddress, ctx.Err())
}

func startShell(session *ssh.Session) (*sshShell, error) {
	if err := session.RequestPty("vt100", 24, 512, ssh.TerminalModes{}); err != nil {
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	sh := &sshShell{
		session:   session,
		stdin:     stdin,
		writes:    make(chan writeReq),
		inputDone: make(chan struct{}),
		notify:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	session.Stdout = sh
	session.Stderr = sh
	if err := session.Shell(); err != nil {
		return nil, err
	}
	go sh.writeLoop()
	go func() {
		// Wait returns only after the library has finished copying output
		// into sh, so nothing written before done closes is lost.
		err := session.Wait()
		if err == nil {
			err = io.EOF
		}
		sh.closedErr = err
		close(sh.done)
	}()
	return sh, nil
}

// Write appends session output to the buffer. It is called by the ssh library.
func (s *sshShell) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.buf = append(s.buf, p...)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return len(p), nil
}

// writeLoop performs every write to stdin, in order, and closes stdin once
// input is closed. A write already accepted finishes before the close, so the
// device sees EOF after the data sent before it.
func (s *sshShell) writeLoop() {
	defer func() { _ = s.stdin.Close() }()
	for {
		select {
		case req := <-s.writes:
			_, err := io.WriteString(s.stdin, req.data)
			req.done <- err
		case <-s.inputDone:
			return
		}
	}
}

// closeInput stops accepting writes and has writeLoop close stdin. It never
// waits for a write in progress.
func (s *sshShell) closeInput() {
	s.inputOnce.Do(func() { close(s.inputDone) })
}

// Send writes data to the shell's stdin. If ctx ends before the write
// completes, the session is closed to unblock it. It fails once input is
// closed.
func (s *sshShell) Send(ctx context.Context, data string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req := writeReq{data: data, done: make(chan error, 1)}
	select {
	case s.writes <- req:
	case <-s.inputDone:
		return errInputClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		// Closing the session is safe during a write, and unblocks one that
		// is waiting on the device.
		_ = s.session.Close()
		return ctx.Err()
	}
}

// ReadUntil returns buffered output up to the end of the first match, waiting
// for more output as needed. It is not safe for concurrent use.
func (s *sshShell) ReadUntil(ctx context.Context, match func(buf []byte) int) ([]byte, error) {
	for {
		s.mu.Lock()
		if i := match(s.buf); i >= 0 {
			out := s.buf[:i:i]
			s.buf = s.buf[i:]
			s.mu.Unlock()
			return out, nil
		}
		select {
		case <-s.done:
			// No more output is coming, and what we have doesn't match.
			out := s.buf
			s.buf = nil
			s.mu.Unlock()
			return out, s.closedErr
		default:
		}
		pending := append([]byte(nil), s.buf...)
		s.mu.Unlock()

		select {
		case <-s.notify:
		case <-s.done:
			// Loop around to check the final output for a match.
		case <-ctx.Done():
			return pending, ctx.Err()
		}
	}
}

// CloseWrite closes the shell's stdin, sending EOF to the device after any
// write in progress. It does not wait for that write.
func (s *sshShell) CloseWrite() error {
	s.closeInput()
	return nil
}

// Close closes the shell's session. It is safe to call more than once.
func (s *sshShell) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.closeInput()
		err = s.session.Close()
		if errors.Is(err, io.EOF) {
			err = nil
		}
	})
	return err
}
