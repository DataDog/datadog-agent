// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurityimpl implements the data security component.
package datasecurityimpl

import (
	"context"
	"runtime"
	"sync"
	"time"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	datasecurity "github.com/DataDog/datadog-agent/comp/datasecurity/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies of the data security component.
type Requires struct {
	Lc            compdef.Lifecycle
	Log           log.Component
	Config        config.Component
	RcClient      rcclient.Component
	Ac            autodiscovery.Component
	SenderManager sender.SenderManager
	Tagger        tagger.Component
	FilterStore   workloadfilter.Component
	LogReceiver   option.Option[integrations.Component]
}

// Provides defines the output of the data security component.
type Provides struct {
	Comp datasecurity.Component
}

// component implements the data security component and acts as an autodiscovery
// config provider that schedules one-shot datasecurity shared-library checks.
type component struct {
	log           log.Component
	enabled       bool
	cfg           config.Component
	rcclient      rcclient.Component
	ac            autodiscovery.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// NewComponent creates a new data security component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		enabled:       reqs.Config.GetBool("data_security.enabled"),
		cfg:           reqs.Config,
		rcclient:      reqs.RcClient,
		ac:            reqs.Ac,
		configChanges: make(chan integration.ConfigChanges, 10),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			collectoraggregator.InitializeCheckContext(
				reqs.SenderManager,
				reqs.LogReceiver,
				reqs.Tagger,
				reqs.FilterStore,
			)
			return c.start(ctx)
		},
	})

	return Provides{Comp: c}, nil
}

// String returns the name of the autodiscovery config provider.
func (c *component) String() string {
	return names.DataSecurity
}

// GetConfigErrors returns a map of errors from the last Collect call.
func (c *component) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream starts sending configuration updates for the datasecurity integration.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	// Unblock autodiscovery's LoadAndRun — it blocks on <-ch until the first message arrives.
	c.configChanges <- integration.ConfigChanges{}

	go func() {
		defer func() {
			c.closeMutex.Lock()
			defer c.closeMutex.Unlock()
			if c.closed {
				return
			}
			c.closed = true
			close(c.configChanges)
		}()

		if !c.enabled || runtime.GOOS == "windows" {
			<-ctx.Done()
			return
		}

		if c.hasEligibleIntegration() {
			c.subscribeAndWait(ctx)
			return
		}

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.hasEligibleIntegration() {
					c.subscribeAndWait(ctx)
					return
				}
			}
		}
	}()

	return c.configChanges
}

func (c *component) subscribeAndWait(ctx context.Context) {
	c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
	c.log.Infof("datasecurity: subscribed to RC product %q", data.ProductDebug)
	<-ctx.Done()
}

func (c *component) sendChanges(changes integration.ConfigChanges) {
	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- changes
}

// start registers this component as an autodiscovery config provider.
func (c *component) start(_ context.Context) error {
	if runtime.GOOS == "windows" {
		// Shared-library checks used by datasecurity are not shipped on Windows.
		c.log.Warn("datasecurity is not supported on Windows; disabling")
		return nil
	}
	if !c.enabled {
		c.log.Info("datasecurity: data_security.enabled is false, not registering config provider")
		return nil
	}
	c.ac.AddConfigProvider(c, false, 0)
	c.log.Info("datasecurity: registered autodiscovery config provider")
	return nil
}
