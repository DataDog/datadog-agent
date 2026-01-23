// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "synthetics_agent"

var (
	// ChecksReceived tracks the number of synthetics checks received from remote config
	ChecksReceived = telemetry.NewCounter(
		subsystem,
		"checks_received",
		nil,
		"Number of synthetics checks received from remote config",
	)

	// ChecksProcessed tracks the number of synthetics checks processed
	ChecksProcessed = telemetry.NewCounter(
		subsystem,
		"checks_processed",
		[]string{"status", "subtype"},
		"Number of synthetics checks processed",
	)

	// ErrorTestConfig tracks errors when interpreting test configuration
	ErrorTestConfig = telemetry.NewCounter(
		subsystem,
		"error_test_config",
		[]string{"subtype"},
		"Errors when interpreting test configuration",
	)

	// TracerouteError tracks errors when running traceroute
	TracerouteError = telemetry.NewCounter(
		subsystem,
		"traceroute_error",
		[]string{"subtype"},
		"Errors when running datadog traceroute",
	)

	// SendResultFailure tracks errors when sending results to the event platform
	SendResultFailure = telemetry.NewCounter(
		subsystem,
		"evp_send_result_failure",
		[]string{"subtype"},
		"Errors when sending results to the event platform",
	)
)
