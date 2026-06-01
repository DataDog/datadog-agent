// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultRetrySchedule is the wait time between probe attempts. Length N
// means an entry is givenUp after the (N+1)th failure. Sum is the total
// retry window per (svcID, integration) pair.
var defaultRetrySchedule = []time.Duration{
	5 * time.Second, 5 * time.Second,
	30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
	30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
}

type defaultDiscoverer struct {
	bridge        Bridge
	cache         *cache
	retrySchedule []time.Duration
	now           func() time.Time
}

func newDiscoverer(bridge Bridge) *defaultDiscoverer {
	now := time.Now
	return &defaultDiscoverer{
		bridge:        bridge,
		cache:         newCache(now),
		retrySchedule: defaultRetrySchedule,
		now:           now,
	}
}

// New constructs a Discoverer wrapping the given Bridge. Pass nil bridge in
// configurations where Python is unavailable (cluster agent today); resolution
// of templates with Discovery set will then fail-closed.
func New(bridge Bridge) Discoverer {
	if bridge == nil {
		return nil
	}
	return newDiscoverer(bridge)
}

func (d *defaultDiscoverer) Discover(_ context.Context, integrationName string, svc listeners.Service) (Result, bool) {
	svcID := svc.GetServiceID()
	state := d.cache.lookup(svcID, integrationName)
	switch state.state {
	case stateHit:
		return state.result, true
	case stateGivenUp:
		return Result{}, false
	case statePending:
		if d.now().Before(state.nextRetryAt) {
			return Result{}, false
		}
		// fall through and probe
	}

	host, ok := pickHost(svc)
	if !ok {
		log.Debugf("autodiscovery/discoverer: %s has no host, skipping", svcID)
		d.cache.putFailure(svcID, integrationName, d.retrySchedule)
		return Result{}, false
	}
	exposed, err := svc.GetPorts()
	if err != nil {
		log.Debugf("autodiscovery/discoverer: %s GetPorts error: %v", svcID, err)
		d.cache.putFailure(svcID, integrationName, d.retrySchedule)
		return Result{}, false
	}

	payload := python.DiscoveryService{ID: svcID, Host: host}
	for _, p := range exposed {
		payload.Ports = append(payload.Ports, python.DiscoveryPort{Number: p.Port, Name: p.Name})
	}

	configs, err := d.bridge.DiscoverConfig(integrationName, payload)
	if err != nil {
		log.Warnf("autodiscovery/discoverer: %s.discover_config() failed for %s: %v", integrationName, svcID, err)
		d.cache.putFailure(svcID, integrationName, d.retrySchedule)
		return Result{}, false
	}
	if len(configs) == 0 {
		d.cache.putFailure(svcID, integrationName, d.retrySchedule)
		return Result{}, false
	}

	for i := range configs {
		if configs[i].Name == "" {
			configs[i].Name = integrationName
		}
		configs[i].Discovery = nil
	}
	r := Result{Configs: configs}
	d.cache.putSuccess(svcID, integrationName, r)
	return r, true
}

// IsPending reports whether the cache holds a "still retrying" failure entry
// for this (svcID, integrationName) pair.
func (d *defaultDiscoverer) IsPending(svcID, integrationName string) bool {
	return d.cache.lookup(svcID, integrationName).state == statePending
}

// Forget drops all cache entries for a service.
func (d *defaultDiscoverer) Forget(svcID string) {
	d.cache.forget(svcID)
}

func pickHost(svc listeners.Service) (string, bool) {
	hosts, err := svc.GetHosts()
	if err != nil || len(hosts) == 0 {
		return "", false
	}
	if h, ok := hosts["bridge"]; ok && h != "" {
		return h, true
	}
	for _, h := range hosts {
		if h != "" {
			return h, true
		}
	}
	return "", false
}
