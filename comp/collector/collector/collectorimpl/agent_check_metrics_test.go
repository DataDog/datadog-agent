// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package collectorimpl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestMetricsRunnerWithCollectorLifecycle(t *testing.T) {
	c := newCollector(fxutil.Test[dependencies](t,
		core.MockBundle(),
		demultiplexerimpl.MockModule(),
		haagentmock.Module(),
		fx.Provide(func() option.Option[serializer.MetricSerializer] {
			return option.None[serializer.MetricSerializer]()
		}),
		fx.Provide(func() option.Option[agenttelemetry.Component] {
			return option.None[agenttelemetry.Component]()
		}),
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{"check_cancel_timeout": 500 * time.Millisecond},
		})))

	assert.Equal(t, stopped, c.state.Load(), "Collector should be stopped initially")
	assert.Nil(t, c.metricsRunnerStop, "Metrics runner should not be started")

	// Start collector collector
	err := c.start(context.TODO())
	assert.NoError(t, err, "Collector should start successfully")
	assert.Equal(t, started, c.state.Load(), "Collector should be started")
	assert.NotNil(t, c.metricsRunnerStop, "Metrics runner should be started with collector")

	var gErr1, gErr2 error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // goroutine 1
		defer wg.Done()
		if e := c.stop(context.TODO()); e != nil {
			gErr1 = e // write to private variable
		}
	}()

	go func() { // goroutine 2
		defer wg.Done()
		if e := c.stop(context.TODO()); e != nil {
			gErr2 = e // write to private variable
		}
	}()

	wg.Wait()

	assert.NoError(t, gErr1, "Collector should stop successfully (goroutine 1)")
	assert.NoError(t, gErr2, "Collector should stop successfully (goroutine 2)")
	assert.Equal(t, stopped, c.state.Load(), "Collector should be stopped")
	assert.Nil(t, c.metricsRunnerStop, "Metrics runner should be stopped with collector")
}

func TestMetricsRunnerWithCollectorLifecycleDisabled(t *testing.T) {
	c := newCollector(fxutil.Test[dependencies](t,
		core.MockBundle(),
		demultiplexerimpl.MockModule(),
		haagentmock.Module(),
		fx.Provide(func() option.Option[serializer.MetricSerializer] {
			return option.None[serializer.MetricSerializer]()
		}),
		fx.Provide(func() option.Option[agenttelemetry.Component] {
			return option.None[agenttelemetry.Component]()
		}),
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"check_cancel_timeout":               500 * time.Millisecond,
				"integration_status_metrics_enabled": false,
			},
		})))

	// Start collector collector
	err := c.start(context.TODO())
	assert.NoError(t, err, "Collector should start successfully")
	assert.Equal(t, started, c.state.Load(), "Collector should be started")
	assert.Nil(t, c.metricsRunnerStop, "Metrics runner should be started with collector")

	// Stop collector
	err = c.stop(context.TODO())
	assert.NoError(t, err, "Collector should stop successfully")
	assert.Equal(t, stopped, c.state.Load(), "Collector should be stopped")
}
