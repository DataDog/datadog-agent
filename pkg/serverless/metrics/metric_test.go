// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
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
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics/metricstest"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func TestMain(m *testing.M) {
	// setting the hostname cache saves about 1s when starting the metric agent
	cacheKey := cache.BuildAgentKey("hostname")
	cache.Cache.Set(cacheKey, hostname.Data{}, cache.NoExpiration)
	os.Exit(m.Run())
}

func TestConstructionDoesNotBlock(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("known to fail on the macOS Gitlab runners because of the already running Agent")
	}
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
	deps := metricstest.New(t, nooptagger.NewComponent())
	metricAgent := &ServerlessMetricAgent{Demux: deps.Demux}
	assert.NotNil(t, metricAgent.Demux)
}

// countingForwarder wraps NoopForwarder, provides a real domain resolver so that
// the serializer's pipeline path is exercised, and counts sketch transactions.
type countingForwarder struct {
	defaultforwarder.NoopForwarder
	sketchCount atomic.Int64
	resolvers   []resolver.DomainResolver
}

func newCountingForwarder() *countingForwarder {
	r, _ := resolver.NewSingleDomainResolver("https://fake.datadoghq.com",
		[]configutils.APIKeys{configutils.NewAPIKeys("api_key", "fakeapikey")})
	return &countingForwarder{resolvers: []resolver.DomainResolver{r}}
}

// GetDomainResolvers returns the fake resolver so buildPipelines creates a pipeline.
func (f *countingForwarder) GetDomainResolvers() []resolver.DomainResolver {
	return f.resolvers
}

// SubmitTransaction increments the sketch counter when a sketch-series transaction arrives.
func (f *countingForwarder) SubmitTransaction(txn *transaction.HTTPTransaction) error {
	if strings.Contains(txn.Endpoint.Name, "sketch") {
		f.sketchCount.Add(1)
	}
	return nil
}

// SubmitSketchSeries is kept for interface compliance but is not called by the pipeline path.
func (f *countingForwarder) SubmitSketchSeries(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// TestStopDrainsBeforeFlush asserts that, with aggregator_drain_samples_on_stop
// enabled, AgentDemultiplexer.Stop(true) drains the timeSamplerWorker's
// samplesChan before its flush, so a sample submitted via AddEnhancedMetric
// immediately before Stop is delivered to the serializer. Without the drain
// barrier the worker's select can pick the flush trigger over samplesChan and
// flush before the sample is incorporated — a race that drops ~50% of samples
// in practice. 100 iterations exercise that race.
func TestStopDrainsBeforeFlush(t *testing.T) {
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
	for i := 0; i < iterations; i++ {
		opts := aggregator.DefaultAgentDemultiplexerOptions()
		opts.FlushInterval = time.Hour // disable automatic flushes
		opts.DontStartForwarders = true
		// Enable the drain-on-stop behavior the cascade relies on. In production
		// createAgentDemultiplexerOptions populates this from
		// aggregator_drain_samples_on_stop; tests set the option directly.
		opts.DrainSamplesOnStop = true
		demux := aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")

		agent := New(demux, Tags{})
		agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

		// Stop(true) must drain the worker's samplesChan before flushing so the
		// late sample reliably reaches the serializer. Without the drain barrier
		// the worker's select can pick the flush trigger first and drop it.
		demux.Stop(true)
	}

	require.Equal(t, int64(iterations), cf.sketchCount.Load(),
		"every AddEnhancedMetric followed by Stop(true) must produce exactly one sketch flush")
}

// The deterministic negative of the drain behavior (gate off => Stop(true) skips
// the WaitForPendingSamples barrier) lives in pkg/aggregator as
// TestStopSkipsDrainWhenDisabled, where white-box access to a non-started demux
// makes a provably-stuck sample possible. A serverless-level negative here would
// have to rely on a late-sample data-loss race (non-deterministic), so it is
// asserted at the aggregator level instead.

// wrappedDemux mirrors the demultiplexerimpl.demultiplexer wrapper struct that
// Fx actually supplies to ServerlessMetricAgent: the aggregator.Demultiplexer
// interface holds a struct that embeds *aggregator.AgentDemultiplexer rather
// than the pointer itself. This test confirms a sample submitted through the
// agent's interface-typed Demux is still drained and flushed by Stop(true) on
// the underlying concrete demultiplexer.
type wrappedDemux struct {
	*aggregator.AgentDemultiplexer
}

func TestStopDrainsThroughWrappedDemux(t *testing.T) {
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

	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = time.Hour
	opts.DontStartForwarders = true
	opts.DrainSamplesOnStop = true
	demux := aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")

	// The agent receives the wrapper (as Fx supplies it in production); the
	// late sample is submitted through that interface-typed Demux.
	agent := New(wrappedDemux{AgentDemultiplexer: demux}, Tags{})
	agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

	demux.Stop(true)

	require.Equal(t, int64(1), cf.sketchCount.Load(),
		"Stop(true) must drain pending samples submitted through the wrapped Demux and flush them")
}

// TestShutdownCascadeFlushesLateSample is the end-to-end integration of the
// serverless-init shutdown cascade. It builds the full Fx graph that
// cmd/serverless-init wires (forwarder -> demultiplexer -> DogStatsD server),
// with a counting forwarder injected and both flush-on-stop gates enabled, then
// submits a late metric through the production emit path (ServerlessMetricAgent
// on the bundle's Demux) and stops the app. app.Stop fires the OnStop hooks in
// reverse construction order — dsdServer.stop -> demux.Stop(true) ->
// forwarder.Stop — so the demux drains its samplesChan and flushes the late
// sample to the serializer before the forwarder tears down. Asserting the
// counting forwarder received exactly one sketch proves the whole cascade
// delivers the sample without an external orchestrator.
func TestShutdownCascadeFlushesLateSample(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("known to fail on the macOS Gitlab runners because of the already running Agent")
	}
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
	// Enable both serverless-init flush-on-stop gates the cascade relies on,
	// mirroring the overrides cmd/serverless-init sets in preloadEarly.
	mockConfig.SetWithoutSource("dogstatsd_flush_on_stop", true)
	mockConfig.SetWithoutSource("aggregator_drain_samples_on_stop", true)
	// The Fx demux uses the 15s DefaultFlushInterval; the test completes well
	// within that window, so the only flush is the one app.Stop drives.

	cf := newCountingForwarder()

	app, deps := metricstest.StartBundle(t, nooptagger.NewComponent(), cf)

	// Submit a late sample through the production emit path, just like an
	// in-flight request would right before serverless-init shuts down.
	agent := New(deps.Demux, Tags{})
	agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

	// Drive the shutdown cascade: OnStop fires dsdServer.stop -> demux.Stop(true)
	// -> forwarder.Stop in reverse construction order.
	require.NoError(t, app.Stop(context.Background()))

	require.Equal(t, int64(1), cf.sketchCount.Load(),
		"the OnStop cascade must drain and flush the late sample to the forwarder")
}
