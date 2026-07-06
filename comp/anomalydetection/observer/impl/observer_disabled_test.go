// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestNewComponentReturnsDisabledStubWhenOff verifies the off-by-default fast
// path: with anomaly_detection.enabled=false and no recorder, NewComponent must
// return the zero-allocation disabledObserver stub rather than building the
// engine, storage, catalog, 1000-cap channel, and dispatch goroutine.
func TestNewComponentReturnsDisabledStubWhenOff(t *testing.T) {
	// anomaly_detection.enabled defaults to false; the mock carries that default.
	cfg := configmock.New(t)

	provides, err := NewComponent(Requires{
		Config:   cfg,
		Recorder: option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, ok := provides.Comp.(*disabledObserver)
	require.Truef(t, ok, "expected *disabledObserver when anomaly detection is disabled, got %T", provides.Comp)
}

type captureWarnLogComponent struct {
	noopLogComponent
	warnings []string
}

func (l *captureWarnLogComponent) Warnf(format string, args ...interface{}) error {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
	return nil
}

var _ logdef.Component = (*captureWarnLogComponent)(nil)

func TestNewComponentSmartSeverityProfilesForceEnableAnalysisAndScorer(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  enabled: false
  anomaly_scorer:
    enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`)

	lc := &testLifecycle{}
	log := &captureWarnLogComponent{}
	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config:    cfg,
		Log:       log,
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

	require.Len(t, log.warnings, 2)
	require.True(t, strings.Contains(log.warnings[0], anomalyDetectionEnabledConfigKey))
	require.True(t, strings.Contains(log.warnings[1], anomalyScorerEnabledConfigKey))
}
