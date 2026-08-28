// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscoveryimpl implements the ndmdiscovery component.
package ndmdiscoveryimpl

import (
	"context"
	"errors"

	"golang.org/x/sync/semaphore"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	networkdevices "github.com/DataDog/datadog-agent/comp/networkdevices/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
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

// Requires declares the dependencies of the ndmdiscovery component.
type Requires struct {
	compdef.In

	Lifecycle      compdef.Lifecycle
	Log            log.Component
	Config         config.Component
	EventPlatform  eventplatform.Component
	NetworkDevices networkdevices.Component
}

// Provides declares what the ndmdiscovery component provides.
type Provides struct {
	Comp       ndmdiscovery.Component
	RCListener rcclienttypes.ListenerProvider
}

type ndmDiscovery struct {
	log   log.Component
	sched *scheduler
	rc    *rcHandler

	checker connectivityChecker
	enabled bool
}

// NewComponent builds the ndmdiscovery component. When the feature is off it
// returns a component that subscribes to nothing and registers no hooks, so
// there is exactly one construction path.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &ndmDiscovery{log: reqs.Log}

	if !configutils.IsRemoteConfigEnabled(reqs.Config) || !reqs.Config.GetBool(enabledConfigKey) {
		reqs.Log.Debug("ndmdiscovery: disabled")
		return Provides{Comp: comp}, nil
	}

	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		return Provides{}, errors.New("ndmdiscovery: the event platform forwarder is not available")
	}

	// The worker count is read exactly once. The same value sizes the
	// semaphore, becomes the sweeper's budget, and becomes the scheduler's
	// global budget. A budget larger than the semaphore is never detected at
	// run time: every Acquire would simply block until the context is
	// cancelled, stalling all sweeps silently.
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
	comp.checker = reqs.NetworkDevices
	comp.sched = newScheduler(
		newSweeper(
			reqs.NetworkDevices,
			newPayloadReporter(forwarder, reqs.Log),
			newPersistentCursorStore(),
			semaphore.NewWeighted(workers),
			workers,
			reqs.Log,
		),
		newConfigCredentialStore(reqs.Config),
		reqs.Log,
		schedulerOptions{Workers: workers, MaxAddresses: defaults.MaxAddresses, Defaults: defaults},
	)
	comp.rc = newRCHandler(comp.sched, reqs.Log, defaults)

	// This hook must run before rcclient's, so that sched.start precedes the
	// first rc.Update. It does today because rcclient consumes the
	// group:"rCListener" value provided below, which makes this constructor a
	// dependency of rcclient's and orders our OnStart first.
	//
	// If that ordering ever inverts, the failure is permanent rather than
	// transient: an update that lands before sched.start is rejected with
	// "the discovery scheduler is not running" (see scheduler.set), and Remote
	// Configuration does not re-send an unchanged config, so every range is
	// rejected once and never retried until the config changes or the agent
	// restarts. Anything that changes how this component is wired into
	// rcclient must preserve the ordering.
	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{
		Comp: comp,
		// One shared product, filtered by the kind field in the payload: a
		// config whose kind is not "autodiscovery" is ignored silently and its
		// apply state is left untouched, so sharing a product with another
		// feature is safe.
		//
		// TODO(NDM): switch to data.ProductNDM once that product is provisioned
		// backend side. MANAGED_DEPLOYMENTS_DEBUG is a POC placeholder: it
		// already exists, which is what lets us push ranges to a real Agent
		// today. It carries other features' payloads, so the kind filter in
		// rcupdate.go is load-bearing here rather than merely defensive.
		RCListener: rcclienttypes.ListenerProvider{
			ListenerProvider: rcclienttypes.RCListener{
				data.ProductManagedDeploymentsDebug: comp.rc.Update,
			},
		},
	}, nil
}

func (d *ndmDiscovery) start(ctx context.Context) error {
	// The connectivity engine reports every ping failure as unreachable,
	// including a lack of privileges. Probing once at start means a
	// ping-incapable agent leaves ping_status empty instead of marking every
	// address in the org unreachable.
	if probePing(ctx, d.checker) {
		d.sched.setPingEnabled(true)
	} else {
		d.log.Warn("ndmdiscovery: this agent cannot send ICMP echo requests, so ping status will not be reported")
	}

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

// RangeCount implements ndmdiscovery.Component.
func (d *ndmDiscovery) RangeCount() int {
	if !d.enabled {
		return 0
	}
	return d.sched.count()
}
