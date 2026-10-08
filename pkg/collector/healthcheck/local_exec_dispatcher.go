// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const localStepTimeout = 30 * time.Second

// localWaitDelay bounds waiting for children holding output pipes after the step times out.
const localWaitDelay = 5 * time.Second

const localOutputLimit = 4096

type localOutputBuffer struct {
	data [localOutputLimit]byte
	size int
}

// Write retains a bounded prefix and discards excess output without interrupting the command.
func (b *localOutputBuffer) Write(p []byte) (int, error) {
	b.size += copy(b.data[b.size:], p)
	return len(p), nil
}

// scrubForLog redacts secrets and bounds size before step output reaches the Agent log.
func scrubForLog(output string) string {
	// A capped prefix may omit closing markers that secret scrubbers need to recognize a secret.
	if len(output) >= localOutputLimit {
		return "[redacted: output reached capture limit]"
	}
	scrubbed, err := scrubber.ScrubString(output)
	if err != nil {
		return "[redacted]"
	}
	scrubbed = strings.TrimSpace(scrubbed)
	const limit = 512
	if len(scrubbed) > limit {
		scrubbed = scrubbed[:limit] + "...[truncated]"
	}
	return scrubbed
}

// LocalExecDispatcher runs remediation steps in-process on the host. Demo-only: it bypasses the
// PAR rshell sandbox so a remediation's effect is directly visible on a bare host. Production uses
// RegistryDispatcher; this path is reached only when execution_mode is "local".
type LocalExecDispatcher struct {
	out      chan<- event.Event
	hostname string
}

// NewLocalExecDispatcher reuses the aggregator's event input for outcome reports.
func NewLocalExecDispatcher(out chan<- event.Event, hostname string) *LocalExecDispatcher {
	return &LocalExecDispatcher{out: out, hostname: hostname}
}

// Dispatch runs each step through the host shell, stops on the first failure, and reports the outcome.
func (d *LocalExecDispatcher) Dispatch(ctx context.Context, id checkid.ID, scName, failureMessage string, cfg *integration.HealthCheckConfig) {
	if d == nil || cfg == nil || ctx.Err() != nil {
		return
	}
	d.emit(ctx, id, scName, "detected", detectedText(scName, d.hostname, failureMessage, id, cfg))

	outcome := "remediated"
	var text strings.Builder
	fmt.Fprintf(&text, "Remediation results for service check %q (check %s):", scName, id)
	for i, step := range cfg.Remediation.Steps {
		exitCode, output, err := d.run(ctx, step.Command)
		result := "succeeded"
		if err != nil || exitCode != 0 {
			result = "failed"
			outcome = "escalate"
		}
		fmt.Fprintf(&text, "\n%d. %s -> %s (exit code %d)", i+1, scrubCommand(step.Command), result, exitCode)
		log.Infof("Health-check remediation (local): check %s step %d -> exit %d err=%v output=%q", id, i+1, exitCode, err, scrubForLog(output))
		if result == "failed" {
			break
		}
	}
	d.emit(ctx, id, scName, outcome, text.String())
}

func (d *LocalExecDispatcher) run(ctx context.Context, command string) (int, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, localStepTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(runCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	}
	configureLocalCommand(cmd)
	cmd.WaitDelay = localWaitDelay
	var output localOutputBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return exitCode, string(output.data[:output.size]), err
}

func (d *LocalExecDispatcher) emit(ctx context.Context, id checkid.ID, scName, outcome, text string) {
	if d.out == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	alertType := outcomeAlertType(outcome)
	e := event.Event{
		Title: "health-check remediation (" + outcome + ")", Text: text,
		Ts: time.Now().Unix(), Host: d.hostname,
		Priority: event.PriorityNormal, AlertType: alertType, SourceTypeName: remediationSource,
		AggregationKey: "health_check_remediation:" + string(id),
		Tags:           []string{"check_id:" + string(id), "service_check:" + scName, "remediation:" + outcome},
	}
	select {
	case d.out <- e:
	case <-ctx.Done():
	}
}
