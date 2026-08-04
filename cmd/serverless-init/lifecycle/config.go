// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lifecycle implements the AWS Lambda MicroVM lifecycle hook server.
package lifecycle

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// DefaultPort is the port the MicroVM platform expects the lifecycle hook server on.
const DefaultPort = 9000

// UserAppPortEnvVar opts in to forwarding lifecycle hooks to the user app.
const UserAppPortEnvVar = "DD_AWS_MICROVM_USER_APP_PORT"

// LifecyclePortEnvVar overrides the port the lifecycle hook server listens on (default 9000).
const LifecyclePortEnvVar = "DD_AWS_MICROVM_LIFECYCLE_PORT"

// ForwardTimeoutMsEnvVar overrides the timeout (ms) for /run, /resume, /suspend, /terminate.
const ForwardTimeoutMsEnvVar = "DD_AWS_MICROVM_FORWARD_TIMEOUT_MS"

// ReadyTimeoutMsEnvVar overrides the timeout (ms) for /ready.
const ReadyTimeoutMsEnvVar = "DD_AWS_MICROVM_READY_TIMEOUT_MS"

// ValidateTimeoutMsEnvVar overrides the timeout (ms) for /validate.
const ValidateTimeoutMsEnvVar = "DD_AWS_MICROVM_VALIDATE_TIMEOUT_MS"

// parsePort parses a port number from a raw env-var string.
// Returns defaultVal when raw is empty. Parsed port must not equal any value in forbidden.
func parsePort(envVar, raw string, defaultVal int, forbidden ...int) (int, error) {
	if raw == "" {
		return defaultVal, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: must be an integer (got %q)", envVar, raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s: must be in [1, 65535] (got %d)", envVar, port)
	}
	for _, f := range forbidden {
		if port == f {
			return 0, fmt.Errorf("%s: must not equal %d", envVar, f)
		}
	}
	return port, nil
}

// parseDurationMs parses a millisecond integer env var. Returns defaultVal when raw is empty.
func parseDurationMs(envVar, raw string, defaultVal time.Duration) (time.Duration, error) {
	if raw == "" {
		return defaultVal, nil
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: must be an integer number of milliseconds (got %q)", envVar, raw)
	}
	if ms <= 0 {
		return 0, fmt.Errorf("%s: must be a positive number of milliseconds (got %d)", envVar, ms)
	}
	if ms > int64(math.MaxInt64/time.Millisecond) {
		return 0, fmt.Errorf("%s: too large to represent as a duration (got %d ms)", envVar, ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}
