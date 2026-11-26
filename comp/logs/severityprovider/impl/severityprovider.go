// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

// Package severityproviderimpl implements the severity provider component.
package severityproviderimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	severityprovider "github.com/DataDog/datadog-agent/comp/logs/severityprovider/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the severity provider component dependencies.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Observer  option.Option[observerdef.Component]
	Log       log.Component
}

// Provides defines the severity provider component output.
type Provides struct {
	Comp severityprovider.Component
}

type readerState struct {
	reader severityeventsdef.Reader
}

type scopedState struct {
	elevated map[severityeventsdef.ScopeKey]severityeventsdef.SeverityLevel
}

type component struct {
	reader atomic.Pointer[readerState]
	scoped atomic.Pointer[scopedState]
	mu     sync.Mutex
}

const scopedSeverityProfilesEnabled = false

const smartSeverityProfilesCooldownConfigKey = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.cooldown"

// smartSeverityProfilesSubscriptionConfig returns the reader configuration for
// smart severity profiles. CooldownSecs has whole-second granularity, so
// sub-second values are rounded rather than truncated to zero.
func smartSeverityProfilesSubscriptionConfig(cfg config.Component) severityeventsdef.SeverityEventsConfiguration {
	cooldown := cfg.GetDuration(smartSeverityProfilesCooldownConfigKey)
	return severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: int64(cooldown.Round(time.Second) / time.Second),
	}
}

// NewComponent creates the severity provider component.
func NewComponent(reqs Requires) (Provides, error) {
	return newComponentWithScoped(reqs, scopedSeverityProfilesEnabled)
}

func newComponentWithScoped(reqs Requires, scopedEnabled bool) (Provides, error) {
	comp := &component{}
	var unsubscribe func()

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			if !anomalydetectionconfig.SmartSeverityProfilesEnabled(reqs.Config) {
				return nil
			}

			observer, ok := reqs.Observer.Get()
			if !ok {
				reqs.Log.Warnf("[severityprovider] disabled: anomaly-detection observer is not running")
				return nil
			}

			if scopedEnabled {
				comp.mu.Lock()
				comp.scoped.Store(&scopedState{})
				comp.mu.Unlock()

				subscription, err := observer.SubscribeScopedSeverityEvents(smartSeverityProfilesSubscriptionConfig(reqs.Config), comp)
				if err != nil {
					comp.mu.Lock()
					comp.scoped.Store(nil)
					comp.mu.Unlock()
					return err
				}

				unsubscribe = subscription.Unsubscribe
				reqs.Log.Debugf("registered scoped smart adaptive-sampling severity listener")
				return nil
			}

			subscription, err := observer.SubscribeSeverityEventsReader(smartSeverityProfilesSubscriptionConfig(reqs.Config))
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
			comp.mu.Lock()
			comp.scoped.Store(nil)
			comp.mu.Unlock()
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

// OnScopedSeverityTransition updates the elevated COW snapshot for evt.ScopeKey.
func (c *component) OnScopedSeverityTransition(evt severityeventsdef.ScopedSeverityEvent) {
	if evt.ScopeKey == 0 {
		return
	}
	level := evt.Event.ToLevel
	if level > severityeventsdef.SeverityHigh {
		level = severityeventsdef.SeverityHigh
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	old := c.scoped.Load()
	if old == nil {
		return
	}

	prevLevel, exists := old.elevated[evt.ScopeKey]
	if level <= severityeventsdef.SeverityLow {
		if !exists {
			return
		}
	} else if exists && prevLevel == level {
		return
	}

	next := make(map[severityeventsdef.ScopeKey]severityeventsdef.SeverityLevel, len(old.elevated)+1)
	for key, val := range old.elevated {
		next[key] = val
	}
	if level <= severityeventsdef.SeverityLow {
		delete(next, evt.ScopeKey)
	} else {
		next[evt.ScopeKey] = level
	}
	c.scoped.Store(&scopedState{elevated: next})
}

// CurrentForScope returns the severity level for getScope, falling back to Current when scoped mode is inactive.
func (c *component) CurrentForScope(getScope func() severityeventsdef.ScopeKey) (severityeventsdef.SeverityLevel, bool) {
	state := c.scoped.Load()
	if state == nil {
		return c.Current()
	}
	if len(state.elevated) == 0 || getScope == nil {
		return severityeventsdef.SeverityLow, true
	}
	key := getScope()
	if key == 0 {
		return severityeventsdef.SeverityLow, true
	}
	if level, ok := state.elevated[key]; ok {
		return level, true
	}
	return severityeventsdef.SeverityLow, true
}

var (
	_ severityprovider.Component                    = (*component)(nil)
	_ severityeventsdef.ScopedSeverityEventListener = (*component)(nil)
)
