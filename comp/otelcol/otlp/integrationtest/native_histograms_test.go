// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && zlib && test && !aix

package integrationtest

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

const nativeHistogramsGate = "datadog.serializerexporter.NativeOTelHistograms"

// OTel SDK default explicit bucket boundaries.
var sdkDefaultBounds = []float64{0, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000}

func optional(v float64) *float64 { return &v }

type explicitCase struct {
	name          string
	bounds        []float64
	counts        []uint64
	count         uint64
	sum, min, max *float64
	noRecorded    bool
	dropped       bool // the exporter must not send the point
}

type exponentialCase struct {
	name           string
	scale          int32
	zeroCount      uint64
	zeroThreshold  float64
	posOffset      int32
	positive       []uint64
	negOffset      int32
	negative       []uint64
	count          uint64
	sum, min, max  *float64
	dropped        bool
	dataPointFlags pmetric.DataPointFlags
}

// Shapes sent by real producers (OTel SDKs, Micrometer, Prometheus, StatsD and Datadog
// receivers), plus points the exporter must drop.
var explicitCases = []explicitCase{
	{name: "explicit.sdk_default", bounds: sdkDefaultBounds, counts: []uint64{0, 3, 7, 12, 30, 25, 10, 6, 4, 2, 1, 0, 0, 0, 0, 1}, count: 101, sum: optional(23456.5), min: optional(1.25), max: optional(12000.75)},
	{name: "explicit.seconds", bounds: []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}, counts: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, count: 120, sum: optional(93.123456789), min: optional(0.0011), max: optional(11.9)},
	{name: "explicit.micrometer", counts: []uint64{7}, count: 7, sum: optional(42), max: optional(9)},
	{name: "explicit.no_sum", bounds: []float64{0, 10}, counts: []uint64{1, 2, 3}, count: 6, min: optional(-5), max: optional(20)},
	{name: "explicit.zero_sum_negative", bounds: []float64{-10, 0, 10}, counts: []uint64{2, 3, 1, 0}, count: 6, sum: optional(0), min: optional(-15), max: optional(7)},
	{name: "explicit.summary_only", count: 3, sum: optional(6)},
	{name: "explicit.full_width_count", counts: []uint64{math.MaxUint64}, count: math.MaxUint64},
	{name: "explicit.nan", bounds: []float64{1}, counts: []uint64{1, 0}, count: 1, sum: optional(math.NaN()), min: optional(math.NaN()), max: optional(math.NaN())},
	{name: "explicit.sum_overflow", bounds: []float64{1e300}, counts: []uint64{0, 2}, count: 2, sum: optional(math.Inf(1)), min: optional(1.5e308), max: optional(1.7e308)},
	{name: "explicit.unsorted_bounds", bounds: []float64{2, 1}, counts: []uint64{1, 1, 1}, count: 3, dropped: true},
	{name: "explicit.no_recorded_value", bounds: []float64{1}, counts: []uint64{1, 1}, count: 2, noRecorded: true, dropped: true},
}

var exponentialCases = []exponentialCase{
	{name: "exp.sdk_default", scale: 20, posOffset: 1_234_567, positive: denseCounts(160), count: sum(denseCounts(160)), sum: optional(98765.4321), min: optional(1.0002), max: optional(1.1)},
	{name: "exp.dotnet_min_scale", scale: -11, positive: []uint64{5}, count: 5, sum: optional(1e30), min: optional(3), max: optional(5e29)},
	{name: "exp.prometheus_native", scale: 3, zeroCount: 2, zeroThreshold: math.Ldexp(1, -128), posOffset: -5, positive: []uint64{1, 0, 0, 4}, negOffset: 2, negative: []uint64{3}, count: 10, sum: optional(12.5)},
	{name: "exp.negative_only", scale: 1, negOffset: -1000, negative: []uint64{1, 2}, count: 3, sum: optional(-60), min: optional(-50), max: optional(-1)},
	{name: "exp.zero_only", zeroCount: 9, count: 9, sum: optional(0), min: optional(0), max: optional(0)},
	{name: "exp.offset_limits", scale: 0, posOffset: math.MaxInt32 - 1, positive: []uint64{1, 2}, negOffset: math.MinInt32, negative: []uint64{3}, count: 6},
	{name: "exp.datadogreceiver", scale: 5, zeroThreshold: 1e-9, zeroCount: 1, posOffset: 10, positive: []uint64{4, 0, 6}, count: 11, sum: optional(31.75)},
	{name: "exp.offset_overflow", posOffset: math.MaxInt32, positive: []uint64{1, 1}, count: 2, dropped: true},
	{name: "exp.nan_zero_threshold", zeroThreshold: math.NaN(), zeroCount: 1, count: 1, dropped: true},
	{name: "exp.no_recorded_value", positive: []uint64{1}, count: 1, dropped: true, dataPointFlags: pmetric.DefaultDataPointFlags.WithNoRecordedValue(true)},
}

func denseCounts(n int) []uint64 {
	c := make([]uint64, n)
	for i := range c {
		c[i] = uint64((i*7 + 3) % 11) // zeros inside the range included
	}
	return c
}

func sum(c []uint64) (s uint64) {
	for _, v := range c {
		s += v
	}
	return s
}

const roundTripTimestamp = 1_700_000_000

func newRoundTripMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("host.name", "roundtrip-host")
	ms := rm.ScopeMetrics().AppendEmpty().Metrics()
	start := pcommon.Timestamp(time.Duration(roundTripTimestamp-10) * time.Second)
	ts := pcommon.Timestamp(time.Duration(roundTripTimestamp) * time.Second)

	for _, c := range explicitCases {
		m := ms.AppendEmpty()
		m.SetName(c.name)
		m.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		p := m.Histogram().DataPoints().AppendEmpty()
		p.SetStartTimestamp(start)
		p.SetTimestamp(ts)
		p.Attributes().PutStr("producer", c.name)
		p.ExplicitBounds().FromRaw(c.bounds)
		p.BucketCounts().FromRaw(c.counts)
		p.SetCount(c.count)
		if c.sum != nil {
			p.SetSum(*c.sum)
		}
		if c.min != nil {
			p.SetMin(*c.min)
		}
		if c.max != nil {
			p.SetMax(*c.max)
		}
		if c.noRecorded {
			p.SetFlags(pmetric.DefaultDataPointFlags.WithNoRecordedValue(true))
		}
	}

	for _, c := range exponentialCases {
		m := ms.AppendEmpty()
		m.SetName(c.name)
		m.SetEmptyExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		p := m.ExponentialHistogram().DataPoints().AppendEmpty()
		p.SetStartTimestamp(start)
		p.SetTimestamp(ts)
		p.Attributes().PutStr("producer", c.name)
		p.SetScale(c.scale)
		p.SetZeroCount(c.zeroCount)
		p.SetZeroThreshold(c.zeroThreshold)
		p.Positive().SetOffset(c.posOffset)
		p.Positive().BucketCounts().FromRaw(c.positive)
		p.Negative().SetOffset(c.negOffset)
		p.Negative().BucketCounts().FromRaw(c.negative)
		p.SetCount(c.count)
		if c.sum != nil {
			p.SetSum(*c.sum)
		}
		if c.min != nil {
			p.SetMin(*c.min)
		}
		if c.max != nil {
			p.SetMax(*c.max)
		}
		p.SetFlags(c.dataPointFlags)
	}

	// The minimal translator drops cumulative histograms.
	m := ms.AppendEmpty()
	m.SetName("explicit.cumulative")
	m.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	p := m.Histogram().DataPoints().AppendEmpty()
	p.SetStartTimestamp(start)
	p.SetTimestamp(ts)
	p.BucketCounts().FromRaw([]uint64{1})
	p.SetCount(1)
	return md
}

// sendNativeHistograms sends md through the DDOT metrics exporter with native histograms
// enabled, over HTTP to an in-process fakeintake, and returns a client of that fakeintake.
func sendNativeHistograms(t *testing.T, md pmetric.Metrics) *fakeintakeclient.Client {
	t.Helper()
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithAddress("127.0.0.1:0"),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })

	reg := featuregate.GlobalRegistry()
	require.NoError(t, reg.Set(nativeHistogramsGate, true))
	t.Cleanup(func() { require.NoError(t, reg.Set(nativeHistogramsGate, false)) })

	cfg := configmock.New(t)
	cfg.SetInTest("dd_url", fi.URL())
	cfg.SetInTest("api_key", "00000000000000000000000000000000")
	cfg.SetInTest("serializer_experimental_use_v3_api.sketches.endpoints", []string{fi.URL()})
	logger := logmock.New(t)
	eds, err := configutils.GetMultipleEndpoints(cfg)
	require.NoError(t, err)
	fwd, err := defaultforwarderimpl.NewOTelSyncForwarder(cfg, logger, &secretnooptypes.SecretNoop{}, eds, &http.Client{Timeout: 10 * time.Second})
	require.NoError(t, err)
	s := serializer.NewSerializer(fwd, nil, zstdimpl.New(zstdimpl.Requires{Level: 1}), cfg, logger, "")

	ctx := context.Background()
	f := serializerexporter.NewFactoryForOTelAgent(s, func(context.Context) (string, error) {
		return "test-host", nil
	}, nil, otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, nil, false)
	exp, err := f.CreateMetrics(ctx, exportertest.NewNopSettings(component.MustNewType("datadog")), f.CreateDefaultConfig())
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	// Shutdown drains the sending queue; the sync forwarder has delivered every payload once it returns.
	require.NoError(t, exp.Shutdown(ctx))

	return fakeintakeclient.NewClient(fi.URL())
}

// TestNativeOTelHistogramsRoundTrip checks that OTel histograms sent through the DDOT
// exporter reach the intake over API v3 with every field preserved.
func TestNativeOTelHistogramsRoundTrip(t *testing.T) {
	fic := sendNativeHistograms(t, newRoundTripMetrics())

	for _, c := range explicitCases {
		t.Run(c.name, func(t *testing.T) {
			histograms, err := fic.FilterHistograms(c.name)
			require.NoError(t, err)
			if c.dropped {
				assert.Empty(t, histograms)
				return
			}
			require.Len(t, histograms, 1)
			h := histograms[0]
			assertSeriesIdentity(t, h, c.name)
			require.Empty(t, h.Exponential)
			require.Len(t, h.Explicit, 1)
			p := h.Explicit[0]
			assert.Equal(t, int64(roundTripTimestamp), p.Timestamp)
			assert.Equal(t, c.count, p.Count)
			assertOptional(t, c.sum, p.Sum, "sum")
			assertOptional(t, c.min, p.Min, "min")
			assertOptional(t, c.max, p.Max, "max")
			assert.Equal(t, nonEmpty(c.bounds), nonEmpty(p.Bounds))
			assert.Equal(t, nonEmpty(c.counts), nonEmpty(p.Counts))
		})
	}

	for _, c := range exponentialCases {
		t.Run(c.name, func(t *testing.T) {
			histograms, err := fic.FilterHistograms(c.name)
			require.NoError(t, err)
			if c.dropped {
				assert.Empty(t, histograms)
				return
			}
			require.Len(t, histograms, 1)
			h := histograms[0]
			assertSeriesIdentity(t, h, c.name)
			require.Empty(t, h.Explicit)
			require.Len(t, h.Exponential, 1)
			p := h.Exponential[0]
			assert.Equal(t, int64(roundTripTimestamp), p.Timestamp)
			assert.Equal(t, c.count, p.Count)
			assertOptional(t, c.sum, p.Sum, "sum")
			assertOptional(t, c.min, p.Min, "min")
			assertOptional(t, c.max, p.Max, "max")
			assert.Equal(t, c.scale, p.Scale)
			assert.Equal(t, c.zeroCount, p.ZeroCount)
			assert.Equal(t, c.zeroThreshold, p.ZeroThreshold)
			assert.Equal(t, nonEmpty(c.positive), nonEmpty(p.Positive))
			assert.Equal(t, nonEmpty(c.negative), nonEmpty(p.Negative))
			// An empty bucket range carries no offset in OTLP terms; only compare non-empty ones.
			if len(c.positive) > 0 {
				assert.Equal(t, c.posOffset, p.PositiveOffset)
			}
			if len(c.negative) > 0 {
				assert.Equal(t, c.negOffset, p.NegativeOffset)
			}
		})
	}

	t.Run("cumulative dropped", func(t *testing.T) {
		histograms, err := fic.FilterHistograms("explicit.cumulative")
		require.NoError(t, err)
		assert.Empty(t, histograms)
	})

	t.Run("no DDSketch sent", func(t *testing.T) {
		names, err := fic.GetSketchNames()
		require.NoError(t, err)
		assert.Empty(t, names)
	})
}

func assertSeriesIdentity(t *testing.T, h *aggregator.Histogram, producer string) {
	t.Helper()
	assert.Contains(t, h.Tags, "producer:"+producer)
	var hosts []string
	for _, r := range h.Resources {
		if r.Type == "host" {
			hosts = append(hosts, r.Name)
		}
	}
	assert.Equal(t, []string{"roundtrip-host"}, hosts)
}

func assertOptional(t *testing.T, want, got *float64, field string) {
	t.Helper()
	if want == nil {
		assert.Nil(t, got, "%s must be absent", field)
		return
	}
	require.NotNil(t, got, "%s must be present", field)
	if math.IsNaN(*want) {
		assert.True(t, math.IsNaN(*got), "%s: want NaN, got %v", field, *got)
		return
	}
	assert.Equal(t, *want, *got, field)
}

// nonEmpty maps an empty slice to nil so that absent and empty ranges compare equal.
func nonEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}
