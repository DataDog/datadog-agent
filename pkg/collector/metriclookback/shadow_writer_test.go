// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	corelookback "github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNewSenderManagerRequiresRetention(t *testing.T) {
	require.Nil(t, NewSenderManager(context.Background(), "default-host", nil))
}

func TestNewSenderManagerWritesShadowCheckSamples(t *testing.T) {
	retention := corelookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	manager := NewSenderManager(context.Background(), "default-host", retention)
	require.NotNil(t, manager)

	sender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	sender.Gauge("shadow.metric", 42, "", []string{"env:staging"})
	sender.Commit()

	points := retention.PointsBetween(
		ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: "cpu:shadow"},
		"shadow.metric",
		time.Unix(0, 0),
		time.Now().Add(time.Minute),
	)
	require.Len(t, points, 1)
	require.Equal(t, float64(42), points[0].Value)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "shadow.metric", series[0].Name)
	require.Equal(t, "default-host", series[0].Host)
	require.Equal(t, []string{"env:staging"}, series[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, metrics.CheckNameToMetricSource("cpu"), series[0].Source)
	require.Equal(t, "System", series[0].SourceTypeName)
}

func TestNewSenderManagerSamplesNotifyMonitor(t *testing.T) {
	retention := corelookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	decisions := make(chan monitor.Decision, 1)
	watcher, err := monitor.New(monitor.Config{
		MetricName:         "shadow.metric",
		RangeEpsilon:       0.05,
		EvaluationInterval: 2 * time.Second,
		MinPoints:          2,
	}, monitor.PointReaderFunc(func(metricName string, from, to time.Time) []monitor.Point {
		points := retention.PointsBetweenSources([]ringbuffer.Source{{Kind: ringbuffer.SourceCheckShadow}}, metricName, from, to)
		out := make([]monitor.Point, 0, len(points))
		for _, point := range points {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value})
		}
		return out
	}), monitor.DecisionSinkFunc(func(decision monitor.Decision) {
		decisions <- decision
	}))
	require.NoError(t, err)
	retention.SetMonitor(watcher)

	manager := NewSenderManager(context.Background(), "default-host", retention)
	sender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	require.NoError(t, sender.GaugeWithTimestamp("shadow.metric", 40, "", nil, 10))
	sender.Commit()
	require.NoError(t, sender.GaugeWithTimestamp("shadow.metric", 40.1, "", nil, 12))
	sender.Commit()

	select {
	case decision := <-decisions:
		require.Equal(t, monitor.Breach, decision.State)
		require.Equal(t, "shadow.metric", decision.MetricName)
		require.Equal(t, float64(40), decision.Min)
		require.Equal(t, 40.1, decision.Max)
		require.InDelta(t, 0.1, decision.Range, 1e-12)
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor decision")
	}
}
