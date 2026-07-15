// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && python

package smartadaptivesamplingimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type noopLogComponent struct{}

func (noopLogComponent) Trace(...interface{})                   {}
func (noopLogComponent) Tracef(string, ...interface{})          {}
func (noopLogComponent) Debug(...interface{})                   {}
func (noopLogComponent) Debugf(string, ...interface{})          {}
func (noopLogComponent) Info(...interface{})                    {}
func (noopLogComponent) Infof(string, ...interface{})           {}
func (noopLogComponent) Warn(...interface{}) error              { return nil }
func (noopLogComponent) Warnf(string, ...interface{}) error     { return nil }
func (noopLogComponent) Error(...interface{}) error             { return nil }
func (noopLogComponent) Errorf(string, ...interface{}) error    { return nil }
func (noopLogComponent) Critical(...interface{}) error          { return nil }
func (noopLogComponent) Criticalf(string, ...interface{}) error { return nil }
func (noopLogComponent) Flush()                                 {}

var _ log.Component = noopLogComponent{}

type fakeReader struct {
	level severityeventsdef.SeverityLevel
}

func (r *fakeReader) GetSeverity() severityeventsdef.SeverityLevel { return r.level }

type fakeObserverComponent struct {
	sub               severityeventsdef.SeverityEventsReaderSubscription
	err               error
	unsubscribeCalled bool
}

func (f *fakeObserverComponent) GetHandle(string) observerdef.Handle { return nil }
func (f *fakeObserverComponent) RecordSamplerDropped(string, string) {}
func (f *fakeObserverComponent) DumpMetrics(string) error            { return nil }
func (f *fakeObserverComponent) SubscribeSeverityEvents(severityeventsdef.SeverityEventsConfiguration, severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	return severityeventsdef.SeverityEventsSubscription{}, nil
}

func (f *fakeObserverComponent) SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	if f.err != nil {
		return severityeventsdef.SeverityEventsReaderSubscription{}, f.err
	}
	return f.sub, nil
}

var _ observerdef.Component = (*fakeObserverComponent)(nil)

func newComponent(t *testing.T, enabled bool, observer option.Option[observerdef.Component]) (*component, *compdef.TestLifecycle) {
	t.Helper()
	lifecycle := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{
		Lifecycle: lifecycle,
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey: enabled,
		}),
		Observer: observer,
		Log:      noopLogComponent{},
	})
	require.NoError(t, err)
	return provides.Comp.(*component), lifecycle
}

func TestLifecycleDisabled(t *testing.T) {
	comp, lifecycle := newComponent(t, false, option.New[observerdef.Component](&fakeObserverComponent{}))
	require.NoError(t, lifecycle.Start(context.Background()))

	level, ok := comp.Current()
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestLifecycleNoObserverProvided(t *testing.T) {
	comp, lifecycle := newComponent(t, true, option.None[observerdef.Component]())
	require.NoError(t, lifecycle.Start(context.Background()))

	level, ok := comp.Current()
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestLifecyclePropagatesSubscriptionError(t *testing.T) {
	comp, lifecycle := newComponent(t, true, option.New[observerdef.Component](&fakeObserverComponent{err: assert.AnError}))
	assert.ErrorIs(t, lifecycle.Start(context.Background()), assert.AnError)

	_, ok := comp.Current()
	assert.False(t, ok)
}

func TestLifecycleRegistersAndUnsubscribesReader(t *testing.T) {
	reader := &fakeReader{level: severityeventsdef.SeverityHigh}
	observer := &fakeObserverComponent{}
	observer.sub = severityeventsdef.SeverityEventsReaderSubscription{
		Reader:      reader,
		Unsubscribe: func() { observer.unsubscribeCalled = true },
	}
	comp, lifecycle := newComponent(t, true, option.New[observerdef.Component](observer))

	require.NoError(t, lifecycle.Start(context.Background()))
	level, ok := comp.Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	require.NoError(t, lifecycle.Stop(context.Background()))
	assert.True(t, observer.unsubscribeCalled)
	_, ok = comp.Current()
	assert.False(t, ok)
}
