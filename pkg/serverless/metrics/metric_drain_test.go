// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics/metricstest"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Same nil-Demux safety already covered for Flush/FlushAll by
// TestMetricAgentNoOpWithoutDemux in main_test.go.
func TestWaitForPendingSamplesNoOpWithoutDemux(t *testing.T) {
	agent := &ServerlessMetricAgent{}
	assert.NotPanics(t, func() {
		agent.WaitForPendingSamples()
	})
}

// TestWaitForPendingSamplesDrainsRealDemux runs against the actual Fx wiring
// production code gets. bundle.Demux's dynamic type is the demultiplexerimpl
// wrapper struct, not a bare *aggregator.AgentDemultiplexer — a concrete-type
// assertion in WaitForPendingSamples would silently no-op here.
func TestWaitForPendingSamplesDrainsRealDemux(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	bundle := metricstest.New(t, fakeTagger)

	_, ok := bundle.Demux.(pendingSampleDrainer)
	require.True(t, ok, "the Fx-provided demux must satisfy pendingSampleDrainer, or WaitForPendingSamples silently no-ops in production")

	agent := New(bundle.Demux, Tags{EnhancedMetric: []string{"tag:1"}})
	assert.NotPanics(t, func() {
		agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 0)
		agent.WaitForPendingSamples()
		agent.WaitForPendingSamples()
	})
}

// TestWaitForPendingSamplesDrainsThroughWrappedDemux mirrors
// TestStopDrainsThroughWrappedDemux above (same wrappedDemux type): proves a
// sample submitted through the wrapped Demux is actually drained, not just
// that the type assertion succeeds. Without a working drain, AddEnhancedMetric
// and the worker processing it race FlushAll — same race TestStopDrainsBeforeFlush
// documents — so a single iteration can pass by luck; 100 iterations make a
// broken drain reliably show up as a sketchCount short of iterations.
func TestWaitForPendingSamplesDrainsThroughWrappedDemux(t *testing.T) {
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)

	cf := newCountingForwarder()

	deps := fxutil.Test[aggregator.TestDeps](t,
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		fx.Provide(func() defaultforwarder.Component { return cf }),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		haagentmock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistmock.MockModule(),
	)

	const iterations = 100
	future := float64(time.Now().Add(time.Hour).UnixNano()) / float64(time.Second)
	for i := 0; i < iterations; i++ {
		opts := aggregator.DefaultAgentDemultiplexerOptions()
		opts.FlushInterval = time.Hour
		opts.DontStartForwarders = true
		demux := aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")

		agent := New(wrappedDemux{AgentDemultiplexer: demux}, Tags{})
		agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, future)

		agent.WaitForPendingSamples()
		agent.FlushAll()

		demux.Stop()
	}

	require.Equal(t, int64(iterations), cf.sketchCount.Load(),
		"WaitForPendingSamples must drain every sample submitted through the wrapped Demux before FlushAll runs")
}
