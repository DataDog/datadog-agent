// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurityimpl implements the data security component.
package datasecurityimpl

import (
	"context"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	datasecurity "github.com/DataDog/datadog-agent/comp/datasecurity/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

// Requires defines the dependencies of the data security component.
type Requires struct {
	Lc            compdef.Lifecycle
	Log           log.Component
	Config        config.Component
	RcClient      rcclient.Component
	Ac            autodiscovery.Component
	EventPlatform eventplatform.Component
}

// Provides defines the output of the data security component.
type Provides struct {
	Comp datasecurity.Component
}

// component implements the data security component.
type component struct {
	log         log.Component
	enabled     bool
	rcclient    rcclient.Component
	ac          autodiscovery.Component
	epforwarder eventplatform.Component
}

// NewComponent creates a new data security component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:         reqs.Log,
		enabled:     reqs.Config.GetBool("data_security.enabled"),
		rcclient:    reqs.RcClient,
		ac:          reqs.Ac,
		epforwarder: reqs.EventPlatform,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

// start subscribes to the DEBUG remote-config product, unless the component is
// disabled via data_security.enabled (the default).
func (c *component) start(_ context.Context) error {
	if !c.enabled {
		c.log.Info("datasecurity: data_security.enabled is false, not subscribing to remote-config")
		return nil
	}
	c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
	c.log.Infof("datasecurity: subscribed to RC product %q", data.ProductDebug)
	return nil
}
