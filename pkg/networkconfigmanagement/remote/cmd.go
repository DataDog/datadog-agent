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
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/cli"
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

// ExecuteCommand runs a command and validates the output with its validation
// rules. The validation runs on the combined stdout and stderr of the command.
func ExecuteCommand(ctx context.Context, conn *SSHConnection, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if cmd.Interactive {
		return runInteractive(ctx, conn, cmd)
	}
	r, err := runMain(ctx, conn.client, cmd)
	if err != nil {
		return nil, err
	}
	return r, r.FormattedError()
}

// sessionEndWait bounds how long to wait for the device to hang up after an
// interactive session ends.
const sessionEndWait = 2 * time.Second

// runInteractive runs the command in a shell session using the CLI dialect
// for the connection's profile, for devices that scope command side effects
// (such as the pager) to the session.
func runInteractive(ctx context.Context, conn *SSHConnection, cmd *profile.PlainCommand) (*types.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if conn.prof == nil {
		return nil, fmt.Errorf("no device type provided for %q", conn.ipAddress)
	}
	dialect, err := cli.ForProfile(conn.prof.Name)
	if err != nil {
		return nil, err
	}
	// Paging devices will not read commands without a terminal; StartShell
	// requests one.
	sh, terr := conn.StartShell(ctx)
	if terr != nil {
		return nil, terr
	}
	defer sh.Close()

	session, err := cli.Open(ctx, sh, dialect)
	if err != nil {
		return nil, err
	}
	reply, runErr := session.Run(ctx, cmd.Command)
	session.End(ctx, sessionEndWait)

	r := &types.CommandResult{
		CommandStr: cmd.Command,
		Output:     strings.Trim(reply.Output, " \n\t"),
	}
	if runErr != nil {
		// this could be a context closure or a problem with the actual send/receive.
		r.Error = runErr.Error()
	} else if err := dialect.CheckError(r.Output); err != nil {
		// indicates that the dialect sees this output as an error
		r.Error = err.Error()
	} else {
		cmd.Validator.ValidateResult(r)
	}
	if reply.Prompt == "" {
		// Without the closing prompt there is no telling whether the output
		// is complete.
		r.Error = fmt.Sprintf("no %s prompt after %q, output may be incomplete: %v", dialect.Name, cmd.Command, runErr)
	}
	return r, r.FormattedError()
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
	// Buffered so the goroutine can finish after a timeout.
	ch := make(chan *types.CommandResult, 1)
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
