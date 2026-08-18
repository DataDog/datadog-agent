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
		return executeWithSetup(ctx, client, cmd)
	}
	r, err := runMain(ctx, client, cmd)
	if err != nil {
		return nil, err
	}
	return r, r.FormattedError()
}

// executeWithSetup runs the setup commands and the main command on a single
// connection. Setup commands (e.g. "terminal pager 0") run first, each in its
// own exec session because many devices execute only the first command of a
// multi-command exec.
func executeWithSetup(ctx context.Context, client sshClient, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	var result *types.CommandResult
	run := func(conn sshClient) error {
		// Setup commands are best-effort, but honor cancellation.
		for _, setup := range cmd.SetupCommands {
			if err := ctx.Err(); err != nil {
				return err
			}
			_ = runExec(ctx, conn, setup)
		}
		r, err := runMain(ctx, conn, cmd)
		result = r
		return err
	}
	if rc, ok := client.(*RetryingSSHClient); ok {
		if err := rc.Pinned(run); err != nil {
			return nil, err
		}
	} else if err := run(client); err != nil {
		return nil, err
	}
	return result, result.FormattedError()
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

// runExec runs a command in its own session and discards its output, used for
// setup commands whose only purpose is a side effect on the connection.
func runExec(ctx context.Context, client sshClient, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	ch := make(chan error, 1)
	go func() {
		_, err := session.CombinedOutput(command)
		ch <- err
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
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
