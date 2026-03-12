// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package nvidia

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

// SetStatsForTest replaces the cached stats. Intended for testing only.
func (c *SystemProbeCache) SetStatsForTest(stats *model.GPUStats) {
	c.stats = stats
}

// InjectEventsForTest pushes events directly into the pending queue for a registered device.
// It is intended for deterministic tests that should not depend on async EventSetWait timing.
func (c *DeviceEventsGatherer) InjectEventsForTest(deviceUUID string, events []ddnvml.DeviceEventData) error {
	cache := c.getDeviceCache(deviceUUID)
	if cache == nil {
		return fmt.Errorf("device %s is not registered for events", deviceUUID)
	}

	for _, event := range events {
		select {
		case cache.pendingEvents <- event:
		default:
			return fmt.Errorf("pending event queue is full for device %s", deviceUUID)
		}
	}

	return nil
}
