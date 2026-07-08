// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurityimpl implements the data security component.
package datasecurityimpl

import (
	"context"
	"runtime"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
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

// component implements the data security component.
type component struct {
	log           log.Component
	enabled       bool
	cfg           config.Component
	rcclient      rcclient.Component
	ac            autodiscovery.Component
	senderManager sender.SenderManager
}

// NewComponent creates a new data security component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		enabled:       reqs.Config.GetBool("data_security.enabled"),
		cfg:           reqs.Config,
		rcclient:      reqs.RcClient,
		ac:            reqs.Ac,
		senderManager: reqs.SenderManager,
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

// start subscribes to the DEBUG remote-config product, unless the component is
// disabled via data_security.enabled (the default).
func (c *component) start(_ context.Context) error {
	if runtime.GOOS == "windows" {
		// Shared-library checks used by datasecurity are not shipped on Windows.
		// Avoid starting subscriptions/runs that would inevitably fail at runtime.
		c.log.Warn("datasecurity is not supported on Windows; disabling")
		return nil
	}
	if !c.enabled {
		c.log.Info("datasecurity: data_security.enabled is false, not subscribing to remote-config")
		return nil
	}
	c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
	c.log.Infof("datasecurity: subscribed to RC product %q", data.ProductDebug)
	return nil
}
