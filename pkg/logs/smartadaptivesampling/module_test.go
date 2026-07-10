// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package smartadaptivesampling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// noopLogComponent satisfies logcomp.Component in tests.
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

var _ logcomp.Component = noopLogComponent{}

// fakeObserverComponent returns a configured reader subscription.
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

func TestStartReader_DisabledReturnsNoUnsubscribeAndNoError(t *testing.T) {
	configmock.New(t)
	resetForTest(t)

	comp := &fakeObserverComponent{}
	unsubscribe, err := startReader(comp, noopLogComponent{})

	require.NoError(t, err)
	assert.Nil(t, unsubscribe)
	_, ok := Current()
	assert.False(t, ok, "reader must not be registered when the feature is disabled")
}

func TestStartReader_SubscribeErrorIsPropagated(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set(anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey, true, pkgconfigmodel.SourceAgentRuntime)
	resetForTest(t)

	wantErr := assert.AnError
	comp := &fakeObserverComponent{err: wantErr}
	unsubscribe, err := startReader(comp, noopLogComponent{})

	assert.Equal(t, wantErr, err)
	assert.Nil(t, unsubscribe)
}

func TestStartReader_SuccessRegistersReaderAndReturnsUnsubscribe(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set(anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey, true, pkgconfigmodel.SourceAgentRuntime)
	resetForTest(t)

	fake := &fakeReader{level: severityeventsdef.SeverityHigh}
	comp := &fakeObserverComponent{}
	comp.sub = severityeventsdef.SeverityEventsReaderSubscription{
		Reader:      fake,
		Unsubscribe: func() { comp.unsubscribeCalled = true },
	}

	unsubscribe, err := startReader(comp, noopLogComponent{})

	require.NoError(t, err)
	require.NotNil(t, unsubscribe)
	level, ok := Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	unsubscribe()
	assert.True(t, comp.unsubscribeCalled)
}
