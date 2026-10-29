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
type sshShell struct {
	session *ssh.Session
	stdin   io.WriteCloser

	mu     sync.Mutex
	buf    []byte
	notify chan struct{} // signaled (non-blocking) whenever buf grows

	done      chan struct{} // closed once the session has exited
	closedErr error         // why the session exited; set before done is closed

	closeOnce sync.Once
}

var _ Shell = (*sshShell)(nil)

// StartShell opens an interactive shell with a PTY on the device.
func (c *SSHConnection) StartShell(_ context.Context) (Shell, types.TypedError) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, types.WrapErrorf(types.ErrCannotConnect, "unable to open session to %q: %w", c.ipAddress, err)
	}
	sh, err := startShell(session)
	if err != nil {
		_ = session.Close()
		return nil, types.WrapErrorf(types.ErrCannotConnect, "unable to start shell on %q: %w", c.ipAddress, err)
	}
	return sh, nil
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
		session: session,
		stdin:   stdin,
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	session.Stdout = sh
	session.Stderr = sh
	if err := session.Shell(); err != nil {
		return nil, err
	}
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

// Send writes data to the shell's stdin. If ctx ends before the write
// completes, the session is closed to unblock it.
func (s *sshShell) Send(ctx context.Context, data string) error {
	errCh := make(chan error, 1)
	go func() {
		_, err := io.WriteString(s.stdin, data)
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		_ = s.Close()
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

// CloseWrite closes the shell's stdin, sending EOF to the device.
func (s *sshShell) CloseWrite() error {
	return s.stdin.Close()
}

// Close closes the shell's session. It is safe to call more than once.
func (s *sshShell) Close() error {
	var err error
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		err = s.session.Close()
		if errors.Is(err, io.EOF) {
			err = nil
		}
	})
	return err
}
