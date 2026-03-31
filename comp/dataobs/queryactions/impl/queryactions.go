// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package queryactionsimpl implements the Data Observability query actions component
package queryactionsimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	doqueryactions "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"gopkg.in/yaml.v3"
)

// Requires defines the dependencies for the Data Observability query actions component
type Requires struct {
	Lc       compdef.Lifecycle
	Log      log.Component
	RcClient rcclient.Component
	Ac       autodiscovery.Component
}

// Provides defines the output of the Data Observability query actions component
type Provides struct {
	Comp doqueryactions.Component
}

// component implements the Data Observability query actions component
type component struct {
	log             log.Component
	ac              autodiscovery.Component
	rcclient        rcclient.Component
	activeConfigs   map[string]activeConfigEntry
	activeConfigsMu sync.Mutex
}

// NewComponent creates a new Data Observability query actions component
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		ac:            reqs.Ac,
		rcclient:      reqs.RcClient,
		activeConfigs: make(map[string]activeConfigEntry),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

func (c *component) start(_ context.Context) error {
	c.ac.AddConfigProvider(c, false, 0)
	c.log.Info("Data Observability query actions component started")
	return nil
}

// String returns the name of the provider
func (c *component) String() string {
	return names.DOQueryActions
}

// GetConfigErrors is required by the ConfigProvider interface. This provider does not track
// per-resource errors; RC apply errors are reported via the applyStatus callback in onRCUpdate.
func (c *component) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream creates a fresh channel per call (same pattern as container/process_log providers).
// An empty ConfigChanges is sent immediately because autodiscovery's LoadAndRun iterates
// providers sequentially and blocks on each streaming provider until its first message arrives,
// before proceeding to the next provider.
//
// RC configs are declarative snapshots so only the latest matters. The RC callback writes to
// outCh with replace semantics: if autodiscovery hasn't consumed the previous update yet, the
// old entry is replaced with the latest one. Unschedule entries from the dropped update are
// preserved to prevent check leaks. outCh is closed when ctx is cancelled so the config poller
// goroutine can observe teardown.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)
	// Unblock autodiscovery's LoadAndRun — it blocks on <-ch until the first message arrives.
	outCh <- integration.ConfigChanges{}

	var (
		mu     sync.Mutex
		closed bool
	)

	// sendChanges delivers changes to outCh under mu. The channel is capacity-1; when full,
	// the old entry is drained and its Unschedule events are merged into changes to prevent
	// check leaks. mu also guards against writing to a closed channel after shutdown.
	sendChanges := func(changes integration.ConfigChanges) {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		select {
		case outCh <- changes:
		default:
			// Channel full: drain old entry, preserving its Unschedule events so that
			// checks already in autodiscovery are not orphaned.
			// The drain is non-blocking because config_poller may have already read the
			// buffer between the default branch firing and this point.
			var dropped integration.ConfigChanges
			select {
			case dropped = <-outCh:
			default:
			}
			changes.Schedule = append(dropped.Schedule, changes.Schedule...)
			changes.Unschedule = append(dropped.Unschedule, changes.Unschedule...)
			outCh <- changes // safe: mu held, closed=false, channel was just drained
		}
	}

	// subscribeAndWait subscribes to the RC product and blocks until ctx is cancelled.
	subscribeAndWait := func() {
		c.rcclient.Subscribe(data.ProductDOQueryActions, func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
			changes := c.onRCUpdate(updates, applyStatus)
			if !changes.IsEmpty() {
				sendChanges(changes)
			}
		})
		c.log.Info("Subscribed to RC DO_QUERY_ACTIONS product for Data Observability query actions")
		<-ctx.Done()
	}

	go func() {
		defer func() {
			// Close outCh so the config poller goroutine can observe shutdown.
			// mu prevents a concurrent RC callback from writing to the closed channel.
			mu.Lock()
			defer mu.Unlock()
			closed = true
			close(outCh)
		}()

		// Check immediately: the file config provider runs before this one in LoadAndRun,
		// so postgres is typically already available when Stream() is called.
		if c.hasSupportedIntegration() {
			subscribeAndWait()
			return
		}

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.hasSupportedIntegration() {
					subscribeAndWait()
					return
				}
			}
		}
	}()

	return outCh
}

// hasSupportedIntegration checks if any supported DB integration with
// data_observability.enabled: true is configured in autodiscovery.
func (c *component) hasSupportedIntegration() bool {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if !isSupportedIntegration(cfg.Name) {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if instanceHasDOEnabled(instance) {
				return true
			}
		}
	}
	return false
}
