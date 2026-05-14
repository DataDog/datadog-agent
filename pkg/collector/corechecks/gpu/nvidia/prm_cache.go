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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type prmCacheKey struct {
	deviceUUID string
	port       int
}

type prmCacheEntry struct {
	counters map[string]uint64
	err      error
}

// PRMCache manages the system-probe PRM metrics cache with fail-fast semantics.
type PRMCache struct {
	client    *sysprobeclient.CheckClient
	requests  []model.PRMRequest
	responses map[prmCacheKey]prmCacheEntry
}

// NewPRMCache creates a PRM cache that talks to system-probe.
func NewPRMCache() *PRMCache {
	timeout := pkgconfigsetup.Datadog().GetDuration("gpu.sp_process_metrics_request_timeout")
	client := sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		sysprobeclient.WithCheckTimeout(timeout),
		sysprobeclient.WithStartupCheckTimeout(timeout),
	)

	return &PRMCache{
		client:    client,
		responses: map[prmCacheKey]prmCacheEntry{},
	}
}

// RegisterRequest records an additional request that should be refreshed on each run.
func (c *PRMCache) RegisterRequest(request model.PRMRequest) {
	if !slices.Contains(c.requests, request) {
		c.requests = append(c.requests, request)
	}
}

// Refresh fetches fresh PRM counters from system-probe.
func (c *PRMCache) Refresh() error {
	if c.client == nil {
		return errors.New("PRM cache client is nil")
	}
	if len(c.requests) == 0 {
		c.responses = map[prmCacheKey]prmCacheEntry{}
		return nil
	}

	responses, err := sysprobeclient.Post[[]model.PRMResponse](c.client, "/prm-metrics", c.requests, sysconfig.GPUMonitoringModule)
	if err != nil {
		c.responses = map[prmCacheKey]prmCacheEntry{}
		if sysprobeclient.IgnoreStartupError(err) == nil {
			log.Debugf("System-probe GPU PRM endpoint not ready yet")
			return nil
		}
		return fmt.Errorf("failed to get PRM metrics from system-probe: %w", err)
	}

	cache := make(map[prmCacheKey]prmCacheEntry, len(responses))
	for _, response := range responses {
		key := prmCacheKey{deviceUUID: response.Request.DeviceUUID, port: response.Request.Port}
		entry := prmCacheEntry{counters: response.Counters}
		if response.Error != "" {
			entry.err = errors.New(response.Error)
		}
		cache[key] = entry
	}

	c.responses = cache
	return nil
}

// GetCounters returns the cached counters for the device/port pair.
func (c *PRMCache) GetCounters(deviceUUID string, port int) (map[string]uint64, error) {
	entry, found := c.responses[prmCacheKey{deviceUUID: deviceUUID, port: port}]
	if !found {
		return nil, fmt.Errorf("missing PRM response for device %s port %d", deviceUUID, port)
	}
	if entry.err != nil {
		return nil, entry.err
	}
	return entry.counters, nil
}
