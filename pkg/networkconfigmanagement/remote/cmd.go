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
	"sync"
	"time"

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

const (
	sessionEndCommand = "exit"
	// idleWait is the fallback for a prompt promptTailRE does not match.
	idleWait       = 3 * time.Second
	sessionEndWait = 2 * time.Second
)

// promptTailRE matches a prompt at the end of the output read so far.
var promptTailRE = regexp.MustCompile(`(?m)^[\w.@/-]+\s*[#>] ?\z`)

// runInteractive runs the setup commands and the main command in one shell
// session, for devices that scope command side effects to the session. Lines
// are sent one at a time, waiting for the prompt in between, so no other
// line's echo can land inside the main command's output.
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
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("could not open stdout: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("could not open stderr: %w", err)
	}
	reader := newDeviceReader(stdout, stderr)
	defer reader.Close()

	if err := session.Shell(); err != nil {
		return nil, fmt.Errorf("could not start shell: %w", err)
	}

	// Discard the login banner.
	if _, err := reader.readToPrompt(ctx); err != nil {
		return nil, err
	}
	for _, setup := range cmd.SetupCommands {
		if err := sendLine(stdin, setup); err != nil {
			return nil, err
		}
		if _, err := reader.readToPrompt(ctx); err != nil {
			return nil, err
		}
	}
	if err := sendLine(stdin, cmd.Command); err != nil {
		return nil, err
	}
	captured, err := reader.readToPrompt(ctx)
	if err != nil {
		return nil, err
	}

	// Only now can the CLI be closed without its echo reaching the capture.
	_ = sendLine(stdin, sessionEndCommand)
	_ = stdin.Close()
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case <-done:
	case <-time.After(sessionEndWait):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	r := &types.CommandResult{
		CommandStr: cmd.Command,
		Output:     trimEchoAndPrompt(captured, cmd.Command),
	}
	if r.Output == "" {
		r.Error = fmt.Sprintf("captured no output for %q", cmd.Command)
	}
	cmd.Validator.ValidateResult(r)
	return r, r.FormattedError()
}

func sendLine(w io.Writer, line string) error {
	if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
		return fmt.Errorf("could not send %q: %w", line, err)
	}
	return nil
}

// trimEchoAndPrompt removes the command's echo from the front of a capture and
// the trailing prompt from the end.
func trimEchoAndPrompt(captured, command string) string {
	if i := strings.Index(captured, command); i >= 0 {
		captured = captured[i+len(command):]
		if nl := strings.IndexByte(captured, '\n'); nl >= 0 {
			captured = captured[nl+1:]
		}
	}
	if loc := promptTailRE.FindStringIndex(captured); loc != nil {
		captured = captured[:loc[0]]
	}
	return strings.Trim(captured, " \n\t")
}

// deviceReader turns blocking streams into chunks that can be waited on with a
// timeout, so a device that stops talking cannot hang a collection.
type deviceReader struct {
	chunks chan []byte
	done   chan struct{}
	once   sync.Once
}

func newDeviceReader(streams ...io.Reader) *deviceReader {
	d := &deviceReader{
		chunks: make(chan []byte, 16),
		done:   make(chan struct{}),
	}
	var wg sync.WaitGroup
	for _, s := range streams {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := r.Read(buf)
				if n > 0 {
					select {
					case d.chunks <- append([]byte(nil), buf[:n]...):
					case <-d.done:
						return
					}
				}
				if err != nil {
					return
				}
			}
		}(s)
	}
	go func() { wg.Wait(); close(d.chunks) }()
	return d
}

// Close releases the reader goroutines. Safe to call more than once.
func (d *deviceReader) Close() {
	d.once.Do(func() { close(d.done) })
}

// readToPrompt accumulates output until a prompt appears at the tail, the
// streams end, or the device goes idle.
func (d *deviceReader) readToPrompt(ctx context.Context) (string, error) {
	var sb strings.Builder
	idle := time.NewTimer(idleWait)
	defer idle.Stop()
	for {
		select {
		case chunk, ok := <-d.chunks:
			if !ok {
				return sb.String(), nil
			}
			// A pty writes CRLF; the (?m)^ anchors here and in the profiles'
			// redaction rules only work on LF.
			text := strings.ReplaceAll(string(chunk), "\r\n", "\n")
			sb.WriteString(strings.ReplaceAll(text, "\r", "\n"))
			if promptTailRE.MatchString(sb.String()) {
				return sb.String(), nil
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleWait)
		case <-idle.C:
			return sb.String(), nil
		case <-ctx.Done():
			return sb.String(), ctx.Err()
		}
	}
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
