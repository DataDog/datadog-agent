// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"time"
)

type schedulerConfigs struct {
	workers                    int
	flushInterval              time.Duration
	syntheticsSchedulerEnabled bool
}

func newSchedulerConfigs(agentConfig config.Component) *schedulerConfigs {
	return &schedulerConfigs{
		syntheticsSchedulerEnabled: agentConfig.GetBool("synthetics.scheduler.enabled"),
		workers:                    agentConfig.GetInt("synthetics.collector.workers"),
		flushInterval:              agentConfig.GetDuration("synthetics.collector.flush_interval"),
	}
}
