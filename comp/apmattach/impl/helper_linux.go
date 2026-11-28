// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	decisionAttached = "attached"
	decisionSkipped  = "skipped"
	decisionFailed   = "failed"
	decisionAllowed  = "allowed"
)

const (
	reasonHelperError   = "helper_error"
	reasonHelperTimeout = "helper_timeout"
)

const (
	helperGracePeriod = 5 * time.Second
	maxCapturedOutput = 64 * 1024
	maxDetailLen      = 512
)

// helperResult is the JSON object the attach helper prints on stdout.
type helperResult struct {
	PID           int    `json:"pid"`
	Language      string `json:"language"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	Detail        string `json:"detail"`
	Rule          string `json:"rule"`
	Layer         string `json:"layer"`
	Service       string `json:"service"`
	TracerVersion string `json:"tracer_version"`
	DurationMs    int64  `json:"duration_ms"`
}

type helperRunner interface {
	run(ctx context.Context, pid int) helperResult
}

type execHelper struct {
	path    string
	timeout time.Duration
	grace   time.Duration
}

func (h *execHelper) run(ctx context.Context, pid int) helperResult {
	ctx, cancel := context.WithTimeout(ctx, h.timeout+h.grace)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.path, "--pid", strconv.Itoa(pid), "--timeout", h.timeout.String())
	stdout := &cappedBuffer{limit: maxCapturedOutput}
	stderr := &cappedBuffer{limit: maxCapturedOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	res, parseErr := parseHelperOutput(stdout.Bytes())
	if parseErr == nil && res.PID != 0 && res.PID != pid {
		parseErr = fmt.Errorf("result is for pid %d", res.PID)
	}
	if parseErr == nil {
		if res.PID == 0 {
			res.PID = pid
		}
		if res.DurationMs == 0 {
			res.DurationMs = elapsed.Milliseconds()
		}
		return res
	}

	res = helperResult{
		PID:        pid,
		Decision:   decisionFailed,
		Reason:     reasonHelperError,
		DurationMs: elapsed.Milliseconds(),
	}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Reason = reasonHelperTimeout
		res.Detail = fmt.Sprintf("attach helper did not finish within %s", h.timeout+h.grace)
	case errors.As(runErr, &exitErr):
		res.Detail = fmt.Sprintf("attach helper exited with code %d and no valid output (%v)", exitErr.ExitCode(), parseErr)
	case runErr != nil:
		res.Detail = fmt.Sprintf("could not run attach helper: %v", runErr)
	default:
		res.Detail = fmt.Sprintf("attach helper exited with code 0 and no valid output (%v)", parseErr)
	}
	if errText := lastLines(stderr.String(), maxDetailLen); errText != "" {
		res.Detail += ": " + errText
	}
	return res
}

func parseHelperOutput(out []byte) (helperResult, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return helperResult{}, errors.New("empty stdout")
	}
	res, err := decodeHelperResult(out)
	if err == nil {
		return res, nil
	}
	lines := bytes.Split(out, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		if res, lineErr := decodeHelperResult(line); lineErr == nil {
			return res, nil
		}
	}
	return helperResult{}, err
}

func decodeHelperResult(data []byte) (helperResult, error) {
	var res helperResult
	if err := json.Unmarshal(data, &res); err != nil {
		return helperResult{}, fmt.Errorf("invalid JSON: %w", err)
	}
	switch res.Decision {
	case decisionAttached, decisionSkipped, decisionFailed, decisionAllowed:
	default:
		return helperResult{}, fmt.Errorf("unknown decision %q", res.Decision)
	}
	if res.Reason == "" {
		return helperResult{}, errors.New("missing reason")
	}
	return res, nil
}

func lastLines(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = "..." + s[len(s)-maxLen:]
	}
	return strings.Join(strings.Fields(s), " ")
}

type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *cappedBuffer) String() string { return b.buf.String() }
