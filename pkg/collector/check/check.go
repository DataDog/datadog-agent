// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// Check is an interface for types capable to run checks
type Check interface {
	// Run runs the check
	Run() error
	// Stop stops the check if it's running
	Stop()
	// Cancel cancels the check. Cancel is called when the check is unscheduled:
	// - unlike Stop, it is called even if the check is not running when it's unscheduled
	// - if the check is running, Cancel is called after Stop and may be called before the call to Stop completes
	Cancel()
	// String provides a printable version of the check name
	String() string
	// Configure configures the check
	Configure(config, initConfig integration.Data, source string) error
	// Interval returns the interval time for the check
	Interval() time.Duration
	// ID provides a unique identifier for every check instance
	ID() ID
	// GetWarnings returns the last warning registered by the check
	GetWarnings() []error
	// GetSenderStats returns the stats from the last run of the check.
	GetSenderStats() (SenderStats, error)
	// Version returns the version of the check if available
	Version() string
	// ConfigSource returns the configuration source of the check
	ConfigSource() string
	// IsTelemetryEnabled returns if telemetry is enabled for this check
	IsTelemetryEnabled() bool
}
