// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportHealthCheck(t *testing.T) {
	recorder := &recordingTaggedStatsdClient{}

	ReportHealthCheck(recorder)

	require.Len(t, recorder.calls, 2)
	assert.Equal(t, RunnerRunningMetric, recorder.calls[0].name)
	assert.Equal(t, HealthCheckMetric, recorder.calls[1].name)
}

func TestReportHealthCheck_NilMetricsClientDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		ReportHealthCheck(nil)
	})
}

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

func TestReportExecutionCompletedEnrichment(t *testing.T) {
	for _, enriched := range []bool{false, true} {
		for _, failure := range []error{nil, errors.New("secret-stderr")} {
			name := fmt.Sprintf("enriched=%t/failure=%t", enriched, failure != nil)
			t.Run(name, func(t *testing.T) {
				var entries []executionLogEntry
				logger := executionLogRecorder{entries: &entries}
				metrics := &recordingTaggedStatsdClient{}
				ctx, details := WithExecutionDetails(context.Background())
				if enriched {
					SetExecutionDetails(ctx, "execution", log.String("package_version", "1.2.3"))
				}
				client := actionsclientpb.Client(0)
				start := ReportExecutionStart(metrics, client, "test.action", "task-id", logger)
				ReportExecutionCompleted(metrics, client, "test.action", "task-id", start, failure, logger, details)
				require.Len(t, entries, 2)
				final := entries[1]
				result, level, message := "success", "info", "Private actions completed with success"
				if failure != nil {
					result, level, message = "error", "warn", "Private actions completed with failure"
				}
				require.Equal(t, level, final.level)
				require.Equal(t, message, final.message)
				if enriched {
					require.Equal(t, "1.2.3", final.fields["package_version"])
					require.NotContains(t, final.fields, "error")
					require.Equal(t, "execution.outcome", final.fields["event"])
					if failure != nil {
						require.Equal(t, "execution", final.fields["failure_stage"])
					}
				} else {
					want := map[string]interface{}{ActionClientTagName: client.String()}
					if failure != nil {
						want["error"] = failure
					}
					require.Equal(t, want, final.fields)
				}
				startTags := []string{ActionClientTagName + ":" + client.String(), ActionFqnTagName + ":test.action"}
				completedTags := append(append([]string(nil), startTags...), ExecutionResultTagName+":"+result)
				latencyTags := append([]string(nil), completedTags...)
				if enriched && failure != nil {
					completedTags = append(completedTags, "failure_stage:execution")
				}
				require.Equal(t, []taggedMetricCall{
					{name: ActionExecutionStartedMetric, tags: startTags},
					{name: ActionExecutionLatencyMetric, tags: latencyTags},
					{name: ActionExecutionCompletedMetric, tags: completedTags},
				}, metrics.calls)
			})
		}
	}
}
