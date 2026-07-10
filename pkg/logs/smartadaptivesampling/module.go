// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Anomaly detection is unavailable in IoT builds.

//go:build python

package smartadaptivesampling

import (
	"context"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"go.uber.org/fx"
)

const smartSeverityProfilesEnabledConfigKey = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"

// startReader registers the observer severity reader when enabled.
func startReader(observerComp observerdef.Component, log logcomp.Component) (func(), error) {
	if !pkgconfigsetup.Datadog().GetBool(smartSeverityProfilesEnabledConfigKey) {
		return nil, nil
	}

	if observerComp == nil {
		return nil, nil
	}

	sub, err := observerComp.SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration{})
	if err != nil {
		return nil, err
	}

	SetReader(sub.Reader)
	log.Infof("registered dynamic adaptive-sampling severity reader")
	return sub.Unsubscribe, nil
}

// moduleParams contains the module's FX dependencies.
type moduleParams struct {
	fx.In

	Lc       fx.Lifecycle
	Observer observerdef.Component `optional:"true"`
	Log      logcomp.Component
}

// Module registers severity reader lifecycle hooks.
func Module() fx.Option {
	return fx.Invoke(func(params moduleParams) {
		var unsubscribe func()
		params.Lc.Append(fx.Hook{
			OnStart: func(_ context.Context) error {
				sub, err := startReader(params.Observer, params.Log)
				if err != nil {
					return err
				}
				unsubscribe = sub
				return nil
			},
			OnStop: func(_ context.Context) error {
				if unsubscribe != nil {
					unsubscribe()
					unsubscribe = nil
				}
				SetReader(nil)
				return nil
			},
		})
	})
}
