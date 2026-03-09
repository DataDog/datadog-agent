// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package queryactionsimpl implements the DO query actions component
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
	compdef "github.com/DataDog/datadog-agent/comp/def"
	doqueryactions "github.com/DataDog/datadog-agent/comp/do/queryactions/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

// Requires defines the dependencies for the DO query actions component
type Requires struct {
	Lc       compdef.Lifecycle
	Config   config.Component
	Log      log.Component
	RcClient rcclient.Component
	Ac       autodiscovery.Component
}

// Provides defines the output of the DO query actions component
type Provides struct {
	Comp doqueryactions.Component
}

// component implements the DO query actions component
type component struct {
	log             log.Component
	ac              autodiscovery.Component
	rcclient        rcclient.Component
	enabled         bool
	configChanges   chan integration.ConfigChanges
	closeMu         sync.RWMutex
	closed          bool
	activeConfigs   map[string]integration.Config
	activeConfigsMu sync.Mutex
	stopCancel      context.CancelFunc
}

// NewComponent creates a new DO query actions component
func NewComponent(reqs Requires) (Provides, error) {
	enabled := reqs.Config.GetBool("data_observability.query_actions.enabled")

	c := &component{
		log:           reqs.Log,
		ac:            reqs.Ac,
		rcclient:      reqs.RcClient,
		enabled:       enabled,
		configChanges: make(chan integration.ConfigChanges, 10),
		activeConfigs: make(map[string]integration.Config),
	}

	// Send an empty ConfigChanges immediately so autodiscovery's Stream() reader unblocks
	// and begins listening, avoiding a deadlock where autodiscovery waits for initial output
	// before the component starts subscribing to RC.
	c.configChanges <- integration.ConfigChanges{}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return Provides{Comp: c}, nil
}

func (c *component) start(_ context.Context) error {
	if !c.enabled {
		c.log.Info("DO query actions component disabled by feature flag (data_observability.query_actions.enabled)")
		return nil
	}
	c.ac.AddConfigProvider(c, false, 0)
	ctx, cancel := context.WithCancel(context.Background())
	c.stopCancel = cancel
	go c.manageSubscriptionToRC(ctx)
	c.log.Info("DO query actions component started")
	return nil
}

func (c *component) stop(_ context.Context) error {
	if c.stopCancel != nil {
		c.stopCancel()
	}
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.configChanges)
	}
	c.log.Info("DO query actions component stopped")
	return nil
}

// manageSubscriptionToRC polls autodiscovery every 10 seconds until a postgres integration
// is detected, then subscribes to the RC DO_QUERY_ACTIONS product exactly once and exits.
// The goroutine exits immediately when ctx is cancelled (via stop()).
func (c *component) manageSubscriptionToRC(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if c.hasPostgresIntegration() {
				c.rcclient.Subscribe(data.ProductDOQueryActions, c.onRCUpdate)
				c.log.Info("Subscribed to RC DO_QUERY_ACTIONS product for DO query actions")
				return
			}
		}
	}
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

// String returns the name of the provider
func (c *component) String() string {
	return names.DOQueryActions
}

// GetConfigErrors is required by the ConfigProvider interface. This provider does not track
// per-resource errors; RC apply errors are reported via the applyStatus callback in onRCUpdate.
func (c *component) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream returns the shared configChanges channel and arranges for it to be closed when ctx
// is done. The channel is also closed by stop(); the closed flag and closeMu guard against
// double-close. Callers must not close the returned channel themselves.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		c.closeMu.Lock()
		defer c.closeMu.Unlock()
		if !c.closed {
			c.closed = true
			close(c.configChanges)
		}
	}()
	return c.configChanges
}
