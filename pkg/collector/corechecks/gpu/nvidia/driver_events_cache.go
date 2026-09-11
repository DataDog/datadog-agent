// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DriverEventsCache manages driver events received from system-probe.
type DriverEventsCache struct {
	client *sysprobeclient.CheckClient
	events []model.DriverEvent
}

// NewDriverEventsCache creates a driver-event cache using client.
func NewDriverEventsCache(client *sysprobeclient.CheckClient) *DriverEventsCache {
	return &DriverEventsCache{client: client}
}

// Refresh fetches and caches new driver events from system-probe.
func (c *DriverEventsCache) Refresh() error {
	if c.client == nil {
		return errors.New("system-probe client is nil")
	}

	events, err := sysprobeclient.GetEndpoint[[]model.DriverEvent](c.client, "/driver-events", sysconfig.GPUMonitoringModule)
	if err != nil {
		c.events = nil
		if sysprobeclient.IgnoreStartupError(err) == nil {
			log.Debugf("System-probe GPU driver events endpoint not ready yet")
			return nil
		}
		return fmt.Errorf("get driver events from system-probe: %w", err)
	}

	c.events = events
	return nil
}

// Get returns the driver events from the latest refresh.
func (c *DriverEventsCache) Get() []model.DriverEvent {
	return slices.Clone(c.events)
}
