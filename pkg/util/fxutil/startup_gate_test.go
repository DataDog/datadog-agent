// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fxutil

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type gateRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *gateRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *gateRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type recordingGate struct{ recorder *gateRecorder }

func (g *recordingGate) Wait(context.Context) error { g.recorder.add("wait"); return nil }
func (g *recordingGate) MarkActive() error          { g.recorder.add("active"); return nil }
func (g *recordingGate) Close() error               { g.recorder.add("close"); return nil }

type recordingComponent struct{}

func TestOneShotWithStartupGateOrdering(t *testing.T) {
	recorder := &gateRecorder{}
	gate := &recordingGate{recorder: recorder}

	err := OneShotWithStartupGate[*recordingGate](
		func(_ *recordingComponent) { recorder.add("run") },
		fx.Supply(gate),
		fx.Provide(func(lc fx.Lifecycle) *recordingComponent {
			recorder.add("construct")
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error { recorder.add("start"); return nil },
				OnStop:  func(context.Context) error { recorder.add("stop"); return nil },
			})
			return &recordingComponent{}
		}),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"construct", "wait", "start", "run", "stop", "close"}, recorder.snapshot())
}

func TestRunWithStartupGateOrdering(t *testing.T) {
	recorder := &gateRecorder{}
	gate := &recordingGate{recorder: recorder}

	err := RunWithStartupGate[*recordingGate](
		fx.Supply(gate),
		fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					recorder.add("start")
					return shutdowner.Shutdown()
				},
				OnStop: func(context.Context) error { recorder.add("stop"); return nil },
			})
		}),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"wait", "start", "active", "stop", "close"}, recorder.snapshot())
}

func TestOneShotWithStartupGateKeepsOwnershipWhenStopFails(t *testing.T) {
	recorder := &gateRecorder{}
	gate := &recordingGate{recorder: recorder}
	stopErr := errors.New("stop failed")

	err := OneShotWithStartupGate[*recordingGate](
		func(_ *recordingComponent) { recorder.add("run") },
		fx.Supply(gate),
		fx.Provide(func(lc fx.Lifecycle) *recordingComponent {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error { recorder.add("start"); return nil },
				OnStop:  func(context.Context) error { recorder.add("stop"); return stopErr },
			})
			return &recordingComponent{}
		}),
	)
	require.ErrorIs(t, err, stopErr)
	require.Equal(t, []string{"wait", "start", "run", "stop"}, recorder.snapshot())
}

func TestRunWithStartupGateKeepsOwnershipWhenStopFails(t *testing.T) {
	recorder := &gateRecorder{}
	gate := &recordingGate{recorder: recorder}
	stopErr := errors.New("stop failed")

	err := RunWithStartupGate[*recordingGate](
		fx.Supply(gate),
		fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error { return shutdowner.Shutdown() },
				OnStop:  func(context.Context) error { recorder.add("stop"); return stopErr },
			})
		}),
	)
	require.ErrorIs(t, err, stopErr)
	require.Equal(t, []string{"wait", "active", "stop"}, recorder.snapshot())
}
