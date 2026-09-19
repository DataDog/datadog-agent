// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// sshClient is a common interface between ssh.Client and RetryingSSHClient
type sshClient interface {
	NewSession() (*ssh.Session, error)
}

// errorStr converts an error to a string. It's just like e.Error() except that
// nil maps to "" instead of panicking.
func errorStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

// Execute runs a command and validates the output with its validation rules.
// The validation runs on the combined stdout and stderr of the command.
func ExecuteCommand(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if len(cmd.SetupCommands) > 0 {
		return runInteractive(ctx, client, cmd)
	}
	r, err := runMain(ctx, client, cmd)
	if err != nil {
		return nil, err
	}
	return r, r.FormattedError()
}

// sessionEndCommand closes the device's CLI so Session.Wait returns.
const sessionEndCommand = "exit"

// A prompt or the echoed sessionEndCommand ends a command's output. The prompt
// is not anchored to end-of-line: devices print the next command after it.
var (
	promptRE     = regexp.MustCompile(`(?m)^[\w.@/-]+\s*[#>] ?`)
	sessionEndRE = regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(sessionEndCommand) + `\s*$`)
)

// runInteractive runs the setup commands and the main command in one shell
// session, for devices that scope command side effects to the session.
func runInteractive(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	// Paging devices will not read commands without a terminal.
	if err := session.RequestPty("vt100", 1000, 200, ssh.TerminalModes{}); err != nil {
		return nil, fmt.Errorf("could not request pty: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("could not open stdin: %w", err)
	}

	var transcript syncBuffer
	session.Stdout = &transcript
	session.Stderr = &transcript

	if err := session.Shell(); err != nil {
		return nil, fmt.Errorf("could not start shell: %w", err)
	}

	lines := make([]string, 0, len(cmd.SetupCommands)+2)
	lines = append(lines, cmd.SetupCommands...)
	lines = append(lines, cmd.Command, sessionEndCommand)
	for _, line := range lines {
		if _, err := fmt.Fprintf(stdin, "%s\n", line); err != nil {
			return nil, fmt.Errorf("could not send %q: %w", line, err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case waitErr := <-done:
		r := &types.CommandResult{
			CommandStr: cmd.Command,
			Output:     commandOutput(transcript.String(), cmd.Command),
		}
		// The status belongs to the shell after "exit", not to the command.
		if r.Output == "" {
			r.Error = errorStr(waitErr)
		}
		cmd.Validator.ValidateResult(r)
		return r, r.FormattedError()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// commandOutput extracts one command's output from an interactive transcript
// that also holds the login banner, command echoes, and prompts.
func commandOutput(transcript, command string) string {
	t := strings.ReplaceAll(transcript, "\r\n", "\n")
	t = strings.ReplaceAll(t, "\r", "\n")

	// The first occurrence of the command is the device's echo of it.
	if i := strings.Index(t, command); i >= 0 {
		t = t[i+len(command):]
		if nl := strings.IndexByte(t, '\n'); nl >= 0 {
			t = t[nl+1:]
		}
	}
	end := len(t)
	for _, re := range []*regexp.Regexp{promptRE, sessionEndRE} {
		if loc := re.FindStringIndex(t); loc != nil && loc[0] < end {
			end = loc[0]
		}
	}
	return strings.Trim(t[:end], " \n\t")
}

// syncBuffer guards the transcript, which ssh writes from its own goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runMain runs the command, captures its output, and validates it.
func runMain(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	ch := make(chan *types.CommandResult, 1)
	go func() {
		output, err := session.CombinedOutput(cmd.Command)
		ch <- &types.CommandResult{
			CommandStr: cmd.Command,
			Output:     string(output),
			Error:      errorStr(err),
		}
	}()
	select {
	case r := <-ch:
		cmd.Validator.ValidateResult(r)
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// We found experimentally that some systems silently fail with unexpected
// filenames; since we provide the filenames ourselves in our profiles, we can
// ensure that we limit them to reasonable characters.
var filenameRE = regexp.MustCompile("^[a-zA-Z0-9_:./-]*$")

// ExecuteSCP executes an SCP command, sending the given data over SSH.
func ExecuteSCP(ctx context.Context, client sshClient, cmd *profile.SCPCommand, data string) (*types.CommandResult, error) {
	if !filenameRE.MatchString(cmd.Filepath) {
		return nil, fmt.Errorf("bad filename for scp: %q", cmd.Filepath)
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	cmdStr := fmt.Sprintf("%s -t %s", cmd.RemoteCommand, cmd.Filepath)
	ch := make(chan *types.CommandResult)
	go func() {
		response, err := executeSCP(session, cmdStr, filepath.Base(cmd.Filepath), data)
		ch <- &types.CommandResult{
			CommandStr: cmdStr,
			Output:     response,
			Error:      errorStr(err),
		}
	}()
	var r *types.CommandResult
	select {
	case r = <-ch:
		// got a result, continue
	case <-ctx.Done():
		return nil, fmt.Errorf("scp command %q failed: %w", cmdStr, ctx.Err())
	}
	cmd.Validator.ValidateResult(r)
	return r, r.FormattedError()
}
