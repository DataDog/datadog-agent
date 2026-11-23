// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The check runs without a deadline, and unlike an exec session an interactive
// shell never ends on its own, so every wait for the prompt is bounded.
var (
	// interactiveLoginTimeout bounds the wait for the first prompt after login.
	// It is short because a device that doesn't show the expected prompt (e.g.
	// while testing profiles against an unknown device) would otherwise stall
	// the check.
	interactiveLoginTimeout = 30 * time.Second
	// interactiveCommandTimeout bounds each command, including printing a large
	// running config.
	interactiveCommandTimeout = 5 * time.Minute
)

// ansiEscape matches terminal control sequences a device may print on a PTY.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// ExecuteInteractive runs cmd in an interactive shell opened by startShell: it
// waits for cmd.Prompt, sends each setup command and then cmd.Command, and
// waits for the prompt again after each one. The returned result holds the
// output of cmd.Command with the echoed command line and the trailing prompt
// removed, and it is validated like the output of ExecuteCommand.
func ExecuteInteractive(ctx context.Context, startShell func(context.Context) (Shell, types.TypedError), cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if cmd.Prompt == nil {
		return nil, fmt.Errorf("command %q has no prompt to run interactively", cmd.Command)
	}
	// The login deadline covers opening the shell too: a peer can accept the
	// connection and then never answer the session, PTY or shell request.
	loginCtx, cancelLogin := context.WithTimeout(ctx, interactiveLoginTimeout)
	defer cancelLogin()
	sh, terr := startShell(loginCtx)
	if terr != nil {
		return nil, terr
	}
	defer sh.Close()

	result := &types.CommandResult{CommandStr: cmd.Command}
	fail := func(err error) (*types.CommandResult, error) {
		result.Error = err.Error()
		return result, result.FormattedError()
	}

	if out, err := readPrompt(loginCtx, sh, cmd.Prompt, interactiveLoginTimeout); err != nil {
		result.Output = string(cleanOutput(out, "", nil))
		return fail(fmt.Errorf("waiting for the device prompt: %w", err))
	}
	for _, setup := range cmd.SetupCommands {
		out, err := runLine(ctx, sh, cmd.Prompt, setup)
		if err != nil {
			result.Output = string(out)
			return fail(fmt.Errorf("running setup command %q: %w", setup, err))
		}
		if len(bytes.TrimSpace(out)) > 0 {
			// Setup commands normally print nothing; output here is usually an
			// error such as "Invalid syntax." that explains a later failure.
			log.Debugf("ncm: setup command %q printed %q", setup, out)
		}
	}
	out, err := runLine(ctx, sh, cmd.Prompt, cmd.Command)
	if err != nil {
		// Leave the output out of the error: it is a partial config that
		// hasn't been redacted yet, and the error ends up in the check status.
		return fail(fmt.Errorf("%w (%d bytes received)", err, len(out)))
	}
	result.Output = string(out)
	// Best effort: we have the output, and Close ends the session anyway.
	_ = sh.Send(ctx, "exit\n")

	cmd.Validator.ValidateResult(result)
	return result, result.FormattedError()
}

// runLine sends line and returns its output, without the echoed line and the
// prompt that follows it.
func runLine(ctx context.Context, sh Shell, prompt *regexp.Regexp, line string) ([]byte, error) {
	if err := sh.Send(ctx, line+"\n"); err != nil {
		return nil, fmt.Errorf("sending %q: %w", line, err)
	}
	out, err := readPrompt(ctx, sh, prompt, interactiveCommandTimeout)
	if err != nil {
		return cleanOutput(out, line, nil), fmt.Errorf("waiting for the prompt after %q: %w", line, err)
	}
	return cleanOutput(out, line, prompt), nil
}

// readPrompt reads until the output ends with the prompt.
func readPrompt(ctx context.Context, sh Shell, prompt *regexp.Regexp, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sh.ReadUntil(ctx, func(buf []byte) int {
		// The prompt is short; only look at the end of the buffer so large
		// outputs don't make each read quadratic.
		const window = 512
		tail := buf
		if len(tail) > window {
			tail = tail[len(tail)-window:]
		}
		if prompt.Match(ansiEscape.ReplaceAll(tail, nil)) {
			return len(buf)
		}
		return -1
	})
}

// cleanOutput normalizes line endings, strips terminal escape sequences, and
// removes the echoed command line and, when prompt is set, the trailing prompt.
func cleanOutput(raw []byte, line string, prompt *regexp.Regexp) []byte {
	out := ansiEscape.ReplaceAll(raw, nil)
	out = bytes.ReplaceAll(out, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), nil)
	if line != "" {
		if i := bytes.IndexByte(out, '\n'); i >= 0 && bytes.Contains(out[:i], []byte(line)) {
			out = out[i+1:]
		}
	}
	if prompt != nil {
		if loc := prompt.FindIndex(out); loc != nil {
			out = out[:loc[0]]
		}
	}
	return bytes.TrimRight(bytes.TrimLeft(out, "\n"), " \n")
}
