// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestNewComponentReturnsDisabledStubWhenOff verifies the off-by-default fast
// path: with no active anomaly-detection gate and no recorder, NewComponent must
// return the zero-allocation disabledObserver stub rather than building the
// engine, storage, catalog, 1000-cap channel, and dispatch goroutine.
func TestNewComponentReturnsDisabledStubWhenOff(t *testing.T) {
	cfg := configmock.New(t)

	provides, err := NewComponent(Requires{
		Config:   cfg,
		Recorder: option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, ok := provides.Comp.(*disabledObserver)
	require.Truef(t, ok, "expected *disabledObserver when anomaly detection is disabled by default, got %T", provides.Comp)
}

func TestNewComponentSmartSeverityProfilesForceEnableObserverAndScorer(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`)

	lc := &testLifecycle{}
	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config:    cfg,
		Log:       &noopLogComponent{},
		Recorder:  option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, disabled := provides.Comp.(*disabledObserver)
	require.False(t, disabled, "smart severity profiles should force-enable anomaly detection")

	sub, err := provides.Comp.SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration{})
	require.NoError(t, err, "smart severity profiles should force-enable the anomaly scorer")
	if sub.Unsubscribe != nil {
		sub.Unsubscribe()
	}
}
