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
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestHandleProvider(t *testing.T) {
	wg := sync.WaitGroup{}

	provider := func(context.Context) time.Duration {
		wg.Done()
		return 1 * time.Minute // Long timeout to block
	}

	wg.Add(1)

	r := createRunner(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Supply(NewProvider(provider)),
		))

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
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Supply(NewProvider(provider)),
		))

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
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Supply(NewProvider(provider)),
		))

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
		config.MockModule(),
		Module(),
		// Supplying our provider by using the helper function
		fx.Supply(NewProvider(provider)),
	)

	ctx := context.Background()
	lc.Start(ctx)

	// either the provider call wg.Done() or the test will fail as a timeout
	wg.Wait()

	assert.NoError(t, lc.Stop(ctx))
}
