// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package exampletaskmanagerimpl implements the example task manager component.
package exampletaskmanagerimpl

import (
	"context"
	"runtime"
	"sync"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	exampletaskmanager "github.com/DataDog/datadog-agent/comp/exampletaskmanager/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

// Requires defines the dependencies of the example task manager component.
type Requires struct {
	Lc       compdef.Lifecycle
	Log      log.Component
	Config   config.Component
	RcClient rcclient.Component
	Ac       autodiscovery.Component
}

// Provides defines the output of the example task manager component.
type Provides struct {
	Comp exampletaskmanager.Component
}

type component struct {
	log           log.Component
	enabled       bool
	rcclient      rcclient.Component
	ac            autodiscovery.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// NewComponent creates a new example task manager component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		enabled:       reqs.Config.GetBool("shared_library_check.enabled"),
		rcclient:      reqs.RcClient,
		ac:            reqs.Ac,
		configChanges: make(chan integration.ConfigChanges, 10),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

func (c *component) String() string {
	return names.ExampleTaskManager
}

func (c *component) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
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

		c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
		c.log.Infof("exampletaskmanager: subscribed to RC product %q", data.ProductDebug)
		<-ctx.Done()
	}()

	return c.configChanges
}

func (c *component) sendChanges(changes integration.ConfigChanges) {
	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- changes
}

func (c *component) start(_ context.Context) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if !c.enabled {
		c.log.Info("exampletaskmanager: shared_library_check.enabled is false, not registering config provider")
		return nil
	}
	c.ac.AddConfigProvider(c, false, 0)
	c.log.Info("exampletaskmanager: registered autodiscovery config provider")
	return nil
}
