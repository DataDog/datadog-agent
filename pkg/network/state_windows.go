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

// maybeSuppressWindowsLingeringFlow detects and suppresses duplicate failure
// re-reports from flows that are stuck in the ddnpm driver's openFlows table.
//
// Root cause: the driver moves a terminated flow from openFlows to closedFlows
// via InterlockedCompareExchange(&refcount, 2, 2) in flowdata.c. A flow's
// refcount reaches 2 only after all 4 WFP flowDeleteNotify callbacks have
// fired. If the WFP transport callout holds a ref concurrently (processing a
// data packet on a real NIC when the 4th notify fires), the CAS sees refcount=3,
// silently skips the move, and the flow stays in openFlows indefinitely with a
// terminal connectionStatus (e.g. CONN_STAT_EST_RECV_RST).
//
// Each subsequent poll re-reports the flow. FlowToConnStat sets TCPFailures to a
// fresh {104:1} map every time (not accumulated), and since TCPFailures is not
// part of StatCounters there is no delta baseline. TCPClosed=1 only ships on the
// first poll (PR #47005); subsequent polls ship TCPClosed=0 delta with a new
// TCPFailures entry, causing the backend rate to climb past 100% without bound.
//
// Fix: once a connection has been reported as closed (sts.TCPClosed > 0), any
// re-report lacking a new close event (last.TCPClosed == 0) is a duplicate.
// Zero out TCPFailures so no additional failures are shipped for this poll.
// If all other delta fields are also zero, the connection is filtered from the
// payload entirely by the IsZero check in GetDelta.
func maybeSuppressWindowsLingeringFlow(c *ConnectionStats, sts, last StatCounters) {
	if sts.TCPClosed > 0 && last.TCPClosed == 0 && len(c.TCPFailures) > 0 {
		stateTelemetry.windowsLingeringFlows.Inc()
		// Log before zeroing so the closure sees the non-nil map.
		log.DebugFunc(func() string {
			return fmt.Sprintf("NPM windows: suppressing lingering flow cookie:%d already-closed re-report (failures: %v)", c.Cookie, c.TCPFailures)
		})
		c.TCPFailures = nil
	}
}
