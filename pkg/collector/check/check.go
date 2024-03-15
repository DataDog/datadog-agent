// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package check

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
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
	Configure(senderManger sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error
	// Interval returns the interval time for the check
	Interval() time.Duration
	// ID provides a unique identifier for every check instance
	ID() checkid.ID
	// GetWarnings returns the last warning registered by the check
	GetWarnings() []error
	// GetSenderStats returns the stats from the last run of the check.
	GetSenderStats() (stats.SenderStats, error)
	// Version returns the version of the check if available
	Version() string
	// ConfigSource returns the configuration source of the check
	ConfigSource() string
	// IsTelemetryEnabled returns if telemetry is enabled for this check
	IsTelemetryEnabled() bool
	// InitConfig returns the init_config configuration of the check
	InitConfig() string
	// InstanceConfig returns the instance configuration of the check
	InstanceConfig() string
	// GetDiagnoses returns the diagnoses cached in last run or diagnose explicitly
	GetDiagnoses() ([]diagnosis.Diagnosis, error)
}

// Info is an interface to pull information from types capable to run checks. This is a subsection from the Check
// interface with only read only method.
type Info interface {
	// String provides a printable version of the check name
	String() string
	// Interval returns the interval time for the check
	Interval() time.Duration
	// ID provides a unique identifier for every check instance
	ID() checkid.ID
	// Version returns the version of the check if available
	Version() string
	// ConfigSource returns the configuration source of the check
	ConfigSource() string
	// InitConfig returns the init_config configuration of the check
	InitConfig() string
	// InstanceConfig returns the instance configuration of the check
	InstanceConfig() string
}

// ErrSkipCheckInstance is returned from Configure() when a check is intentionally refusing to load a
// check instance, and NOT due to an error. The distinction is important for deciding whether or not
// to log the error and report it on the status page.
//
// Loaders should check for this error after calling Configure() and should not log it as an error.
// The scheduler reports this error to the agent status if and only if all loaders fail to load the
// check instance.
//
// Usage example: one version of the check is written in Python, and another is written in Golang. Each
// loader is called on the given configuration, and will reject the configuration if it does not
// match the right version, without raising errors to the log or agent status. If another error is
// returned then the errors will be properly logged and reported in the agent status.
var ErrSkipCheckInstance = errors.New("refused to load the check instance")
