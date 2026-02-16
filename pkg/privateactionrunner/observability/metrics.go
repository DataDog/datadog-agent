// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
)

// Metrics in this file are submitted on behalf of the user so they must be not billable.
// Make sure to allowlist all the metrics in dd-analytics.
// Example https://github.com/DataDog/dd-analytics/pull/39949
const (
	ActionExecutionStartedMetric   = "datadog.actions.private_runner.executions.started"
	ActionExecutionCompletedMetric = "datadog.actions.private_runner.executions.completed"
	ActionExecutionLatencyMetric   = "datadog.actions.private_runner.executions.latency"

	HealthCheckMetric = "datadog.actions.private_runner.health.check"

	KeysManagerStartupLatencyMetric = "datadog.actions.private_runner.keys_manager.startup_latency"
)

func ReportExecutionStart(metricsClient statsd.ClientInterface, client actionsclientpb.Client, fqn, taskID string, logger log.Logger) time.Time {
	logger.Info("Running private action", log.String(ActionFqnTagName, fqn), log.String(ActionClientTagName, client.String()), log.String(TaskIDTagName, taskID))
	tags := []string{fmt.Sprintf("%s:%s", ActionClientTagName, client), fmt.Sprintf("%s:%s", ActionFqnTagName, fqn)}
	_ = metricsClient.Incr(ActionExecutionStartedMetric, tags, 1.0)
	return time.Now()
}

func ReportExecutionCompleted(metricsClient statsd.ClientInterface, client actionsclientpb.Client, fqn, taskId string, startTime time.Time, err error, logger log.Logger) {
	logger = logger.With(log.String(ActionFqnTagName, fqn), log.String(ActionClientTagName, client.String()), log.String(TaskIDTagName, taskId))
	tags := []string{fmt.Sprintf("%s:%s", ActionClientTagName, client), fmt.Sprintf("%s:%s", ActionFqnTagName, fqn)}
	if err != nil {
		logger.Warn("Private actions completed with failure", log.ErrorField(err))
		tags = append(tags, fmt.Sprintf("%s:%s", ExecutionResultTagName, "error"))
	} else {
		logger.Info("Private actions completed with success")
		tags = append(tags, fmt.Sprintf("%s:%s", ExecutionResultTagName, "success"))
	}
	_ = metricsClient.Timing(ActionExecutionLatencyMetric, time.Since(startTime), tags, 1.0)
	_ = metricsClient.Incr(ActionExecutionCompletedMetric, tags, 1.0)
}

func ReportKeysManagerReady(client statsd.ClientInterface, logger log.Logger, startTime time.Time) {
	logger.Info("Keys manager ready", log.Duration(Duration, time.Since(startTime)))
	_ = client.Timing(KeysManagerStartupLatencyMetric, time.Since(startTime), []string{}, 1.0)
}
