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

var orderedResult []int

type provider struct {
	wg    *sync.WaitGroup
	index int
}

func (p provider) Index() int {
	return p.index
}

func (p provider) SendInformation(_ context.Context) time.Duration {
	orderedResult = append(orderedResult, p.index)
	p.wg.Done()
	return 1 * time.Minute // Long timeout to block
}

func TestRunnerCreation(t *testing.T) {
	defer func() {
		// reset orderedResult
		orderedResult = []int{}
	}()

	waitGroup := &sync.WaitGroup{}

	provider := provider{
		wg:    waitGroup,
		index: 0,
	}

	waitGroup.Add(1)

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

	// either the provider SendInformation function is called or the test will fail as a timeout
	waitGroup.Wait()
	assert.Equal(t, []int{0}, orderedResult)
	assert.NoError(t, lc.Stop(ctx))
}

func TestOrderExcutionOfProviders(t *testing.T) {
	defer func() {
		// reset orderedResult
		orderedResult = []int{}
	}()

	waitGroup := &sync.WaitGroup{}

	provider0 := provider{
		wg:    waitGroup,
		index: 0,
	}

	provider1 := provider{
		wg:    waitGroup,
		index: 1,
	}

	provider2 := provider{
		wg:    waitGroup,
		index: 2,
	}

	waitGroup.Add(3)

	lc := fxtest.NewLifecycle(t)
	fxutil.Test[runner.Component](
		t,
		fx.Supply(lc),
		logimpl.MockModule(),
		config.MockModule(),
		Module(),
		// Supplying our provider by using the helper function
		fx.Supply(NewProvider(provider2)),
		fx.Supply(NewProvider(provider0)),
		fx.Supply(NewProvider(provider1)),
	)

	ctx := context.Background()
	lc.Start(ctx)

	// either the providers SendInformation function is called or the test will fail as a timeout
	waitGroup.Wait()
	assert.Equal(t, []int{0, 1, 2}, orderedResult)
	assert.NoError(t, lc.Stop(ctx))
}
