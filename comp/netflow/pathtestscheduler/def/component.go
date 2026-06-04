// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package pathtestscheduler is the NDM dynamic path test scheduler component.
package pathtestscheduler

import (
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// team: network-devices

// Component is the NDM dynamic path test scheduler. It receives batches of
// NetFlow records (typically from the FlowAggregator at flush time) and,
// when enabled, converts them into path tests and hands them off to the
// npcollector for execution.
type Component interface {
	// ScheduleFromFlows is called with a batch of NetFlow records. It is
	// non-blocking and must never error-propagate; failures are reported
	// via metrics. Safe to call with a nil or empty slice.
	ScheduleFromFlows(flows []*common.Flow)
}
