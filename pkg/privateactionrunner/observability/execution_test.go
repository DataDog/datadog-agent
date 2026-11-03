// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

type executionLogEntry struct {
	level   string
	message string
	fields  map[string]interface{}
}

type executionLogRecorder struct {
	log.Logger
	entries *[]executionLogEntry
	fields  []log.Field
}

func (l executionLogRecorder) With(fields ...log.Field) log.Logger {
	l.fields = append(append([]log.Field(nil), l.fields...), fields...)
	return l
}
func (l executionLogRecorder) record(level, message string, fields []log.Field) {
	entry := executionLogEntry{level: level, message: message, fields: map[string]interface{}{}}
	for _, field := range append(append([]log.Field(nil), l.fields...), fields...) {
		entry.fields[field.Key] = field.Value
	}
	*l.entries = append(*l.entries, entry)
}
func (l executionLogRecorder) Info(message string, fields ...log.Field) {
	l.record("info", message, fields)
}
func (l executionLogRecorder) Warn(message string, fields ...log.Field) {
	l.record("warn", message, fields)
}

func TestExecutionDetails(t *testing.T) {
	ctx, first := WithExecutionDetails(context.Background())
	_, second := WithExecutionDetails(context.Background())
	fields := []log.Field{log.String("version", "1.2.3")}
	SetExecutionDetails(context.WithoutCancel(ctx), "execution", fields...)
	fields[0] = log.String("version", "mutated")
	stage, got, provided := first.snapshot()
	require.True(t, provided)
	require.Equal(t, "execution", stage)
	require.Equal(t, []log.Field{log.String("version", "1.2.3")}, got)
	got[0] = log.String("version", "mutated snapshot")
	_, got, _ = first.snapshot()
	require.Equal(t, "1.2.3", got[0].Value)
	_, _, provided = second.snapshot()
	require.False(t, provided, "metadata must not leak to another execution")
	SetExecutionDetails(context.Background(), "execution", fields...) // Direct action calls have no collector.
}

func TestReportExecutionOutcome(t *testing.T) {
	for _, tc := range []struct {
		name          string
		outcome       ExecutionOutcome
		level, result string
	}{
		{"success", ExecutionOutcome{}, "info", "success"},
		{"failure", ExecutionOutcome{Failed: true, FailureStage: "execution"}, "warn", "failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var entries []executionLogEntry
			logger := executionLogRecorder{entries: &entries}
			ReportExecutionOutcome(logger, time.Now(), tc.outcome, log.String("version", "1.2.3"))
			require.Len(t, entries, 1)
			entry := entries[0]
			require.Equal(t, tc.level, entry.level)
			require.Equal(t, tc.result, entry.fields["outcome"])
			require.Equal(t, "execution.outcome", entry.fields["event"])
			require.Equal(t, "1.2.3", entry.fields["version"])
			if tc.outcome.Failed {
				require.Equal(t, "execution", entry.fields["failure_stage"])
			} else {
				require.NotContains(t, entry.fields, "failure_stage")
			}
		})
	}
}
