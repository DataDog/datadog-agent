// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The `python` build tag is used here as a proxy for "full agent, not IoT agent".
// Anomaly detection (and therefore the severity signal this module bridges to
// the log sampler) is unavailable without it.
// See module_noop.go for the stub used in IoT agent builds.

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

// startReader subscribes to the observer's severity reader and registers it
// as the active reader (see SetReader), when smart severity profiles are
// enabled and an observer is present. It returns a nil unsubscribe func
// (and nil error) when either condition doesn't hold; callers must
// nil-check before invoking the returned func.
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

// moduleParams declares the module's fx dependencies. The observer component
// is marked optional as defense in depth: observer/fx.Module always provides
// it under the python build tag, but not every fx graph built with the tag is
// guaranteed to include observer/fx.Module (e.g. targeted tests).
type moduleParams struct {
	fx.In

	Lc       fx.Lifecycle
	Observer observerdef.Component `optional:"true"`
	Log      logcomp.Component
}

// Module wires the dynamic adaptive sampling reader into the app lifecycle once
// the observer component is ready.
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
