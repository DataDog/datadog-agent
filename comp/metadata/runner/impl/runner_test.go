// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runnerimpl implements a component to generate metadata payload at the right interval.
package runnerimpl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// testDeps is a subset of Requires that uses fx.In for testing createRunner directly.
type testDeps struct {
	fx.In

	Log    log.Component
	Config config.Component

	PrimaryProviders []runner.MetadataProvider `group:"metadata_provider_primary"`
	Providers        []runner.MetadataProvider `group:"metadata_provider"`
}

func makeRequires(_ *testing.T, deps testDeps) Requires {
	return Requires{
		Log:              deps.Log,
		Config:           deps.Config,
		PrimaryProviders: deps.PrimaryProviders,
		Providers:        deps.Providers,
	}
}

func TestHandleProvider(t *testing.T) {
	wg := sync.WaitGroup{}

	provider := func(context.Context) time.Duration {
		wg.Done()
		return 1 * time.Minute // Long timeout to block
	}

	wg.Add(1)

	r := createRunner(
		makeRequires(t, fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Supply(runner.NewProvider(provider)),
		)))

	r.start()
	// either the provider call wg.Done() or the test will fail as a timeout
	wg.Wait()
	assert.NoError(t, r.stop())
}

func TestHandleProviderShortTimeout(t *testing.T) {
	provider := func(context.Context) time.Duration {
		time.Sleep(1 * time.Minute) // Long timeout to block
		return 1 * time.Minute
	}

	r := createRunner(
		makeRequires(t, fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Supply(runner.NewProvider(provider)),
		)))

	r.config.Set("metadata_provider_stop_timeout", time.Duration(0), model.SourceFile)
	require.NoError(t, r.start())

	cerr := make(chan error)
	go func() {
		cerr <- r.stop()
	}()

	select {
	case err := <-cerr:
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		require.Fail(t, "timeout waiting for stop")
	}
}

func TestHandleProviderLongTimeout(t *testing.T) {
	provider := func(ctx context.Context) time.Duration {
		<-ctx.Done()
		return 1 * time.Minute
	}

	r := createRunner(
		makeRequires(t, fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Supply(runner.NewProvider(provider)),
		)))

	r.config.Set("metadata_provider1_stop_timeout", 1*time.Minute, model.SourceFile)
	require.NoError(t, r.start())

	cerr := make(chan error)
	go func() {
		cerr <- r.stop()
	}()

	select {
	case err := <-cerr:
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		require.Fail(t, "timeout waiting for stop")
	}
}

func TestRunnerCreation(t *testing.T) {
	wg := sync.WaitGroup{}

	provider := func(context.Context) time.Duration {
		wg.Done()
		return 1 * time.Minute // Long timeout to block
	}

	wg.Add(1)

	lc := fxtest.NewLifecycle(t)
	fxutil.Test[runner.Component](
		t,
		fx.Supply(lc),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fxutil.ProvideComponentConstructor(NewComponent),
		// Supplying our provider by using the helper function
		fx.Supply(runner.NewProvider(provider)),
	)

	ctx := context.Background()
	lc.Start(ctx)

	// either the provider call wg.Done() or the test will fail as a timeout
	wg.Wait()

	assert.NoError(t, lc.Stop(ctx))
}

func TestPrimaryProviderOrdering(t *testing.T) {
	newRunner := func(primary, secondary runner.MetadataProvider, timeout time.Duration) *runnerImpl {
		r := createRunner(makeRequires(t, fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Supply(runner.NewPrimaryProvider(primary)),
			fx.Supply(runner.NewProvider(secondary)),
		)))
		r.primaryWaitTimeout = timeout
		return r
	}

	t.Run("secondary waits for primary", func(t *testing.T) {
		release, secondaryRan := make(chan struct{}), make(chan struct{})
		r := newRunner(
			func(context.Context) time.Duration { <-release; return time.Minute },
			func(context.Context) time.Duration { close(secondaryRan); return time.Minute },
			5*time.Second,
		)
		require.NoError(t, r.start())
		select {
		case <-secondaryRan:
			require.Fail(t, "secondary ran before primary finished")
		case <-time.After(30 * time.Millisecond):
		}
		close(release)
		select {
		case <-secondaryRan:
		case <-time.After(time.Second):
			require.Fail(t, "secondary did not run after primary finished")
		}
		assert.NoError(t, r.stop())
	})

	t.Run("timeout fallback and stop", func(t *testing.T) {
		secondaryRan := make(chan struct{})
		r := newRunner(
			func(ctx context.Context) time.Duration { <-ctx.Done(); return time.Minute },
			func(context.Context) time.Duration { close(secondaryRan); return time.Minute },
			30*time.Millisecond,
		)
		require.NoError(t, r.start())
		select {
		case <-secondaryRan:
		case <-time.After(time.Second):
			require.Fail(t, "secondary did not start after primaryWaitTimeout")
		}
		assert.NoError(t, r.stop())
	})
}
