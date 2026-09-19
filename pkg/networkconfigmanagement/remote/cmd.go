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

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	log "github.com/DataDog/datadog-agent/pkg/util/log"
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
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	ch := make(chan *types.CommandResult, 1)
	go func() {
		if cmd.Interactive {
			// Some CLIs (e.g. PAN-OS) only emit output on an interactive TTY; a
			// one-shot exec returns just the login banner.
			results, execErr := runInteractive(session, cmd)
			ch <- collapseInteractiveResults(results, cmd, execErr)
			return
		}
		command := cmd.Command
		if len(cmd.SetupCommands) > 0 {
			lines := append(append([]string{}, cmd.SetupCommands...), cmd.Command)
			command = strings.Join(lines, "\n")
		}
		out, execErr := session.CombinedOutput(command)
		ch <- &types.CommandResult{
			CommandStr: cmd.Command,
			Output:     string(out),
			Error:      errorStr(execErr),
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

// collapseInteractiveResults reduces the per-command results of an interactive
// session to the single CommandResult the config pipeline consumes: the main
// command's result (which carries the config output). Setup-command output is
// logged so a failure like `set cli pager off` printing "% Invalid input" is
// visible rather than silently swallowed. If the main command never ran (an
// earlier step failed), a synthesized result carries the captured transcript
// and the error, so the raw output is not lost.
func collapseInteractiveResults(results types.ResultList, cmd *profile.PlainCommand, execErr error) *types.CommandResult {
	var main *types.CommandResult
	for _, r := range results {
		if r.CommandStr == cmd.Command {
			main = r
			continue
		}
		// A setup command; surface any output/error it produced.
		if r.Output != "" || r.Error != "" {
			log.Warnf("NCM interactive setup command %q produced output=%q error=%q", r.CommandStr, r.Output, r.Error)
		}
	}
	if main == nil {
		main = &types.CommandResult{
			CommandStr: cmd.Command,
			Output:     interactiveTranscript(results),
		}
	}
	if execErr != nil && main.Error == "" {
		main.Error = errorStr(execErr)
	}
	return main
}

// interactiveTranscript renders the commands run so far as a readable block,
// used only on the error path (before the main command produced config) so the
// failure carries context.
func interactiveTranscript(results types.ResultList) string {
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "$ %s\n%s\n", r.CommandStr, r.Output)
	}
	return b.String()
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
