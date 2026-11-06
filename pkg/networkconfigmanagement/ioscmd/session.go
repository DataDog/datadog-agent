// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ioscmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// Shell is an interactive shell on a device. It is satisfied by remote.Shell.
type Shell interface {
	Send(ctx context.Context, data string) error
	ReadUntil(ctx context.Context, match func(buf []byte) int) ([]byte, error)
}

var (
	// promptRE matches an IOS prompt such as `R1#`, `R1>` or `R1(config-if)#`.
	promptRE = regexp.MustCompile(`^[A-Za-z0-9._-]+(\([A-Za-z0-9._-]+\))?[>#] ?$`)
	// questionRE matches a question IOS asks before running a command, such
	// as `Destination filename [startup-config]?`.
	questionRE = regexp.MustCompile(`(\?|\[confirm\]) ?$`)
	// errorRE matches the line IOS prints when it rejects or fails to run a
	// command. Other lines starting with "%" are informational and are not
	// matched, e.g. "% Warning: ...", "% Note: ..." or syslog messages such as
	// "%SYS-5-CONFIG_I: ...".
	// Note that cisco does not publish a list of all possible errors and
	// warnings; this list should cover the vast majority of cases with very few
	// false positives, but it is difficult to be certain and new commands could
	// introduce new error messages in future versions of IOS.
	errorRE = regexp.MustCompile(`(?m)^(?:` + strings.Join([]string{
		// Parser rejections.
		`% Invalid `,           // % Invalid input detected at '^' marker. / % Invalid IP address / % Invalid interface type and number
		`% Incomplete command`, // % Incomplete command.
		`% Ambiguous command`,  // % Ambiguous command:  "sh i"
		`% Unrecognized command`,
		`% Unknown `, // % Unknown command or computer name, or unable to find computer address / % Unknown VRF specified
		`% Bad `,     // % Bad IP address or host name / % Bad mask
		// Generic failures, e.g. from copy, delete or file operations.
		`%Error`,  // %Error opening flash:foo (No such file or directory)
		`% Error`, // % Error in authentication.
		`% Cannot `,
		`% Can't `,
		`% Unable to `,
		`% Failed `,
		`% No such `,
		`% Not enough space`,
		`% Insufficient `,
		`% \S+ overlaps with `, // % 10.0.0.0 overlaps with GigabitEthernet0/1
		// Authorization failures.
		`% Command authorization failed`,
		`% Authorization failed`,
		`% Access denied`,
		`% Permission denied`,
	}, "|") + `).*$`)
)

// setupCommands are sent before the requested lines so that output is not
// paginated and echoed commands are not wrapped.
var setupCommands = []string{"terminal length 0", "terminal width 511"}

// Run sends lines to an IOS shell one at a time and returns a result for each
// line that ran. It stops at the first line that IOS rejects or that fails to
// complete, leaving config mode if necessary, and returns the results so far
// along with an error.
//
// An empty line answers the question asked by the line before it (e.g. the
// "Destination filename?" prompt from `copy`); its output is added to that
// line's result rather than getting a result of its own.
func Run(ctx context.Context, sh Shell, lines []string) ([]*types.CommandResult, error) {
	s := &session{sh: sh}
	raw, err := sh.ReadUntil(ctx, lastLineMatches(promptRE))
	if err != nil {
		return nil, fmt.Errorf("waiting for prompt: %w", err)
	}
	_, prompt := clean("", raw)
	if strings.HasSuffix(strings.TrimSpace(prompt), ">") {
		return nil, errors.New("device is not in privileged EXEC mode")
	}
	s.observePrompt(prompt)

	for _, setup := range setupCommands {
		out, err := s.exchange(ctx, setup, false)
		if err == nil {
			err = iosError(out)
		}
		if err != nil {
			return nil, fmt.Errorf("running %q: %w", setup, err)
		}
	}

	var results []*types.CommandResult
	for i, line := range lines {
		expectQuestion := i+1 < len(lines) && lines[i+1] == ""
		out, err := s.exchange(ctx, line, expectQuestion)
		var result *types.CommandResult
		if line == "" && len(results) > 0 {
			result = results[len(results)-1]
			if result.Output != "" && !strings.HasSuffix(result.Output, "\n") {
				result.Output += "\n"
			}
			result.Output += out
		} else {
			result = &types.CommandResult{CommandStr: line, Output: out}
			results = append(results, result)
		}
		if err == nil {
			err = iosError(out)
			if err != nil {
				s.leaveConfig(ctx)
			}
		}
		if err != nil {
			result.Error = err.Error()
			return results, fmt.Errorf("running %q: %w", result.CommandStr, err)
		}
	}
	return results, nil
}

// session tracks the state of an IOS shell between commands.
type session struct {
	sh       Shell
	inConfig bool // whether the last prompt was in a config mode
}

// exchange sends a line and returns its output, without the echoed line or
// the trailing prompt. If expectQuestion is set, it also stops at a question
// prompt, which is left in the output.
func (s *session) exchange(ctx context.Context, line string, expectQuestion bool) (string, error) {
	if err := s.sh.Send(ctx, line+"\n"); err != nil {
		return "", err
	}
	match := lastLineMatches(promptRE)
	if expectQuestion {
		match = lastLineMatches(promptRE, questionRE)
	}
	raw, err := s.sh.ReadUntil(ctx, match)
	out, prompt := clean(line, raw)
	s.observePrompt(prompt)
	return out, err
}

func (s *session) observePrompt(prompt string) {
	if prompt != "" {
		s.inConfig = strings.Contains(prompt, "(config")
	}
}

// leaveConfig returns to privileged EXEC mode, if needed, after a failure.
// It is best effort; errors are ignored.
func (s *session) leaveConfig(ctx context.Context) {
	if s.inConfig {
		_, _ = s.exchange(ctx, "end", false)
	}
}

// lastLineMatches returns a matcher for Shell.ReadUntil that matches the
// whole buffer once its last line matches any of res.
func lastLineMatches(res ...*regexp.Regexp) func([]byte) int {
	return func(buf []byte) int {
		last := buf[bytes.LastIndexByte(buf, '\n')+1:]
		last = bytes.ReplaceAll(last, []byte("\r"), nil)
		for _, re := range res {
			if re.Match(last) {
				return len(buf)
			}
		}
		return -1
	}
}

// clean normalizes line endings in raw, strips the echoed command from the
// start, and strips the trailing prompt (if there is one), which it returns
// separately.
func clean(command string, raw []byte) (output, prompt string) {
	text := strings.ReplaceAll(string(raw), "\r", "")
	if first, rest, ok := strings.Cut(text, "\n"); ok && first == command {
		text = rest
	}
	i := strings.LastIndexByte(text, '\n')
	if last := text[i+1:]; promptRE.MatchString(last) {
		return text[:i+1], last
	}
	return text, ""
}

// iosError returns an error if output contains an IOS error message.
func iosError(output string) error {
	if msg := errorRE.FindString(output); msg != "" {
		return errors.New(msg)
	}
	return nil
}
