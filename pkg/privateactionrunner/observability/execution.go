// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"context"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

// ExecutionOutcome describes the result and failure stage of an execution.
type ExecutionOutcome struct {
	Failed       bool
	FailureStage string
}

// ExecutionDetails collects action metadata for a single execution's completion record.
type ExecutionDetails struct {
	mu           sync.Mutex
	failureStage string
	fields       []log.Field
	provided     bool
}

type executionDetailsKey struct{}

// WithExecutionDetails creates a collector for one action execution.
func WithExecutionDetails(ctx context.Context) (context.Context, *ExecutionDetails) {
	details := &ExecutionDetails{}
	return context.WithValue(ctx, executionDetailsKey{}, details), details
}

// SetExecutionDetails stores an action's failure stage and completion fields.
func SetExecutionDetails(ctx context.Context, failureStage string, fields ...log.Field) {
	details, _ := ctx.Value(executionDetailsKey{}).(*ExecutionDetails)
	if details == nil {
		return
	}
	details.mu.Lock()
	defer details.mu.Unlock()
	details.failureStage = failureStage
	details.fields = append([]log.Field(nil), fields...)
	details.provided = true
}

func (d *ExecutionDetails) snapshot() (string, []log.Field, bool) {
	if d == nil {
		return "", nil, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failureStage, append([]log.Field(nil), d.fields...), d.provided
}

// ReportExecutionOutcome logs the execution result, duration, and action-specific fields.
func ReportExecutionOutcome(logger log.Logger, start time.Time, outcome ExecutionOutcome, additionalFields ...log.Field) {
	fields := make([]log.Field, 0, len(additionalFields)+4)
	fields = append(fields, additionalFields...)
	fields = append(fields,
		log.String("event", "execution.outcome"),
		log.Duration(Duration, time.Since(start)),
	)
	if outcome.Failed {
		fields = append(fields, log.String("outcome", "failure"))
		if outcome.FailureStage != "" {
			fields = append(fields, log.String("failure_stage", outcome.FailureStage))
		}
		logger.Warn("Private actions completed with failure", fields...)
		return
	}
	fields = append(fields, log.String("outcome", "success"))
	logger.Info("Private actions completed with success", fields...)
}
