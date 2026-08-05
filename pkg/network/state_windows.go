// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package network

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dropStaleFlowFailures suppresses duplicate TCPFailures from flows that are
// still in the driver's open state and being re-reported across multiple polls
// while waiting to fully close. Once a connection has already been reported as
// closed (sts.TCPClosed > 0), any re-report with no new close event
// (last.TCPClosed == 0) is a duplicate — zero out its TCPFailures so no
// additional failures ship for that poll.
func dropStaleFlowFailures(c *ConnectionStats, sts, last StatCounters) {
	if sts.TCPClosed > 0 && last.TCPClosed == 0 && len(c.TCPFailures) > 0 {
		stateTelemetry.windowsLingeringFlows.Inc()
		// Log before zeroing so the closure sees the non-nil map.
		log.DebugFunc(func() string {
			return fmt.Sprintf("NPM windows: suppressing lingering flow cookie:%d already-closed re-report (failures: %v)", c.Cookie, c.TCPFailures)
		})
		c.TCPFailures = nil
	}
}
