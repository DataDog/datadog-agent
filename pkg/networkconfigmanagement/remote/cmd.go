// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// CommandResult records a command that was run and the resulting output.
type CommandResult struct {
	CommandStr string `json:"command_str"`
	Output     string `json:"output"`
	Error      error  `json:"error"`
}

// FormattedError returns nil if there was no error, and otherwise wraps
// .AnyError to append any captured output to the error message.
func (c *CommandResult) FormattedError() error {
	if c.Error == nil {
		return nil
	}
	if c.Output != "" {
		return fmt.Errorf("%w: %q", c.Error, c.Output)
	}
	return c.Error
}

type ResultList []*CommandResult

func (rl ResultList) AnyError() error {
	for _, result := range rl {
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// sshClient is a common interface between ssh.Client and RetryingSSHClient
type sshClient interface {
	NewSession() (*ssh.Session, error)
}

// Execute runs a command and validates the output with its validation rules.
// The validation runs on the combined stdout and stderr of the command.
func ExecuteCommand(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (*CommandResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	ch := make(chan *CommandResult, 1)
	go func() {
		output, err := session.CombinedOutput(cmd.Command)
		ch <- &CommandResult{
			CommandStr: cmd.Command,
			Output:     string(output),
			Error:      err,
		}
	}()
	select {
	case r := <-ch:
		if r.Error == nil {
			r.Error = cmd.Validator.Validate(r.Output)
		}
		err := r.FormattedError()
		if err != nil {
			if r.Output != "" {
				return r, fmt.Errorf("%w: %q", err, r.Output)
			}
			return r, err
		}
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
func ExecuteSCP(ctx context.Context, client sshClient, cmd *profile.SCPCommand, data string) (*CommandResult, error) {
	if !filenameRE.MatchString(cmd.Filepath) {
		return nil, fmt.Errorf("bad filename for scp: %q", cmd.Filepath)
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	cmdStr := fmt.Sprintf("%s -t %s", cmd.RemoteCommand, cmd.Filepath)
	ch := make(chan *CommandResult)
	go func() {
		response, err := executeSCP(session, cmdStr, filepath.Base(cmd.Filepath), data)
		ch <- &CommandResult{
			CommandStr: cmdStr,
			Output:     response,
			Error:      err,
		}
	}()
	var r *CommandResult
	select {
	case r = <-ch:
		// got a result, continue
	case <-ctx.Done():
		return nil, fmt.Errorf("scp command %q failed: %w", cmdStr, ctx.Err())
	}

	if r.Error == nil {
		r.Error = cmd.Validator.Validate(r.Output)
	}
	err = r.FormattedError()
	if err != nil {
		if r.Output != "" {
			return r, fmt.Errorf("%w: %q", err, r.Output)
		}
		return r, err
	}

	return r, nil
}
