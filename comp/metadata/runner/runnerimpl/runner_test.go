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

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
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
			logimpl.MockModule(),
			config.MockModule(),
			fx.Supply(NewProvider(provider)),
		))

	r.start()
	// either the provider call wg.Done() or the test will fail as a timeout
	wg.Wait()
	assert.NoError(t, r.stop())
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
		logimpl.MockModule(),
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
