// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermocks "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func TestEgressControllerForwardsEligibleRetainedRange(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	from := time.Unix(100, 0)
	require.NoError(t, retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "target",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: float64(from.Unix()),
	}}))

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
	}).Return(nil).Once()

	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		SendDelay: time.Nanosecond,
		Now:       func() time.Time { return from.Add(time.Minute) },
	})
	controller.policy.OnDecision(monitor.Decision{State: monitor.Breach, WindowFrom: from, WindowTo: from.Add(time.Second)})

	controller.RunOnce()
	require.Len(t, controller.policy.ForwardedRanges(), 1)
}

func TestEgressControllerRespectsSendDelay(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	pointTime := time.Unix(100, 0)
	require.NoError(t, retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "target",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: float64(pointTime.Unix()),
	}}))

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Return(nil).Once()

	now := pointTime.Add(5 * time.Second)
	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		SendDelay: 30 * time.Second,
		Now:       func() time.Time { return now },
	})
	controller.policy.OnDecision(monitor.Decision{State: monitor.Breach, WindowFrom: pointTime, WindowTo: pointTime.Add(time.Second)})

	controller.RunOnce()
	require.Empty(t, controller.policy.ForwardedRanges())

	now = pointTime.Add(31 * time.Second)
	controller.RunOnce()
	require.Len(t, controller.policy.ForwardedRanges(), 1)
}

func TestEgressControllerRetriesFailedRange(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	from := time.Unix(100, 0)
	require.NoError(t, retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "target",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: float64(from.Unix()),
	}}))

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Return(errors.New("boom")).Once()
	serializer.On("SendIterableSeries", mock.Anything).Return(nil).Once()

	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		SendDelay: time.Nanosecond,
		Now:       func() time.Time { return from.Add(time.Minute) },
	})
	controller.policy.OnDecision(monitor.Decision{State: monitor.Breach, WindowFrom: from, WindowTo: from.Add(time.Second)})

	controller.RunOnce()
	require.Empty(t, controller.policy.ForwardedRanges())

	controller.RunOnce()
	require.Len(t, controller.policy.ForwardedRanges(), 1)
}

func TestEgressControllerAppliesMonitorDecisions(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	serializer := serializermocks.NewMetricSerializer(t)
	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		HealthyWindowsToSuppress: 1,
		Now:                      func() time.Time { return time.Unix(100, 0) },
	})

	require.Equal(t, EgressSuppressed, controller.Mode())

	controller.OnDecision(monitor.Decision{
		State:      monitor.Breach,
		WindowFrom: time.Unix(60, 0),
		WindowTo:   time.Unix(90, 0),
	})
	require.Eventually(t, func() bool {
		return controller.Mode() == EgressForwarding
	}, time.Second, time.Millisecond)

	controller.OnDecision(monitor.Decision{
		State:      monitor.Healthy,
		WindowFrom: time.Unix(90, 0),
		WindowTo:   time.Unix(120, 0),
	})
	require.Eventually(t, func() bool {
		return controller.Mode() == EgressSuppressed
	}, time.Second, time.Millisecond)
}
