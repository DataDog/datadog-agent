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
)

const localStepTimeout = 30 * time.Second

// LocalExecDispatcher runs remediation steps in-process on the host. Demo-only: it bypasses the
// PAR rshell sandbox so a remediation's effect is directly visible on a bare host. Production uses
// PARDispatcher; this path is reached only when execution_mode is "local".
type LocalExecDispatcher struct {
	out      chan<- event.Event
	hostname string
}

// NewLocalExecDispatcher reuses the aggregator's event input for outcome reports.
func NewLocalExecDispatcher(out chan<- event.Event, hostname string) *LocalExecDispatcher {
	return &LocalExecDispatcher{out: out, hostname: hostname}
}

// Dispatch runs each step through the host shell, stops on the first failure, and reports the outcome.
func (d *LocalExecDispatcher) Dispatch(ctx context.Context, id checkid.ID, scName string, cfg *integration.HealthCheckConfig) {
	if d == nil || cfg == nil || ctx.Err() != nil {
		return
	}
	d.emit(ctx, id, scName, "detected", "Health check became CRITICAL; local remediation was dispatched.")
	outcome := "remediated"
	var text strings.Builder
	text.WriteString("Local remediation step results:")
	for i, step := range cfg.Remediation.Steps {
		exitCode, output, err := d.run(ctx, step.Command)
		result := "succeeded"
		if err != nil || exitCode != 0 {
			result = "failed"
			outcome = "escalate"
		}
		fmt.Fprintf(&text, "\n%d. %s (exit code %d)", i+1, result, exitCode)
		log.Infof("Health-check remediation (local): check %s step %d %q -> exit %d output=%q err=%v", id, i+1, step.Command, exitCode, strings.TrimSpace(output), err)
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
	output, err := cmd.CombinedOutput()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return exitCode, string(output), err
}

func (d *LocalExecDispatcher) emit(ctx context.Context, id checkid.ID, scName, outcome, text string) {
	if d.out == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	alertType := event.AlertTypeInfo
	if outcome == "escalate" {
		alertType = event.AlertTypeError
	}
	e := event.Event{
		Title: "health-check remediation (" + outcome + ")", Text: text,
		Ts: time.Now().Unix(), Host: d.hostname,
		Priority: event.PriorityNormal, AlertType: alertType, SourceTypeName: "datadog-agent",
		AggregationKey: "health_check_remediation:" + string(id),
		Tags:           []string{"check_id:" + string(id), "service_check:" + scName, "remediation:" + outcome},
	}
	select {
	case d.out <- e:
	case <-ctx.Done():
	}
}
