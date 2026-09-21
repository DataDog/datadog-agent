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
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

// Config keys. The network_devices.autodiscovery.* prefix belongs to the
// legacy snmp_listener, so this component uses network_devices.discovery.*.
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
	Comp ndmdiscovery.Component
}

type ndmDiscovery struct {
	log      log.Component
	sched    *scheduler
	defaults rangeDefaults

	pingOnce sync.Once
	ping     pingprobe.Capability
	enabled  bool
}

// NewComponent builds the ndmdiscovery component. When the feature is off it
// returns a component that rejects every range and registers no hooks.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &ndmDiscovery{log: reqs.Log}

	if !reqs.Config.GetBool(enabledConfigKey) {
		reqs.Log.Debug("ndmdiscovery: disabled")
		return Provides{Comp: comp}, nil
	}

	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		return Provides{}, errors.New("ndmdiscovery: the event platform forwarder is not available")
	}

	// The worker count is read once: the same value sizes the semaphore, the
	// sweeper's budget, and the scheduler's global budget.
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

	return Provides{Comp: comp}, nil
}

// pingCapability detects once whether this process can send ICMP echo requests.
func (d *ndmDiscovery) pingCapability() pingprobe.Capability {
	d.pingOnce.Do(func() {
		d.ping = pingprobe.Detect()
		if !d.ping.Available {
			d.log.Warnf("ndmdiscovery: the ping probe is not available: %s", d.ping.Reason)
		}
	})
	return d.ping
}

func (d *ndmDiscovery) start(_ context.Context) error {
	d.pingCapability()

	// The scheduler outlives the start hook's context, which is cancelled once
	// startup finishes.
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
		return errs
	}

	scheduled := make(map[string]struct{}, len(ranges))
	for _, r := range ranges {
		cfg, err := parseRange(r, d.defaults, d.pingCapability(), d.log)
		if err != nil {
			errs[r.ID] = err
			continue
		}
		if err := d.sched.set(cfg); err != nil {
			errs[r.ID] = err
			continue
		}
		scheduled[cfg.AutodiscoveryID] = struct{}{}
	}

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
