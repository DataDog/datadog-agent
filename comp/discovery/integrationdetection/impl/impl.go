// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrationdetectionimpl implements the integration detection Fx component.
package integrationdetectionimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	integrationdetectiondef "github.com/DataDog/datadog-agent/comp/discovery/integrationdetection/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/integrationdetection"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// enabledKey is the config key that gates the integration detection component.
const enabledKey = "discovery.integration_detection.enabled"

// Requires lists the Fx dependencies of the integration detection component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	AC        autodiscovery.Component
}

// Provides lists the Fx outputs of the integration detection component.
type Provides struct {
	Comp option.Option[integrationdetectiondef.Component]
}

type impl struct {
	detector *integrationdetection.Detector
	log      log.Component
}

// NewComponent creates the integration detection component.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool(enabledKey) {
		return Provides{Comp: option.None[integrationdetectiondef.Component]()}, nil
	}
	d := integrationdetection.NewDetector()
	c := &impl{detector: d, log: reqs.Log}
	// Copy AC out of reqs so the OnStart closure does not transitively retain
	// the full Requires struct (and all its dependencies) after NewComponent returns.
	ac := reqs.AC
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			// Registration happens here (not in the constructor) because the AD
			// controller waits for workloadmeta to finish initialising before it
			// begins dispatching events.
			c.log.Info("Starting integration detection component")
			d.Start(ac)
			return nil
		},
		OnStop: c.stop, // OnStop Fx hook; calls d.Stop() and logs
	})
	return Provides{Comp: option.New[integrationdetectiondef.Component](c)}, nil
}

func (c *impl) stop(_ context.Context) error {
	c.log.Info("Stopping integration detection component")
	c.detector.Stop()
	return nil
}

// EnabledIntegrations returns a live snapshot of enabled integrations.
func (c *impl) EnabledIntegrations() []integrationdetectiondef.EnabledIntegration {
	return c.detector.Snapshot()
}
