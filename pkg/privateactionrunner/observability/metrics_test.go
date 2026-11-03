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

func TestReportExecutionCompletedEnrichment(t *testing.T) {
	for _, mode := range []string{"nil", "empty", "enriched"} {
		for _, failure := range []error{nil, errors.New("secret-stderr")} {
			name := mode
			if failure != nil {
				name += " failure"
			} else {
				name += " success"
			}
			t.Run(name, func(t *testing.T) {
				var entries []executionLogEntry
				logger := executionLogRecorder{entries: &entries}
				metrics := &recordingTaggedStatsdClient{}
				var details *ExecutionDetails
				if mode != "nil" {
					var ctx context.Context
					ctx, details = WithExecutionDetails(context.Background())
					if mode == "enriched" {
						SetExecutionDetails(ctx, "execution", log.String("package_version", "1.2.3"))
					}
				}
				client := actionsclientpb.Client(0)
				start := ReportExecutionStart(metrics, client, "test.action", "task-id", logger)
				ReportExecutionCompleted(metrics, client, "test.action", "task-id", start, failure, logger, details)
				require.Len(t, entries, 2, "enrichment must not add lifecycle records")
				require.Equal(t, "Running private action", entries[0].message)
				require.Equal(t, "info", entries[0].level)
				identityFields := map[string]interface{}{ActionClientTagName: client.String()}
				require.Equal(t, identityFields, entries[0].fields)
				final := entries[1]
				result, level, message := "success", "info", "Private actions completed with success"
				if failure != nil {
					result, level, message = "error", "warn", "Private actions completed with failure"
				}
				require.Equal(t, level, final.level)
				require.Equal(t, message, final.message)
				if mode == "enriched" {
					require.Equal(t, "1.2.3", final.fields["package_version"])
					require.NotContains(t, final.fields, "error", "raw errors may contain script output")
				} else {
					if failure != nil {
						identityFields["error"] = failure
					}
					require.Equal(t, identityFields, final.fields, "unenriched actions retain their original log fields")
				}
				require.Len(t, metrics.calls, 3)
				require.Equal(t, ActionExecutionStartedMetric, metrics.calls[0].name)
				require.Equal(t, ActionExecutionLatencyMetric, metrics.calls[1].name)
				require.Equal(t, ActionExecutionCompletedMetric, metrics.calls[2].name)
				startTags := []string{fmt.Sprintf("%s:%s", ActionClientTagName, client), ActionFqnTagName + ":test.action"}
				completedTags := append(append([]string(nil), startTags...), ExecutionResultTagName+":"+result)
				require.Equal(t, startTags, metrics.calls[0].tags)
				require.Equal(t, completedTags, metrics.calls[1].tags)
				require.Equal(t, completedTags, metrics.calls[2].tags)
			})
		}
	}
}
