// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// The `python` build tag is used here as a proxy for "full agent, not IoT agent".
// See scorer_rate_limiter_noop.go for the stub used in IoT agent builds.

//go:build python

// Package anomalysampler bridges the anomaly scorer to the adaptive log sampler.
// When the scorer severity changes, a subscriber updates a process-wide multiplier
// that all AdaptiveSampler instances read on every Process call.
package anomalysampler

import (
	"context"

	"go.uber.org/fx"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module wires the scorer→sampler bridge into the fx graph. It must only be
// included in builds that also include observerfx.Module() (i.e. the full
// agent's run command) so that the observer optional is populated.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Invoke(registerScorerRateLimiter),
	)
}

type bridgeDeps struct {
	fx.In

	Lc       fx.Lifecycle
	Observer option.Option[observerdef.Component]
	Config   configComponent.Component
	Log      log.Component
}

func registerScorerRateLimiter(deps bridgeDeps) {
	if !deps.Config.GetBool("logs_config.experimental_adaptive_sampling.anomaly_severity.enabled") {
		return
	}
	obs, ok := deps.Observer.Get()
	if !ok {
		return
	}

	multipliers := [3]float64{
		deps.Config.GetFloat64("logs_config.experimental_adaptive_sampling.anomaly_severity.low_multiplier"),
		deps.Config.GetFloat64("logs_config.experimental_adaptive_sampling.anomaly_severity.medium_multiplier"),
		deps.Config.GetFloat64("logs_config.experimental_adaptive_sampling.anomaly_severity.high_multiplier"),
	}

	listener := &scorerRateLimiter{
		multipliers: multipliers,
		log:         deps.Log,
	}

	var unsub func()
	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			unsub = obs.SubscribeScorer(observerdef.AnomalyScorerConfiguration{
				Listener: listener,
			})
			deps.Log.Infof("[anomalysampler] subscribed to anomaly scorer (low=%.2f, medium=%.2f, high=%.2f)",
				multipliers[0], multipliers[1], multipliers[2])
			return nil
		},
		OnStop: func(_ context.Context) error {
			if unsub != nil {
				unsub()
			}
			return nil
		},
	})
}

// scorerRateLimiter implements observerdef.ScorerListener. It maps the incoming
// severity level to a configured multiplier and writes it to the shared
// adaptivesampler.Controller, which all AdaptiveSampler instances consult.
type scorerRateLimiter struct {
	// multipliers[SeverityLow/Medium/High]
	multipliers [3]float64
	log         log.Component
}

// OnSeverityTransition implements observerdef.ScorerListener.
func (r *scorerRateLimiter) OnSeverityTransition(evt observerdef.SeverityEvent) {
	level := int(evt.ToLevel)
	if level < 0 || level >= len(r.multipliers) {
		return
	}
	mult := r.multipliers[level]
	adaptivesampler.Shared().SetMultiplier(mult)
	r.log.Infof("[anomalysampler] severity → %s: adaptive sampler multiplier set to %.2f",
		severityName(evt.ToLevel), mult)
}

func severityName(l observerdef.SeverityLevel) string {
	switch l {
	case observerdef.SeverityLow:
		return "Low"
	case observerdef.SeverityMedium:
		return "Medium"
	case observerdef.SeverityHigh:
		return "High"
	default:
		return "Unknown"
	}
}
