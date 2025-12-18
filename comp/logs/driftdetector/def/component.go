// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package def provides the interface definition for the drift detector component.
package def

import (
	"time"
)

// team: agent-log-pipelines

// Component is the drift detector component interface
type Component interface {
	// ProcessLog processes a single log entry through the drift detection pipeline
	ProcessLog(timestamp time.Time, content string)

	// Start starts the drift detection pipeline
	Start() error

	// Stop stops the drift detection pipeline
	Stop()

	// IsEnabled returns whether drift detection is enabled
	IsEnabled() bool
}
