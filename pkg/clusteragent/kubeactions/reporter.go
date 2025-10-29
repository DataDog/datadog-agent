// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// ResultReportingEnabled controls whether action results are reported back
	// Set to false to disable reporting (default)
	ResultReportingEnabled = false
)

// ResultReporter handles reporting action execution results back to the backend
type ResultReporter struct {
	enabled bool
	// TODO: Add fields for sending results back to backend
	// For example: HTTP client, API endpoint, batch queue, etc.
}

// NewResultReporter creates a new ResultReporter
func NewResultReporter() *ResultReporter {
	return &ResultReporter{
		enabled: ResultReportingEnabled,
	}
}

// ReportResult reports an action execution result
// This is currently disabled but can be enabled in the future
func (r *ResultReporter) ReportResult(actionKey ActionKey, result ExecutionResult, executedAt time.Time) {
	if !r.enabled {
		log.Debugf("Result reporting is disabled, skipping report for action %s", actionKey.String())
		return
	}

	// Create the result proto
	actionResult := &kubeactions.KubeActionResult{
		ActionId:   actionKey.ID,
		Version:    int64(actionKey.Version),
		Status:     result.Status,
		Message:    result.Message,
		ExecutedAt: timestamppb.New(executedAt),
	}

	// TODO: Implement actual reporting logic here
	// This could involve:
	// - Batching results
	// - Sending to a backend API
	// - Writing to a queue
	// - Publishing to a message bus
	// - etc.

	log.Debugf("Would report result (disabled): %+v", actionResult)
}

// Enable enables result reporting
func (r *ResultReporter) Enable() {
	r.enabled = true
	log.Infof("Result reporting enabled")
}

// Disable disables result reporting
func (r *ResultReporter) Disable() {
	r.enabled = false
	log.Infof("Result reporting disabled")
}

// IsEnabled returns whether result reporting is enabled
func (r *ResultReporter) IsEnabled() bool {
	return r.enabled
}
