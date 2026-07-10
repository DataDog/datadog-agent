// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

// Package smartadaptivesamplingimpl implements the smart adaptive sampling component.
package smartadaptivesamplingimpl

import (
	"context"
	"sync/atomic"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	smartadaptivesampling "github.com/DataDog/datadog-agent/comp/logs/smartadaptivesampling/def"
)

// Requires defines the smart adaptive sampling component dependencies.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Observer  observerdef.Component
	Log       log.Component
}

// Provides defines the smart adaptive sampling component output.
type Provides struct {
	Comp smartadaptivesampling.Component
}

type readerState struct {
	reader severityeventsdef.Reader
}

type component struct {
	reader atomic.Pointer[readerState]
}

// NewComponent creates the smart adaptive sampling component.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &component{}
	var unsubscribe func()

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			if !anomalydetectionconfig.SmartSeverityProfilesEnabled(reqs.Config) {
				return nil
			}

			subscription, err := reqs.Observer.SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration{})
			if err != nil {
				return err
			}

			comp.reader.Store(&readerState{reader: subscription.Reader})
			unsubscribe = subscription.Unsubscribe
			reqs.Log.Debugf("registered smart adaptive-sampling severity reader")
			return nil
		},
		OnStop: func(_ context.Context) error {
			if unsubscribe != nil {
				unsubscribe()
				unsubscribe = nil
			}
			comp.reader.Store(nil)
			return nil
		},
	})

	return Provides{Comp: comp}, nil
}

// Current returns the current severity level, if available.
func (c *component) Current() (severityeventsdef.SeverityLevel, bool) {
	if state := c.reader.Load(); state != nil {
		return state.reader.GetSeverity(), true
	}
	return severityeventsdef.SeverityLow, false
}

var _ smartadaptivesampling.Component = (*component)(nil)
