// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

type result struct {
	message string
	err     error
}

// sshClient is a common interface between ssh.Client and RetryingSSHClient
type sshClient interface {
	NewSession() (*ssh.Session, error)
}

// Execute runs a command and validates the output with its validation rules.
// The validation runs on the combined stdout and stderr of the command.
func ExecuteCommand(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (string, error) {
	if len(cmd.SetupCommands) > 0 {
		return executeShellCommand(ctx, client, cmd)
	}

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	ch := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(cmd.Command)
		ch <- result{string(output), err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			if r.message != "" {
				return "", fmt.Errorf("%w: %q", r.err, r.message)
			}
			return "", r.err
		}
		return r.message, cmd.Validator.Validate(r.message)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// executeShellCommand runs cmd.SetupCommands and then cmd.Command inside a single
// interactive shell session, so a setting applied by a setup command (such as
// disabling the pager) is still in effect when cmd.Command runs.
func executeShellCommand(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	// Many network devices only offer an interactive shell over a pseudo-terminal.
	if err := session.RequestPty("xterm", 80, 32768, ssh.TerminalModes{ssh.ECHO: 0}); err != nil {
		return "", fmt.Errorf("failed to request pty: %w", err)
	}
	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("failed to start shell: %w", err)
	}

	var commands []string
	commands = append(commands, cmd.SetupCommands...)
	commands = append(commands, cmd.Command)
	for _, line := range commands {
		if _, err := fmt.Fprintf(stdinPipe, "%s\n", line); err != nil {
			return "", fmt.Errorf("failed to write command %q to stdin: %w", line, err)
		}
	}

	if err := stdinPipe.Close(); err != nil {
		return "", err
	}

	ch := make(chan result, 1)
	go func() {
		output, err := io.ReadAll(stdoutPipe)
		ch <- result{string(output), err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		output := cleanShellOutput(r.message, commands)
		return output, cmd.Validator.Validate(output)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

var (
	promptRE       = regexp.MustCompile(`^\S+[#>]\s*$`)
	promptPrefixRE = regexp.MustCompile(`^\S+[#>]\s+`)
)

// cleanShellOutput strips device prompts and command echoes from a shell
// transcript, leaving only the real command output.
func cleanShellOutput(transcript string, sent []string) string {
	echoed := make(map[string]struct{}, len(sent))
	for _, c := range sent {
		echoed[c] = struct{}{}
	}
	var kept []string
	for _, line := range strings.Split(transcript, "\n") {
		trimmed := strings.TrimSpace(line)
		if promptRE.MatchString(trimmed) {
			continue
		}
		echo := trimmed
		if loc := promptPrefixRE.FindStringIndex(echo); loc != nil {
			echo = strings.TrimSpace(echo[loc[1]:])
		}
		if _, isEcho := echoed[echo]; isEcho {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")) + "\n"
}

// We found experimentally that some systems silently fail with unexpected
// filenames; since we provide the filenames ourselves in our profiles, we can
// ensure that we limit them to reasonable characters.
var filenameRE = regexp.MustCompile("^[a-zA-Z0-9_:./-]*$")

// ExecuteSCP executes an SCP command, sending the given data over SSH.
func ExecuteSCP(ctx context.Context, client sshClient, cmd *profile.SCPCommand, data string) (string, error) {
	if !filenameRE.MatchString(cmd.Filepath) {
		return "", fmt.Errorf("bad filename for scp: %q", cmd.Filepath)
	}
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	cmdStr := fmt.Sprintf("%s -t %s", cmd.RemoteCommand, cmd.Filepath)
	ch := make(chan result)
	go func() {
		response, err := executeSCP(session, cmdStr, filepath.Base(cmd.Filepath), data)
		ch <- result{response, err}
	}()
	var response string
	select {
	case result := <-ch:
		response = result.message
		err = result.err
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("scp command %q failed: %w", cmdStr, err)
	}
	if err := cmd.Validator.Validate(response); err != nil {
		return response, fmt.Errorf("scp command %q bad output: %w", cmdStr, err)
	}
	return response, nil
}
