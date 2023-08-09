// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package limiter TODO comment
package limiter

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FromConfig builds new Limiter from the configuration.
func FromConfig(pipelineCount int, enabled bool) *Limiter {
	return fromConfig(pipelineCount, enabled, getCgroupMemoryLimit)
}

func fromConfig(pipelineCount int, enabled bool, cgroupLimitGetter func() (uint64, error)) *Limiter {
	if !enabled {
		return nil
	}

	// If all of the following are true:
	//
	// - dogstatsd_context_limiter.cgroup_memory_ratio is set to a valid value
	// - dogstatsd_context_limiter.bytes_per_context is set to a valid value
	// - no errors occur while fetching cgroup limit
	//
	// Then the mem ratio based limiting will apply.
	//
	// Else the static limit defined by dogstatsd_context_limiter.limit will be used.

	limit := 0
	memoryRatio := config.Datadog.GetFloat64("dogstatsd_context_limiter.cgroup_memory_ratio")
	bytesPerContext := config.Datadog.GetInt("dogstatsd_context_limiter.bytes_per_context")
	if memoryRatio > 0 && bytesPerContext > 0 {
		cgroupLimit, err := cgroupLimitGetter()
		if err != nil {
			log.Errorf("dogstatsd context limiter: memory based limit configured, but: %v", err)
		} else {
			limit = int(memoryRatio*float64(cgroupLimit)) / bytesPerContext
			log.Debugf("dogstatsd context limiter: memory limit=%d, ratio=%f, contexts limit=%d", cgroupLimit, memoryRatio, limit)
		}
	}

	if limit == 0 {
		limit = config.Datadog.GetInt("dogstatsd_context_limiter.limit")
		log.Debugf("dogstatsd context limiter: using fixed global limit %d", limit)
	}

	if pipelineCount > 0 {
		limit = limit / pipelineCount
	}

	return NewGlobal(
		limit,
		config.Datadog.GetInt("dogstatsd_context_limiter.entry_timeout"),
		config.Datadog.GetString("dogstatsd_context_limiter.key_tag_name"),
		config.Datadog.GetStringSlice("dogstatsd_context_limiter.telemetry_tag_names"),
	)
}
