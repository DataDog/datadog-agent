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
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	doqueryactions "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Requires defines the dependencies for the Data Observability query actions component
type Requires struct {
	Lc       compdef.Lifecycle
	Config   config.Component
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
	enabled         bool
	activeConfigs   map[string]integration.Config
	activeConfigsMu sync.Mutex
}

// NewComponent creates a new Data Observability query actions component
func NewComponent(reqs Requires) (Provides, error) {
	enabled := reqs.Config.GetBool("data_observability.query_actions.enabled")

	c := &component{
		log:           reqs.Log,
		ac:            reqs.Ac,
		rcclient:      reqs.RcClient,
		enabled:       enabled,
		activeConfigs: make(map[string]integration.Config),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

func (c *component) start(_ context.Context) error {
	if !c.enabled {
		c.log.Info("Data Observability query actions component disabled (data_observability.query_actions.enabled)")
		return nil
	}
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
// Assumption: the file config provider runs before this one in LoadAndRun, so the postgres check
// config is already in activeConfigs when our ticker first fires.
//
// RC configs are declarative snapshots so only the latest matters. The RC callback writes to
// outCh with replace semantics (non-blocking): if autodiscovery hasn't consumed the previous
// update yet, it is replaced with the latest one rather than blocking the rcclient goroutine.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)
	// Unblock autodiscovery's LoadAndRun — it blocks on <-ch until the first message arrives.
	outCh <- integration.ConfigChanges{}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.hasPostgresIntegration() {
					c.rcclient.Subscribe(data.ProductDOQueryActions, func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
						changes := c.onRCUpdate(updates, applyStatus)
						if changes.IsEmpty() {
							return
						}
						// Non-blocking replace: if the buffer is full (autodiscovery hasn't read yet),
						// drain it and write the latest. The drain is non-blocking because config_poller
						// may have already read the buffer between the default branch firing and the drain.
						// After the drain (or if already empty), the buffer is guaranteed free for our send.
						select {
						case outCh <- changes:
						default:
							select {
							case <-outCh:
							default:
							}
							outCh <- changes
						}
					})
					c.log.Info("Subscribed to RC DO_QUERY_ACTIONS product for Data Observability query actions")
					<-ctx.Done()
					return
				}
			}
		}
	}()

	return outCh
}

// hasPostgresIntegration checks if any postgres integration is configured in autodiscovery
func (c *component) hasPostgresIntegration() bool {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if isPostgresIntegration(cfg.Name) {
			return true
		}
	}
	return false
}
