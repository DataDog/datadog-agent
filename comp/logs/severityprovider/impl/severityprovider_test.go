// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && python

package severityproviderimpl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityeventsimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/impl"
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
	config            severityeventsdef.SeverityEventsConfiguration
	dispatcher        *severityeventsimpl.Dispatcher
	scopedConfig      severityeventsdef.SeverityEventsConfiguration
	scopedListener    severityeventsdef.ScopedSeverityEventListener
	scopedInitial     []severityeventsdef.ScopedSeverityEvent
}

func (f *fakeObserverComponent) GetHandle(string) observerdef.Handle { return nil }
func (f *fakeObserverComponent) DumpMetrics(string) error            { return nil }
func (f *fakeObserverComponent) SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	if f.err != nil {
		return severityeventsdef.SeverityEventsSubscription{}, f.err
	}
	f.dispatcher = severityeventsimpl.NewDispatcher(cfg, listener)
	return severityeventsdef.SeverityEventsSubscription{
		Dispatcher: f.dispatcher,
		Unsubscribe: func() {
			f.unsubscribeCalled = true
		},
	}, nil
}

func (f *fakeObserverComponent) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	f.config = cfg
	if f.err != nil {
		return severityeventsdef.SeverityEventsReaderSubscription{}, f.err
	}
	if f.sub.Reader == nil {
		return severityeventsimpl.NewSeverityReader(f, cfg)
	}
	return f.sub, nil
}

func (f *fakeObserverComponent) SubscribeScopedSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.ScopedSeverityEventListener) (severityeventsdef.ScopedSeverityEventsSubscription, error) {
	f.scopedConfig = cfg
	if f.err != nil {
		return severityeventsdef.ScopedSeverityEventsSubscription{}, f.err
	}
	f.scopedListener = listener
	for _, evt := range f.scopedInitial {
		listener.OnScopedSeverityTransition(evt)
	}
	return severityeventsdef.ScopedSeverityEventsSubscription{
		Unsubscribe: func() {
			f.unsubscribeCalled = true
		},
	}, nil
}

func (f *fakeObserverComponent) advance(sec int64, level severityeventsdef.SeverityLevel) {
	f.dispatcher.Advance(sec, level)
}

var _ observerdef.Component = (*fakeObserverComponent)(nil)

func newComponent(t *testing.T, enabled bool, cooldown time.Duration, observer option.Option[observerdef.Component]) (*component, *compdef.TestLifecycle) {
	t.Helper()
	lifecycle := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{
		Lifecycle: lifecycle,
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey: enabled,
			smartSeverityProfilesCooldownConfigKey:                       cooldown,
		}),
		Observer: observer,
		Log:      noopLogComponent{},
	})
	require.NoError(t, err)
	return provides.Comp.(*component), lifecycle
}

func TestLifecycleDisabled(t *testing.T) {
	comp, lifecycle := newComponent(t, false, 0, option.New[observerdef.Component](&fakeObserverComponent{}))
	require.NoError(t, lifecycle.Start(context.Background()))

	level, ok := comp.Current()
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestLifecycleNoObserverProvided(t *testing.T) {
	comp, lifecycle := newComponent(t, true, 0, option.None[observerdef.Component]())
	require.NoError(t, lifecycle.Start(context.Background()))

	level, ok := comp.Current()
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestLifecyclePropagatesSubscriptionError(t *testing.T) {
	comp, lifecycle := newComponent(t, true, 0, option.New[observerdef.Component](&fakeObserverComponent{err: assert.AnError}))
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
	comp, lifecycle := newComponent(t, true, 500*time.Millisecond, option.New[observerdef.Component](observer))

	require.NoError(t, lifecycle.Start(context.Background()))
	assert.Equal(t, int64(1), observer.config.CooldownSecs)
	level, ok := comp.Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	require.NoError(t, lifecycle.Stop(context.Background()))
	assert.True(t, observer.unsubscribeCalled)
	_, ok = comp.Current()
	assert.False(t, ok)
}

func newScopedComponent(t *testing.T, enabled bool, cooldown time.Duration, observer option.Option[observerdef.Component]) (*component, *compdef.TestLifecycle) {
	t.Helper()
	lifecycle := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{
		Lifecycle: lifecycle,
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey: enabled,
			anomalydetectionconfig.AnomalyScorerScopedEnabledConfigKey:   true,
			smartSeverityProfilesCooldownConfigKey:                       cooldown,
		}),
		Observer: observer,
		Log:      noopLogComponent{},
	})
	require.NoError(t, err)
	return provides.Comp.(*component), lifecycle
}

func TestNewComponentUsesGlobalWhenScopedDisabled(t *testing.T) {
	obs := &fakeObserverComponent{}
	comp, lifecycle := newComponent(t, true, 0, option.New[observerdef.Component](obs))
	require.NoError(t, lifecycle.Start(context.Background()))
	assert.NotNil(t, obs.dispatcher)
	assert.Nil(t, obs.scopedListener)
	assert.Nil(t, comp.scoped.Load())
}

func TestLifecycleReaderHonorsSmartSeverityProfilesCooldown(t *testing.T) {
	observer := &fakeObserverComponent{}
	comp, lifecycle := newComponent(t, true, 10*time.Second, option.New[observerdef.Component](observer))

	require.NoError(t, lifecycle.Start(context.Background()))
	require.NotNil(t, observer.dispatcher)
	assert.Equal(t, int64(10), observer.config.CooldownSecs)

	observer.advance(100, severityeventsdef.SeverityHigh)
	level, ok := comp.Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	observer.advance(101, severityeventsdef.SeverityLow)
	level, ok = comp.Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level, "cooldown must delay the reader's de-escalation")

	observer.advance(110, severityeventsdef.SeverityLow)
	level, ok = comp.Current()
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestScopedDisabledFallsBackToGlobal(t *testing.T) {
	reader := &fakeReader{level: severityeventsdef.SeverityMedium}
	observer := &fakeObserverComponent{
		sub: severityeventsdef.SeverityEventsReaderSubscription{Reader: reader, Unsubscribe: func() {}},
	}
	comp, lifecycle := newComponent(t, true, 0, option.New[observerdef.Component](observer))
	require.NoError(t, lifecycle.Start(context.Background()))

	called := false
	level, ok := comp.CurrentForScope(func() severityeventsdef.ScopeKey {
		called = true
		return severityeventsdef.ScopeKeyForService("checkout")
	})
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityMedium, level)
	assert.False(t, called)
}

func TestScopedLifecycleBootstrapAndErrorCleanup(t *testing.T) {
	checkout := severityeventsdef.ScopeKeyForService("checkout")
	billing := severityeventsdef.ScopeKeyForService("billing")

	obs := &fakeObserverComponent{
		scopedInitial: []severityeventsdef.ScopedSeverityEvent{
			{ScopeKey: checkout, Event: severityeventsdef.SeverityEvent{ToLevel: severityeventsdef.SeverityHigh}},
		},
	}
	comp, lifecycle := newScopedComponent(t, true, 1500*time.Millisecond, option.New[observerdef.Component](obs))
	require.NoError(t, lifecycle.Start(context.Background()))
	assert.Equal(t, int64(2), obs.scopedConfig.CooldownSecs)

	level, ok := comp.CurrentForScope(func() severityeventsdef.ScopeKey { return checkout })
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return billing })
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)

	errObs := &fakeObserverComponent{err: assert.AnError}
	errComp, errLifecycle := newScopedComponent(t, true, 0, option.New[observerdef.Component](errObs))
	assert.ErrorIs(t, errLifecycle.Start(context.Background()), assert.AnError)
	_, ok = errComp.CurrentForScope(func() severityeventsdef.ScopeKey { return checkout })
	assert.False(t, ok)
}

func TestScopedTransitionsIsolationAndStop(t *testing.T) {
	checkout := severityeventsdef.ScopeKeyForService("checkout")
	billing := severityeventsdef.ScopeKeyForService("billing")
	obs := &fakeObserverComponent{}
	comp, lifecycle := newScopedComponent(t, true, 0, option.New[observerdef.Component](obs))
	require.NoError(t, lifecycle.Start(context.Background()))

	// Steady state skips getScope completely.
	getScopeCalled := false
	level, ok := comp.CurrentForScope(func() severityeventsdef.ScopeKey {
		getScopeCalled = true
		return checkout
	})
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
	assert.False(t, getScopeCalled)

	// Escalate checkout; billing and unscoped (key=0) stay Low.
	obs.scopedListener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
		ScopeKey: checkout,
		Event:    severityeventsdef.SeverityEvent{ToLevel: severityeventsdef.SeverityMedium},
	})
	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return checkout })
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityMedium, level)

	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return billing })
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)

	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return 0 })
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)

	// De-escalate checkout back to Low deletes the key and restores the fast path.
	obs.scopedListener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
		ScopeKey: checkout,
		Event:    severityeventsdef.SeverityEvent{ToLevel: severityeventsdef.SeverityLow},
	})
	getScopeCalled = false
	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey {
		getScopeCalled = true
		return checkout
	})
	require.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
	assert.False(t, getScopeCalled)

	// Stop unsubscribes, resets availability, and ignores late callbacks.
	require.NoError(t, lifecycle.Stop(context.Background()))
	assert.True(t, obs.unsubscribeCalled)
	obs.scopedListener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
		ScopeKey: checkout,
		Event:    severityeventsdef.SeverityEvent{ToLevel: severityeventsdef.SeverityHigh},
	})
	level, ok = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return checkout })
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestScopedConcurrentReadsAndWrites(t *testing.T) {
	obs := &fakeObserverComponent{}
	comp, lifecycle := newScopedComponent(t, true, 0, option.New[observerdef.Component](obs))
	require.NoError(t, lifecycle.Start(context.Background()))
	defer func() { require.NoError(t, lifecycle.Stop(context.Background())) }()

	keys := []severityeventsdef.ScopeKey{
		severityeventsdef.ScopeKeyForService("svc-a"),
		severityeventsdef.ScopeKeyForService("svc-b"),
		severityeventsdef.ScopeKeyForService("svc-c"),
	}

	var wg sync.WaitGroup
	for readerIdx := range 4 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			target := keys[idx%len(keys)]
			for range 500 {
				_, _ = comp.CurrentForScope(func() severityeventsdef.ScopeKey { return target })
			}
		}(readerIdx)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for iter := range 200 {
			lvl := severityeventsdef.SeverityLevel(iter % int(severityeventsdef.NumSeverityLevels))
			obs.scopedListener.OnScopedSeverityTransition(severityeventsdef.ScopedSeverityEvent{
				ScopeKey: keys[iter%len(keys)],
				Event:    severityeventsdef.SeverityEvent{ToLevel: lvl},
			})
		}
	}()
	wg.Wait()
}

func BenchmarkCurrentForScopeSteadyState(b *testing.B) {
	comp := &component{}
	comp.scoped.Store(&scopedState{})
	key := severityeventsdef.ScopeKeyForService("checkout")
	getScope := func() severityeventsdef.ScopeKey { return key }

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = comp.CurrentForScope(getScope)
	}
}

func BenchmarkCurrentForScopeActive(b *testing.B) {
	comp := &component{}
	comp.scoped.Store(&scopedState{
		elevated: map[severityeventsdef.ScopeKey]severityeventsdef.SeverityLevel{
			severityeventsdef.ScopeKeyForService("svc-a"): severityeventsdef.SeverityMedium,
			severityeventsdef.ScopeKeyForService("svc-b"): severityeventsdef.SeverityHigh,
			severityeventsdef.ScopeKeyForService("svc-c"): severityeventsdef.SeverityHigh,
		},
	})
	key := severityeventsdef.ScopeKeyForService("svc-b")
	getScope := func() severityeventsdef.ScopeKey { return key }

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = comp.CurrentForScope(getScope)
	}
}
