// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"os"
	"strconv"
	"time"

	"go.uber.org/fx"
)

const (
	envFxStartTimeoutOverride = "DD_FX_START_TIMEOUT_SECONDS"
	envFxStopTimeoutOverride  = "DD_FX_STOP_TIMEOUT_SECONDS"
	defaultFxTimeout          = 5 * time.Minute
)

// TemporaryAppTimeouts returns new fx Start/Stop timeout options, defaulting to 5 minutes.
//
// The start timeout can be overridden with the DD_FX_START_TIMEOUT_SECONDS environment variable.
// The stop timeout can be overridden with the DD_FX_STOP_TIMEOUT_SECONDS environment variable.
//
// Before fx the Agent did not have any start/stop timeouts, it would hang indefinitely. As we have
// have been adding more fx.Hooks we began hitting flaky tests with expired fx timeouts.
// We use a large timeout value by default to minimize the chance that customers will be impacted by the timeout.
// However, note that most platforms service managers send SIGKILL after a timeout
//   - upstart default is 5 seconds
//   - see pkg/util/winutil/servicemain/servicemain.go:Service.HardStopTimeout
//
// We can revisit this once we can better characterize the agent start/stop behavior and be intentional
// about timeout values
func TemporaryAppTimeouts() fx.Option {
	return fx.Options(
		fx.StartTimeout(timeoutFromEnv(envFxStartTimeoutOverride)),
		fx.StopTimeout(timeoutFromEnv(envFxStopTimeoutOverride)),
	)
}

// timeoutFromEnv reads the environment variable named @envVariable and returns a go duration for that many seconds.
// Returns defaultFxTimeout (5 minutes) if the environment variable does not exist or is not an integer.
func timeoutFromEnv(envVariable string) time.Duration {
	timeString, found := os.LookupEnv(envVariable)
	if !found {
		return defaultFxTimeout
	}
	timeValue, err := strconv.Atoi(timeString)
	if err != nil {
		return defaultFxTimeout
	}
	return time.Duration(timeValue) * time.Second
}
