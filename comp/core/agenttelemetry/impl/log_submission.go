// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import "context"

// SubmitLog accepts a value copy without waiting for the telemetry intake.
func (a *atel) SubmitLog(log Log) bool {
	a.logsMu.RLock()
	defer a.logsMu.RUnlock()
	if !a.logsAccepting {
		return false
	}
	select {
	case a.logsCh <- log:
		return true
	default:
		return false
	}
}

func (a *atel) startLogSubmission() {
	if a.logsCh == nil {
		return
	}
	a.logsDone = make(chan struct{})
	a.logsMu.Lock()
	a.logsAccepting = true
	a.logsMu.Unlock()
	go a.runLogSubmission()
}

func (a *atel) runLogSubmission() {
	defer close(a.logsDone)
	for a.cancelCtx.Err() == nil {
		select {
		case <-a.cancelCtx.Done():
			return
		case log := <-a.logsCh:
			// Batch records already queued without waiting for more. Snapshot
			// the depth so continuous producers cannot extend a batch forever.
			logs := append([]Log{log}, a.drainSubmittedLogs()...)
			a.sendSubmittedLogs(a.cancelCtx, logs)
		}
	}
}

func (a *atel) drainSubmittedLogs() []Log {
	n := len(a.logsCh)
	logs := make([]Log, 0, n)
	for range n {
		logs = append(logs, <-a.logsCh)
	}
	return logs
}

// Called only after the worker has exited and submissions have been disabled.
func (a *atel) flushSubmittedLogs(ctx context.Context) {
	a.sendSubmittedLogs(ctx, a.drainSubmittedLogs())
}

func (a *atel) sendSubmittedLogs(ctx context.Context, logs []Log) {
	if len(logs) == 0 {
		return
	}
	if err := a.sender.sendLogsBatch(ctx, logs); err != nil {
		// Never log at Error: that would feed the errortracking pipeline.
		a.logComp.Debugf("Agent telemetry log submission failed (%d logs): %v", len(logs), err)
	}
}
