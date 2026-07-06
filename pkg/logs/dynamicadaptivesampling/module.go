// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dynamicadaptivesampling

import (
	"context"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.uber.org/fx"
)

const (
	noActiveAnomalyScorerError            = "no active anomaly scorer"
	smartSeverityProfilesEnabledConfigKey = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
)

// Module wires the dynamic adaptive sampling reader into the app lifecycle once
// the observer component is ready.
func Module() fx.Option {
	return fx.Invoke(func(lc fx.Lifecycle, observerOpt option.Option[observerdef.Component], log logcomp.Component) {
		var unsubscribe func()
		lc.Append(fx.Hook{
			OnStart: func(_ context.Context) error {
				if !pkgconfigsetup.Datadog().GetBool(smartSeverityProfilesEnabledConfigKey) {
					return nil
				}

				observerComp, ok := observerOpt.Get()
				if !ok {
					return nil
				}

				sub, err := observerComp.SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration{})
				if err != nil {
					if err.Error() == noActiveAnomalyScorerError {
						log.Infof("dynamic adaptive-sampling severity reader not registered: %v", err)
						return nil
					}
					return err
				}

				SetReader(sub.Reader)
				unsubscribe = sub.Unsubscribe
				log.Infof("registered dynamic adaptive-sampling severity reader")
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
