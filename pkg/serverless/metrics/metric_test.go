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
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
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

// countingForwarder wraps a no-op forwarder, provides a real domain resolver so
// that the serializer's pipeline path is exercised, and counts sketch transactions.
type countingForwarder struct {
	defaultforwarder.Component
	sketchCount atomic.Int64
	resolvers   []resolver.DomainResolver
}

func newCountingForwarder() *countingForwarder {
	r, _ := resolver.NewSingleDomainResolver("https://fake.datadoghq.com",
		[]configutils.APIKeys{configutils.NewAPIKeys("api_key", "fakeapikey")})
	return &countingForwarder{
		Component: defaultforwardernoop.NewComponent(),
		resolvers: []resolver.DomainResolver{r},
	}
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

// TestStopDrainsBeforeFlush asserts that, with dogstatsd_flush_incomplete_buckets
// enabled, AgentDemultiplexer.Stop() drains the timeSamplerWorker's samplesChan
// before its final flush, so a sample submitted via AddEnhancedMetric
// immediately before Stop is delivered to the serializer. Without the drain
// barrier the worker's select can pick the flush trigger over samplesChan and
// flush before the sample is incorporated — a race that drops ~50% of samples
// in practice. 100 iterations exercise that race.
func TestStopDrainsBeforeFlush(t *testing.T) {
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
	// Gate Stop()'s per-worker sample drain (and the incomplete-bucket flush),
	// the same way serverless-init does via preloadEarly.
	mockConfig.SetInTest("dogstatsd_flush_incomplete_buckets", true)

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
		demux := aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")

		agent := New(demux, Tags{})
		agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

		// Stop() must drain the worker's samplesChan before flushing so the
		// late sample reliably reaches the serializer. Without the drain barrier
		// the worker's select can pick the flush trigger first and drop it.
		demux.Stop()
	}

	require.Equal(t, int64(iterations), cf.sketchCount.Load(),
		"every AddEnhancedMetric followed by Stop() must produce exactly one sketch flush")
}

// wrappedDemux mirrors the demultiplexerimpl.demultiplexer wrapper struct that
// Fx actually supplies to ServerlessMetricAgent: the aggregator.Demultiplexer
// interface holds a struct that embeds *aggregator.AgentDemultiplexer rather
// than the pointer itself. This test confirms a sample submitted through the
// agent's interface-typed Demux is still drained and flushed by Stop() on
// the underlying concrete demultiplexer.
type wrappedDemux struct {
	*aggregator.AgentDemultiplexer
}

func TestStopDrainsThroughWrappedDemux(t *testing.T) {
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
	mockConfig.SetInTest("dogstatsd_flush_incomplete_buckets", true)

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
	demux := aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "")

	// The agent receives the wrapper (as Fx supplies it in production); the
	// late sample is submitted through that interface-typed Demux.
	agent := New(wrappedDemux{AgentDemultiplexer: demux}, Tags{})
	agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

	demux.Stop()

	require.Equal(t, int64(1), cf.sketchCount.Load(),
		"Stop() must drain pending samples submitted through the wrapped Demux and flush them")
}

// TestShutdownCascadeFlushesLateSample is the end-to-end integration of the
// serverless-init shutdown cascade. It builds the full Fx graph that
// cmd/serverless-init wires (forwarder -> demultiplexer -> DogStatsD server),
// with a counting forwarder injected and the flush-on-stop gate enabled, then
// submits a late metric through the production emit path (ServerlessMetricAgent
// on the bundle's Demux) and stops the app. app.Stop fires the OnStop hooks in
// reverse construction order — dsdServer.stop -> demux.Stop() ->
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
	// Enable the flush-on-stop gate the cascade relies on, mirroring the
	// override cmd/serverless-init sets in preloadEarly.
	mockConfig.SetInTest("dogstatsd_flush_incomplete_buckets", true)
	// The Fx demux uses the 15s DefaultFlushInterval; the test completes well
	// within that window, so the only flush is the one app.Stop drives.

	cf := newCountingForwarder()

	app, deps := metricstest.StartBundle(t, nooptagger.NewComponent(), cf)

	// Submit a late sample through the production emit path, just like an
	// in-flight request would right before serverless-init shuts down.
	agent := New(deps.Demux, Tags{})
	agent.AddEnhancedMetric("test.metric", 1.0, pkgmetrics.MetricSourceServerless, 1000.0)

	// Drive the shutdown cascade: OnStop fires dsdServer.stop -> demux.Stop()
	// -> forwarder.Stop in reverse construction order.
	require.NoError(t, app.Stop(context.Background()))

	require.Equal(t, int64(1), cf.sketchCount.Load(),
		"the OnStop cascade must drain and flush the late sample to the forwarder")
}
