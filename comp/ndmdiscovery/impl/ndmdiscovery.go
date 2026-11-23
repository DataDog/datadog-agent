// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscoveryimpl implements the ndmdiscovery component.
package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
)

// Config keys. network_devices.autodiscovery.* belongs to the legacy snmp_listener.
const (
	enabledConfigKey         = "network_devices.discovery.enabled"
	workersConfigKey         = "network_devices.discovery.workers"
	defaultIntervalConfigKey = "network_devices.discovery.default_interval_sec"
	maxAddressesConfigKey    = "network_devices.discovery.max_range_addresses"
	namespaceConfigKey       = "network_devices.discovery.default_namespace"
)

// errDisabled is returned for every range handed to a disabled component.
var errDisabled = errors.New("the ndm discovery component is disabled: set network_devices.discovery.enabled")

// Requires declares the dependencies of the ndmdiscovery component.
type Requires struct {
	compdef.In

	Lifecycle     compdef.Lifecycle
	Log           log.Component
	Config        config.Component
	EventPlatform eventplatform.Component
}

// Provides declares what the ndmdiscovery component provides.
type Provides struct {
	compdef.Out

	Comp           ndmdiscovery.Component
	StatusProvider status.InformationProvider
}

type ndmDiscovery struct {
	log      log.Component
	sched    *scheduler
	defaults rangeDefaults

	enabled bool

	mu sync.Mutex
	// rejected is every range the latest Schedule call refused, for the status page.
	rejected []rejectedRange
}

// rejectedRange is a range Schedule refused, and why.
type rejectedRange struct {
	ID             string
	NetworkAddress string
	Error          string
}

// NewComponent builds the ndmdiscovery component. A disabled component rejects every range.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &ndmDiscovery{log: reqs.Log}

	if !reqs.Config.GetBool(enabledConfigKey) {
		reqs.Log.Debug("ndmdiscovery: disabled")
		return Provides{Comp: comp, StatusProvider: status.NewInformationProvider(nil)}, nil
	}

	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		return Provides{}, errors.New("ndmdiscovery: the event platform forwarder is not available")
	}

	workers := reqs.Config.GetInt64(workersConfigKey)
	if workers < 1 {
		workers = 1
	}
	defaults := rangeDefaults{
		Namespace:    reqs.Config.GetString(namespaceConfigKey),
		IntervalSec:  reqs.Config.GetInt(defaultIntervalConfigKey),
		MaxAddresses: reqs.Config.GetInt(maxAddressesConfigKey),
	}

	comp.enabled = true
	comp.defaults = defaults
	comp.sched = newScheduler(
		newSweeper(
			probe.Scan,
			newPayloadReporter(forwarder, reqs.Log),
			newPersistentCursorStore(),
			semaphore.NewWeighted(workers),
			workers,
			reqs.Log,
		),
		reqs.Log,
		schedulerOptions{
			Workers:      workers,
			MaxAddresses: defaults.MaxAddresses,
			Defaults:     defaults,
			Credentials:  credentials.NewStore(reqs.Config),
		},
	)

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{Comp: comp, StatusProvider: status.NewInformationProvider(statusProvider{d: comp})}, nil
}

func (d *ndmDiscovery) start(_ context.Context) error {
	d.sched.start(context.Background())
	d.log.Info("ndmdiscovery: started")
	return nil
}

func (d *ndmDiscovery) stop(_ context.Context) error {
	d.sched.stop()
	return nil
}

// Schedule implements ndmdiscovery.Component.
func (d *ndmDiscovery) Schedule(ranges []ndmdiscovery.Range) map[string]error {
	errs := make(map[string]error)

	if !d.enabled {
		for _, r := range ranges {
			errs[r.ID] = errDisabled
		}
		if len(ranges) > 0 {
			d.log.Warnf("ndmdiscovery: rejecting %d range(s): %v", len(ranges), errDisabled)
		}
		return errs
	}

	var rejected []rejectedRange
	scheduled := make(map[string]struct{}, len(ranges))
	for _, r := range ranges {
		cfg, err := parseRange(r, d.defaults, d.log)
		if err == nil {
			err = d.sched.set(cfg)
		}
		if err != nil {
			errs[r.ID] = err
			rejected = append(rejected, rejectedRange{ID: r.ID, NetworkAddress: r.NetworkAddress, Error: err.Error()})
			d.log.Warnf("ndmdiscovery: rejected range %s (%s): %v", r.ID, r.NetworkAddress, err)
			continue
		}
		scheduled[cfg.AutodiscoveryID] = struct{}{}
	}

	d.mu.Lock()
	d.rejected = rejected
	d.mu.Unlock()

	for _, id := range d.sched.ids() {
		if _, kept := scheduled[id]; !kept {
			d.sched.remove(id)
			d.log.Infof("ndmdiscovery: stopped sweeping range %s", id)
		}
	}
	return errs
}

// RangeCount implements ndmdiscovery.Component.
func (d *ndmDiscovery) RangeCount() int {
	if !d.enabled {
		return 0
	}
	return d.sched.count()
}
