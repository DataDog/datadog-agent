// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// maxInteractiveOutput caps how much data we buffer while waiting for a prompt,
// so a device that never returns to its prompt can't make us buffer unbounded
// output. Running configs are typically well under 1 MiB.
const maxInteractiveOutput = 16 << 20 // 16 MiB

// runInteractive runs cmd over an interactive PTY shell session. This is
// required for devices (e.g. PAN-OS) whose CLI only produces command output on
// an interactive TTY: a non-interactive exec returns only the login banner.
//
// The flow is expect-style: wait for the login prompt, send each SetupCommand
// (waiting for the prompt after each), send Command, read until the prompt, and
// then request a clean exit. The returned string is the command output with the
// echoed command line and the trailing prompt stripped.
//
// Cancellation is handled by the caller (ExecuteCommand runs this in a goroutine
// and closes the session on context timeout, which unblocks the reads here).
func runInteractive(session *ssh.Session, cmd *profile.PlainCommand) (string, error) {
	if cmd.Prompt == nil {
		return "", fmt.Errorf("interactive command %q has no prompt matcher", cmd.Command)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("open stdin: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open stdout: %w", err)
	}

	// Disable local echo; use a large window to reduce the chance of pager
	// pagination before `set cli pager off` (or equivalent) takes effect.
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("vt100", 1000, 1000, modes); err != nil {
		return "", fmt.Errorf("request pty: %w", err)
	}
	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("start shell: %w", err)
	}

	// Wait for the initial prompt after the login banner.
	if _, err := readUntilPrompt(stdout, cmd.Prompt); err != nil {
		return "", fmt.Errorf("waiting for initial prompt: %w", err)
	}

	// Send setup commands (e.g. `set cli pager off`), discarding their output.
	for _, setup := range cmd.SetupCommands {
		if _, err := io.WriteString(stdin, setup+"\n"); err != nil {
			return "", fmt.Errorf("sending setup %q: %w", setup, err)
		}
		if _, err := readUntilPrompt(stdout, cmd.Prompt); err != nil {
			return "", fmt.Errorf("running setup %q: %w", setup, err)
		}
	}

	// Send the actual command and capture everything up to the next prompt.
	if _, err := io.WriteString(stdin, cmd.Command+"\n"); err != nil {
		return "", fmt.Errorf("sending command %q: %w", cmd.Command, err)
	}
	raw, err := readUntilPrompt(stdout, cmd.Prompt)
	if err != nil {
		return "", fmt.Errorf("running %q: %w", cmd.Command, err)
	}

	// Best-effort clean exit; ignore errors since we already have the output.
	_, _ = io.WriteString(stdin, "exit\n")
	_ = stdin.Close()

	return cleanInteractiveOutput(string(raw), cmd.Command, cmd.Prompt), nil
}

// readUntilPrompt reads from r until prompt matches the tail of the accumulated
// output, then returns everything read so far (prompt included). It returns an
// error if r is exhausted (EOF) before the prompt appears, or if the output
// exceeds maxInteractiveOutput.
func readUntilPrompt(r io.Reader, prompt *regexp.Regexp) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			// Only scan the tail: the prompt is a single short line, and this
			// keeps the match cost bounded regardless of total output size. The
			// window overlaps enough to catch a prompt split across reads.
			window := buf
			if len(window) > len(tmp) {
				window = window[len(window)-len(tmp):]
			}
			if prompt.Match(window) {
				return buf, nil
			}
			if len(buf) > maxInteractiveOutput {
				return buf, fmt.Errorf("output exceeded %d bytes without reaching prompt", maxInteractiveOutput)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return buf, fmt.Errorf("connection closed before reaching prompt")
			}
			return buf, readErr
		}
	}
}

// cleanInteractiveOutput removes the echoed command line and the trailing prompt
// from raw interactive output, and normalizes CR/LF to LF.
func cleanInteractiveOutput(raw, command string, prompt *regexp.Regexp) string {
	out := strings.ReplaceAll(raw, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")

	// Drop the echoed command line, if present, up to and including its newline.
	if i := strings.IndexByte(out, '\n'); i >= 0 && strings.Contains(out[:i], command) {
		out = out[i+1:]
	}

	// Drop the trailing prompt. Use the last match so a stray prompt-like line
	// earlier in the output (shouldn't happen with a tightly anchored prompt)
	// doesn't truncate real config.
	if locs := prompt.FindAllStringIndex(out, -1); len(locs) > 0 {
		out = out[:locs[len(locs)-1][0]]
	}

	return strings.Trim(out, " \n\t")
}
