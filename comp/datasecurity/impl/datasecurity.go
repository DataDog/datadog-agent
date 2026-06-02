// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurityimpl implements the data security component.
package datasecurityimpl

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	provtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	datasecurity "github.com/DataDog/datadog-agent/comp/datasecurity/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

// Requires defines the dependencies of the data security component.
type Requires struct {
	Lc       compdef.Lifecycle
	Log      log.Component
	RcClient rcclient.Component
	Ac       autodiscovery.Component
}

// Provides defines the output of the data security component.
type Provides struct {
	Comp datasecurity.Component
}

// component implements the data security component.
//
// It is also an autodiscovery streaming config provider: matching RC payloads
// are turned into integration check configs and scheduled so an
// integrations-core check can consume the rules.
type component struct {
	log      log.Component
	rcclient rcclient.Component
	ac       autodiscovery.Component

	// configChanges carries scheduled/unscheduled check configs to autodiscovery.
	// closeMu guards closing the channel against concurrent RC callbacks.
	configChanges chan integration.ConfigChanges
	closeMu       sync.RWMutex
	closed        bool

	// activeConfigs tracks, keyed by RC config path, the takeover state for each
	// handled data_security config: the enriched postgres config(s) we scheduled
	// and the original file config(s) we unscheduled to make room for them.
	activeConfigsMu sync.Mutex
	activeConfigs   map[string]takeover
}

// Compile-time checks that the component satisfies the config provider interfaces.
var (
	_ provtypes.ConfigProvider          = (*component)(nil)
	_ provtypes.StreamingConfigProvider = (*component)(nil)
)

// NewComponent creates a new data security component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		rcclient:      reqs.RcClient,
		ac:            reqs.Ac,
		configChanges: make(chan integration.ConfigChanges, 10),
		activeConfigs: make(map[string]takeover),
	}

	// Send an initial empty change so autodiscovery's LoadAndRun, which blocks
	// on the first message from each streaming provider, can proceed.
	c.configChanges <- integration.ConfigChanges{}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

// start registers the component as an autodiscovery config provider and
// subscribes to the DEBUG remote-config product.
func (c *component) start(_ context.Context) error {
	c.ac.AddConfigProvider(c, false, 0)
	c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
	c.log.Infof("datasecurity: subscribed to RC product %q", data.ProductDebug)
	return nil
}

// String returns the name of the provider. All Config instances produced by
// this provider carry this value in their Provider field.
func (c *component) String() string {
	return names.DataSecurity
}

// GetConfigErrors is required by the ConfigProvider interface. This provider
// does not track per-resource errors; RC apply errors are reported via the
// applyStatus callback in onUpdate.
func (c *component) GetConfigErrors() map[string]provtypes.ErrorMsgSet {
	return map[string]provtypes.ErrorMsgSet{}
}

// Stream returns the channel of config changes and closes it when ctx is
// cancelled so the autodiscovery config poller can observe teardown.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		c.closeMu.Lock()
		defer c.closeMu.Unlock()
		if c.closed {
			return
		}
		c.closed = true
		close(c.configChanges)
	}()
	return c.configChanges
}

// sendChanges delivers changes to autodiscovery, unless the provider has been
// torn down. Empty changes are dropped.
func (c *component) sendChanges(changes integration.ConfigChanges) {
	if changes.IsEmpty() {
		return
	}
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- changes
}
