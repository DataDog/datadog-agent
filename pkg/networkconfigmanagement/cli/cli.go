// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package cli drives a device's interactive command-line interface: it waits
// for prompts, sends lines one at a time, and separates each line's output
// from its echo and the prompt that follows. Everything that differs between
// vendors (prompt syntax, session setup, error messages, how to log out) is
// described by a Dialect, so the same Session works for any CLI.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Shell is the byte transport a Session drives. It is satisfied by
// remote.Shell.
type Shell interface {
	// Send writes data to the shell verbatim.
	Send(ctx context.Context, data string) error
	// ReadUntil waits for output until match reports a match, then returns
	// the output up to the end of the match. match returns the end index of
	// the match in buf, or -1 for none. If the shell closes or ctx ends
	// first, it returns the output buffered so far along with the error.
	ReadUntil(ctx context.Context, match func(buf []byte) int) ([]byte, error)
}

// writeCloser is implemented by shells that can close their input while
// still reading output.
type writeCloser interface {
	CloseWrite() error
}

// Dialect describes the parts of a CLI that differ between vendors.
type Dialect struct {
	// Name identifies the dialect in errors.
	Name string
	// Prompt matches the whole last line of output when the CLI is ready
	// for the next line, e.g. `R1#`. Line endings are normalized to LF
	// before matching.
	Prompt *regexp.Regexp
	// Setup lines are sent once, after the first prompt, to make the CLI
	// suitable for scripting (e.g. disabling the pager).
	Setup []string
	// Error, if set, matches output that means the CLI rejected or failed to
	// run a line.
	Error *regexp.Regexp
	// Exit is the line that ends the CLI session. If empty, End only closes
	// the shell's input.
	Exit string
}

// CheckError returns an error holding the first match of d.Error in output,
// or nil if there is none or d.Error is unset.
func (d *Dialect) CheckError(output string) error {
	if d.Error == nil {
		return nil
	}
	if msg := d.Error.FindString(output); msg != "" {
		return errors.New(msg)
	}
	return nil
}

// Session is an interactive CLI session on a Shell. It is not safe for
// concurrent use.
type Session struct {
	sh     Shell
	d      *Dialect
	prompt string
}

// Reply is the result of sending one line.
type Reply struct {
	// Output is what the CLI printed in response to the line, with line
	// endings normalized to LF and without the echoed line or the trailing
	// prompt. It is otherwise untrimmed.
	Output string
	// Prompt is the prompt that ended the output, or "" if the read stopped
	// for another reason: a pattern passed to Run, the shell closing, or ctx
	// ending. Without a prompt, the output may be incomplete.
	Prompt string
}

// Open starts a session on sh: it discards everything up to the first prompt
// (e.g. a login banner), then runs the dialect's setup lines, failing if any
// of them is rejected.
func Open(ctx context.Context, sh Shell, d *Dialect) (*Session, error) {
	s := &Session{sh: sh, d: d}
	raw, err := s.read(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("waiting for %s prompt: %w", d.Name, err)
	}
	_, s.prompt = s.splitPrompt(normalizeNewlines(string(raw)))
	for _, line := range d.Setup {
		reply, err := s.Run(ctx, line)
		if err == nil {
			err = d.CheckError(reply.Output)
		}
		if err != nil {
			return nil, fmt.Errorf("running %q: %w", line, err)
		}
	}
	return s, nil
}

// Prompt returns the last prompt the CLI printed, or "" if none has been
// recognized yet.
func (s *Session) Prompt() string {
	return s.prompt
}

// Run sends line and returns its output once the CLI prints a prompt. If
// alsoStopAt is given, the read also stops when the last line of output
// matches any of them (e.g. a confirmation question); that line is left in
// Output and Reply.Prompt is "".
//
// The error is from the shell: the shell closed or ctx ended before a prompt
// appeared. The output read so far is returned with it. Whether the CLI
// accepted the line is up to the caller (see Dialect.CheckError).
func (s *Session) Run(ctx context.Context, line string, alsoStopAt ...*regexp.Regexp) (Reply, error) {
	if err := s.sh.Send(ctx, line+"\n"); err != nil {
		return Reply{}, fmt.Errorf("could not send %q: %w", line, err)
	}
	raw, err := s.read(ctx, true, alsoStopAt...)
	text := stripEcho(normalizeNewlines(string(raw)), line)
	output, prompt := s.splitPrompt(text)
	if prompt != "" {
		s.prompt = prompt
	}
	return Reply{Output: output, Prompt: prompt}, err
}

// End ends the session as gracefully as the device allows: it sends the
// dialect's Exit line, closes the shell's input, and waits up to wait for
// the device to hang up. It is best effort and reports nothing; the caller
// still closes the shell.
func (s *Session) End(ctx context.Context, wait time.Duration) {
	endCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	if s.d.Exit != "" {
		_ = s.sh.Send(endCtx, s.d.Exit+"\n")
	}
	if wc, ok := s.sh.(writeCloser); ok {
		_ = wc.CloseWrite()
	}
	_, _ = s.sh.ReadUntil(endCtx, func([]byte) int { return -1 })
}

// read waits until the last line of output is a prompt or matches one of
// extra. With afterLine, only a last line that follows a complete line
// counts: the reply to a line always starts with its echo, or with output
// when the CLI doesn't echo, so a prompt on the first line is one the CLI
// redrew before echoing the line, not the end of the reply.
func (s *Session) read(ctx context.Context, afterLine bool, extra ...*regexp.Regexp) ([]byte, error) {
	patterns := append([]*regexp.Regexp{s.d.Prompt}, extra...)
	return s.sh.ReadUntil(ctx, func(buf []byte) int {
		if afterLine && bytes.IndexByte(buf, '\n') < 0 {
			return -1
		}
		last := lastLine(buf)
		for _, re := range patterns {
			if re.Match(last) {
				return len(buf)
			}
		}
		return -1
	})
}

// splitPrompt separates a trailing prompt line from text.
func (s *Session) splitPrompt(text string) (output, prompt string) {
	i := strings.LastIndexByte(text, '\n')
	if last := text[i+1:]; s.d.Prompt.MatchString(last) {
		return text[:i+1], last
	}
	return text, ""
}

// lastLine returns the last line of buf as it would read once line endings
// are normalized, without normalizing (or copying) the whole buffer.
func lastLine(buf []byte) []byte {
	last := buf[bytes.LastIndexByte(buf, '\n')+1:]
	return last[bytes.LastIndexByte(last, '\r')+1:]
}

// stripEcho removes the CLI's echo of line from the start of text. The echo
// is the first line; it counts if it ends with line, since some CLIs redraw
// the prompt in front of it.
func stripEcho(text, line string) string {
	first, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return text
	}
	first = strings.TrimRight(first, " \t")
	if line == "" && first != "" || !strings.HasSuffix(first, line) {
		return text
	}
	return rest
}

// normalizeNewlines turns a pty's CRLF and bare CR line endings into LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}
