// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPreparationBarrier(t *testing.T) {
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var calls atomic.Int32
	ac := &AutoConfig{preparation: &preparation{initialize: func() {
		calls.Add(1)
		<-release
	}}}
	t.Cleanup(func() {
		unblock()
		ac.preparation.stop()
	})

	// Preloading is non-blocking and concurrent requests share one operation.
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { ac.Preload(t.Context()) })
	}
	wg.Wait()
	require.Contains(t, health.GetReady().Unhealthy, "ad-initialization")

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ac.LoadAndRun(canceled), context.Canceled)

	// Waiting directly exercises concurrent waiters without starting providers
	// multiple times (LoadAndRun's successful invocation remains single-shot).
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() { results <- ac.preparation.wait(t.Context()) })
	}
	unblock()
	wg.Wait()
	for range 10 {
		require.NoError(t, <-results)
	}
	require.NoError(t, ac.LoadAndRun(t.Context()))
	require.EqualValues(t, 1, calls.Load())
	require.NotContains(t, health.GetReady().Unhealthy, "ad-initialization")
}

func TestPreparationContexts(t *testing.T) {
	for _, preload := range []bool{false, true} {
		t.Run(map[bool]string{false: "lazy waiter", true: "preload deadline"}[preload], func(t *testing.T) {
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			ac := &AutoConfig{preparation: &preparation{initialize: func() { <-release }}}
			t.Cleanup(func() {
				unblock()
				ac.preparation.stop()
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if preload {
				ac.Preload(ctx)
			}
			cancel()
			require.ErrorIs(t, ac.LoadAndRun(ctx), context.Canceled)
			if preload {
				// A fresh caller still observes the original preload deadline.
				require.ErrorIs(t, ac.LoadAndRun(t.Context()), context.Canceled)
			} else {
				// A lazy caller's cancellation must not poison later callers.
				require.NoError(t, ac.preparation.startCtx.Err())
			}
			unblock()
			<-ac.preparation.done
			// Completed preparation wins over the expired preload context.
			require.NoError(t, ac.LoadAndRun(t.Context()))
		})
	}
}

func TestPreparationStopWithoutStarting(t *testing.T) {
	ac := &AutoConfig{preparation: &preparation{initialize: func() {
		t.Error("preparation started after shutdown")
	}}}
	ac.preparation.stop() // Must not block when preparation was never requested.
	ac.Preload(t.Context())
	require.ErrorContains(t, ac.LoadAndRun(t.Context()), "already stopped")
}

// Exercise the same decorator ordering as the core Agent, including child-module
// invokes. Preparation must begin before consumer hooks, and be joined by the
// real AutoConfig stop hook before dependency teardown, also on startup rollback.
func TestPreparationLifecycle(t *testing.T) {
	for _, failStart := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal shutdown", true: "startup rollback"}[failStart], func(t *testing.T) {
			reqs := preparationTestRequires(t)
			started := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			consumerStarted := make(chan struct{})
			var prepared atomic.Bool
			var stoppedAfterPreparation atomic.Bool
			failure := errors.New("consumer failed")
			type dependency struct{}
			app := fx.New(fx.NopLogger, fxutil.FxLifecycleAdapter(),
				fx.Provide(func(lc fx.Lifecycle) *dependency {
					lc.Append(fx.Hook{OnStart: func(context.Context) error { return nil }, OnStop: func(context.Context) error {
						stoppedAfterPreparation.Store(prepared.Load())
						return nil
					}})
					return &dependency{}
				}),
				fx.Provide(func(lc compdef.Lifecycle, _ *dependency) autodiscovery.Component {
					reqs.Lc = lc
					ac := NewComponent(reqs).Comp.(*AutoConfig)
					ac.preparation.initialize = func() {
						close(started)
						<-release
						prepared.Store(true)
					}
					return ac
				}),
				fx.Decorate(func(lc fx.Lifecycle, ac autodiscovery.Component) autodiscovery.Component {
					lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
						ac.Preload(ctx)
						return nil
					}})
					return ac
				}),
				fx.Module("consumer", fx.Invoke(func(lc fx.Lifecycle, _ autodiscovery.Component) {
					lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
						select {
						case <-started:
						case <-ctx.Done():
							return ctx.Err()
						}
						close(consumerStarted)
						if failStart {
							return failure
						}
						return nil
					}})
				})),
			)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				if err := app.Start(ctx); err != nil {
					finished <- err
					return
				}
				finished <- app.Stop(ctx)
			}()
			select {
			case <-consumerStarted:
			case <-ctx.Done():
				t.Fatal("consumer hook did not start")
			}
			unblock()
			select {
			case err := <-finished:
				if failStart {
					require.ErrorIs(t, err, failure)
				} else {
					require.NoError(t, err)
				}
			case <-ctx.Done():
				t.Fatal("lifecycle did not finish")
			}
			require.True(t, stoppedAfterPreparation.Load())
			require.NotContains(t, health.GetReady().Unhealthy, "ad-initialization")
		})
	}
}
