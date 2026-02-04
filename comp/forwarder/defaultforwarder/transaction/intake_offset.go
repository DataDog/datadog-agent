// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"expvar"
	"net/http"

	"github.com/benbjohnson/clock"
)

const intakeTimeOffsetExpvarName = "corechecks_net_ntp_intake_time_offset"

var (
	intakeTimeOffsetExpvar = expvar.NewFloat(intakeTimeOffsetExpvarName)
	defaultClock           = clock.New()
)

// updateIntakeTimeOffset parses the Date header from an HTTP response and updates the intake time offset.
// The offset uses NTP convention: positive means agent is behind, negative means ahead.
func updateIntakeTimeOffset(dateHeader string) {
	updateIntakeTimeOffsetWithClock(dateHeader, defaultClock)
}

// updateIntakeTimeOffsetWithClock is the same as updateIntakeTimeOffset but accepts a clock for testing.
func updateIntakeTimeOffsetWithClock(dateHeader string, clk clock.Clock) {
	if dateHeader == "" {
		return
	}

	intakeServerTime, err := http.ParseTime(dateHeader)
	if err != nil {
		return
	}

	now := clk.Now()
	// Calculate offset using NTP convention: positive means agent clock is behind, negative means ahead
	// serverTime - agentTime: if result is positive, agent is behind (needs to add time)
	offset := intakeServerTime.Sub(now).Seconds()
	intakeTimeOffsetExpvar.Set(offset)
}
