// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// maxEnableHandshakeBytes bounds how much output we buffer while waiting for
// the enable password prompt (or, if configured, its validator to resolve),
// to avoid growing without bound if a device never produces the expected text.
const maxEnableHandshakeBytes = 64 * 1024

// ExecuteCommandWithEnable runs enableCmd (answering its password prompt with
// password) and, if that succeeds, runs cmd - all within the same interactive
// SSH session. Unlike ExecuteCommand, this requires a PTY/shell since the
// enable password prompt needs mid-stream interaction that a non-interactive
// exec channel cannot provide.
//
// If the password prompt is never detected, or enableCmd.Validator rejects the
// response (or fails to match Require rules within the byte budget), cmd is
// never sent and an error is returned.
func ExecuteCommandWithEnable(ctx context.Context, client sshClient, cmd *profile.PlainCommand, enableCmd *profile.EnableCommand, password string) (*types.CommandResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	ch := make(chan *types.CommandResult, 1)
	go func() {
		output, err := runEnableThenCommand(session, enableCmd, password, cmd.Command)
		ch <- &types.CommandResult{
			CommandStr: cmd.Command,
			Output:     output,
			Error:      errorStr(err),
		}
	}()
	select {
	case r := <-ch:
		cmd.Validator.ValidateResult(r)
		return r, r.FormattedError()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runEnableThenCommand(session *ssh.Session, enableCmd *profile.EnableCommand, password string, realCommand string) (output string, err error) {
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	rawStdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	stdout := bufio.NewReader(rawStdout)

	if err := session.RequestPty("vt100", 80, 200, ssh.TerminalModes{}); err != nil {
		return "", fmt.Errorf("enable: could not allocate pty: %w", err)
	}
	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("enable: could not start shell: %w", err)
	}

	// Send the enable command and wait for the password prompt.
	if _, err := fmt.Fprintf(stdin, "%s\n", enableCmd.Command); err != nil {
		return "", fmt.Errorf("enable: failed to send enable command: %w", err)
	}
	if _, err := readUntilMatch(stdout, enableCmd.EffectivePasswordPrompt()); err != nil {
		return "", fmt.Errorf("enable: password prompt not detected: %w", err)
	}

	// Send the password.
	if _, err := fmt.Fprintf(stdin, "%s\n", password); err != nil {
		return "", fmt.Errorf("enable: failed to send password: %w", err)
	}

	// If a validator is configured, confirm enable actually succeeded before
	// proceeding. If not configured, we can't distinguish success from
	// failure here, so we optimistically proceed (matches pre-existing
	// behavior for profiles that don't set a Validator).
	if len(enableCmd.Validator.Require) > 0 || len(enableCmd.Validator.Reject) > 0 {
		if err := readUntilValid(stdout, enableCmd.Validator); err != nil {
			return "", fmt.Errorf("enable: %w", err)
		}
	}

	// Enable succeeded (or wasn't checked) - run the real command and end the
	// session.
	if _, err := fmt.Fprintf(stdin, "%s\n", realCommand); err != nil {
		return "", fmt.Errorf("enable: failed to send command: %w", err)
	}
	if _, err := fmt.Fprintln(stdin, "exit"); err != nil {
		return "", fmt.Errorf("enable: failed to send exit: %w", err)
	}
	_ = stdin.Close()
	_ = session.Wait()

	result, _ := io.ReadAll(stdout)
	return stripCommandEcho(string(result), realCommand), nil
}

// readUntilMatch reads from r one byte at a time, accumulating into a buffer,
// until re matches the accumulated buffer. It never reads more than
// maxEnableHandshakeBytes bytes.
func readUntilMatch(r *bufio.Reader, re *regexp.Regexp) ([]byte, error) {
	var buf []byte
	for len(buf) < maxEnableHandshakeBytes {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return buf, io.ErrUnexpectedEOF
			}
			return buf, err
		}
		buf = append(buf, b)
		if re.Match(buf) {
			return buf, nil
		}
	}
	return buf, fmt.Errorf("expected output not found within %d bytes", maxEnableHandshakeBytes)
}

// readUntilValid reads from r one byte at a time, accumulating into a buffer,
// until a Reject rule matches (failure, returned immediately without waiting
// for more output) or the response can be deemed successful. If Require rules
// are configured, success means all of them match. Otherwise - a Reject-only
// (or empty) Validator can't positively confirm success from matching text
// alone, so it waits for the device to return to its CLI prompt, which is
// itself a signal that no more output (e.g. a rejection) is coming.
func readUntilValid(r *bufio.Reader, v profile.Validator) error {
	var buf []byte
	for len(buf) < maxEnableHandshakeBytes {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		buf = append(buf, b)
		text := string(buf)
		for _, rule := range v.Reject {
			if rule.MatchString(text) {
				return fmt.Errorf("matches failure regex %q", rule)
			}
		}
		if len(v.Require) > 0 {
			if v.Validate(text) == nil {
				return nil
			}
			continue
		}
		if trailingPromptRE.MatchString(lastLine(text)) {
			return nil
		}
	}
	return fmt.Errorf("expected response not found within %d bytes", maxEnableHandshakeBytes)
}

// stripCommandEcho removes a leading echoed command line, if present, and any
// trailing device CLI prompt line - both artifacts of running commands over
// an interactive shell/pty rather than a non-interactive exec channel.
func stripCommandEcho(output string, realCommand string) string {
	output = trimLeadingEcho(output, realCommand)
	output = trimTrailingPrompt(output)
	return output
}

// trimLeadingEcho drops leading blank lines (left over from the newline that
// terminates the enable password) followed by the device's echo of
// realCommand as typed at the interactive shell, if present.
func trimLeadingEcho(output string, realCommand string) string {
	rest := output
	for {
		line, tail, found := strings.Cut(rest, "\n")
		if !found {
			return output
		}
		if strings.TrimSpace(line) == "" {
			rest = tail
			continue
		}
		if strings.TrimRight(line, "\r") == realCommand {
			return tail
		}
		return output
	}
}

// trailingPromptRE matches a typical device CLI prompt line, e.g. "Router#"
// or "switch>".
var trailingPromptRE = regexp.MustCompile(`^\S+[#>]\s*$`)

// trimTrailingPrompt drops a trailing device CLI prompt line, if present.
func trimTrailingPrompt(output string) string {
	trimmed := strings.TrimRight(output, "\r\n")
	lastNL := strings.LastIndexByte(trimmed, '\n')
	if trailingPromptRE.MatchString(trimmed[lastNL+1:]) {
		return trimmed[:lastNL+1]
	}
	return output
}

// lastLine returns the text after the final newline in s (or all of s if it
// contains none), after trimming trailing carriage returns/newlines.
func lastLine(s string) string {
	trimmed := strings.TrimRight(s, "\r\n")
	lastNL := strings.LastIndexByte(trimmed, '\n')
	return trimmed[lastNL+1:]
}
