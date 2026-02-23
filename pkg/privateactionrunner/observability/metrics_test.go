// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"errors"
	"testing"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observabilityLogger captures structured log calls so tests can assert on
// which level was used and what fields were attached.
type observabilityLogger struct {
	infoMessages []string
	infoFields   [][]log.Field
	warnMessages []string
	warnFields   [][]log.Field
}

func (l *observabilityLogger) Debug(msg string, fields ...log.Field)     {}
func (l *observabilityLogger) Debugf(format string, args ...interface{}) {}
func (l *observabilityLogger) Info(msg string, fields ...log.Field) {
	l.infoMessages = append(l.infoMessages, msg)
	l.infoFields = append(l.infoFields, fields)
}
func (l *observabilityLogger) Infof(format string, args ...interface{})  {}
func (l *observabilityLogger) Error(msg string, fields ...log.Field)     {}
func (l *observabilityLogger) Errorf(format string, args ...interface{}) {}
func (l *observabilityLogger) Warn(msg string, fields ...log.Field) {
	l.warnMessages = append(l.warnMessages, msg)
	l.warnFields = append(l.warnFields, fields)
}
func (l *observabilityLogger) Warnf(format string, args ...interface{}) {}
func (l *observabilityLogger) With(fields ...log.Field) log.Logger      { return l }

func hasFieldWithKey(fields []log.Field, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// TestReportExecutionStart_LogsInfoWithStructuredFields verifies that the execution start
// event is logged at Info level and includes the FQN, client, and task ID as separate fields.
func TestReportExecutionStart_LogsInfoWithStructuredFields(t *testing.T) {
	logger := &observabilityLogger{}
	const fqn = "com.datadoghq.http.request"
	const taskID = "task-abc"

	ReportExecutionStart(&statsd.NoOpClient{}, actionsclientpb.Client_WORKFLOWS, fqn, taskID, logger)

	require.Len(t, logger.infoMessages, 1)
	assert.Contains(t, logger.infoMessages[0], "Running private action")

	// Verify the structured fields carry observability context.
	require.Len(t, logger.infoFields, 1)
	assert.True(t, hasFieldWithKey(logger.infoFields[0], ActionFqnTagName), "action_fqn field must be present")
	assert.True(t, hasFieldWithKey(logger.infoFields[0], ActionClientTagName), "action_client field must be present")
	assert.True(t, hasFieldWithKey(logger.infoFields[0], TaskIDTagName), "task_id field must be present")
}

// TestReportExecutionCompleted_SuccessLogsInfoOnly verifies that a successful execution
// produces exactly one Info log and no Warn, which is the signal operators use to
// confirm an action ran without errors.
func TestReportExecutionCompleted_SuccessLogsInfoOnly(t *testing.T) {
	logger := &observabilityLogger{}

	ReportExecutionCompleted(
		&statsd.NoOpClient{},
		actionsclientpb.Client_WORKFLOWS,
		"com.datadoghq.http.request",
		"task-1",
		time.Now().Add(-time.Second),
		nil,
		logger,
	)

	assert.Empty(t, logger.warnMessages, "no warning should be emitted on success")
	require.Len(t, logger.infoMessages, 1)
	assert.Contains(t, logger.infoMessages[0], "success")
}

// TestReportExecutionCompleted_FailureLogsWarnWithErrorField verifies that a failed execution
// produces a Warn (not Info) with an "error" structured field carrying the original error.
// This is important: operators need the error field to diagnose action failures in log aggregators.
func TestReportExecutionCompleted_FailureLogsWarnWithErrorField(t *testing.T) {
	logger := &observabilityLogger{}
	execErr := errors.New("http timeout after 30s")

	ReportExecutionCompleted(
		&statsd.NoOpClient{},
		actionsclientpb.Client_APP_BUILDER,
		"com.datadoghq.kubernetes.core.getPods",
		"task-2",
		time.Now().Add(-2*time.Second),
		execErr,
		logger,
	)

	assert.Empty(t, logger.infoMessages, "no info should be emitted on failure")
	require.Len(t, logger.warnMessages, 1)
	assert.Contains(t, logger.warnMessages[0], "failure")

	require.Len(t, logger.warnFields, 1)
	assert.True(t, hasFieldWithKey(logger.warnFields[0], "error"),
		"error field must be present so the error surfaces in log aggregators")
}

// TestReportKeysManagerReady_LogsInfoWithDurationField verifies that the keys manager
// startup event is logged at Info and includes a duration field. This is the signal that
// the runner is ready to accept tasks after the initial key sync.
func TestReportKeysManagerReady_LogsInfoWithDurationField(t *testing.T) {
	logger := &observabilityLogger{}

	ReportKeysManagerReady(&statsd.NoOpClient{}, logger, time.Now().Add(-100*time.Millisecond))

	require.Len(t, logger.infoMessages, 1)
	assert.Contains(t, logger.infoMessages[0], "Keys manager ready")
	require.Len(t, logger.infoFields, 1)
	assert.True(t, hasFieldWithKey(logger.infoFields[0], Duration),
		"duration field must be present to track startup latency in logs")
}
