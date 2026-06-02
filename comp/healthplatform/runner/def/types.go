// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runner

import "time"

// BuiltInHealthCheck is the base configuration shared by all built-in health checks.
// Source is the reporting component label.
// Fn returns zero or more IssueReports; returning nil/empty means no issue detected.
// IssueNames is populated automatically by Registry.RegisterModule from module.IssueName();
// module authors must not set it. bundle.go uses it to query the store for persisted
// issues from a prior run so checks can resolve them after restart.
type BuiltInHealthCheck struct {
	Source     string
	Fn         HealthCheckFunc
	IssueNames []string
}

// BuiltInPeriodicHealthCheck is a BuiltInHealthCheck that runs on a recurring schedule.
// Interval is the period between runs; zero uses the scheduler's default.
type BuiltInPeriodicHealthCheck struct {
	BuiltInHealthCheck
	Interval time.Duration
}
