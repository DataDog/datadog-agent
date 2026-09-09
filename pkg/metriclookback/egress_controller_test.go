// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
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

func TestEgressControllerDoesNotRetrySeriesAfterSketchFailure(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	from := time.Unix(100, 0)
	require.NoError(t, retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "target.series",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: float64(from.Unix()),
	}}))
	require.NoError(t, retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "target.sketch"},
		Points: []metrics.SketchPoint{{
			Ts:     from.Unix(),
			Sketch: testSketchData(1, 2),
		}},
	}))

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
	}).Return(nil).Once()
	serializer.On("SendSketch", mock.Anything).Return(errors.New("boom")).Once()
	serializer.On("SendSketch", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SketchesSource)
		require.Equal(t, uint64(1), source.Count())
	}).Return(nil).Once()

	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		SendDelay: time.Nanosecond,
		Now:       func() time.Time { return from.Add(time.Minute) },
	})
	controller.policy.OnDecision(monitor.Decision{State: monitor.Breach, WindowFrom: from, WindowTo: from.Add(time.Second)})

	controller.RunOnce()
	require.Len(t, controller.policy.ForwardedSeriesRanges(), 1)
	require.Empty(t, controller.policy.ForwardedSketchRanges())

	controller.RunOnce()
	require.Len(t, controller.policy.ForwardedSeriesRanges(), 1)
	require.Len(t, controller.policy.ForwardedSketchRanges(), 1)
}

func TestEgressControllerAppliesMonitorDecisions(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	serializer := serializermocks.NewMetricSerializer(t)
	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		Now: func() time.Time { return time.Unix(100, 0) },
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

func TestEgressControllerStopInterruptsEgressInterval(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	serializer := serializermocks.NewMetricSerializer(t)
	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		EgressInterval: time.Hour,
	})
	controller.Start()

	stopped := make(chan struct{})
	go func() {
		controller.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		require.FailNow(t, "egress controller Stop waited for the egress interval")
	}
}

func TestEgressControllerDryRunStartsForwardingAndIgnoresMonitorDecisions(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	serializer := serializermocks.NewMetricSerializer(t)
	controller := NewEgressController(retention, serializer, EgressControllerOptions{
		DryRun: true,
		Now:    func() time.Time { return time.Unix(100, 0) },
	})

	require.Equal(t, EgressForwarding, controller.Mode())
	require.Equal(t, []TimeRange{{}}, controller.policy.ForwardingRanges())

	controller.OnDecision(monitor.Decision{
		State:      monitor.Healthy,
		WindowFrom: time.Unix(60, 0),
		WindowTo:   time.Unix(90, 0),
	})
	controller.OnDecision(monitor.Decision{
		State:      monitor.Healthy,
		WindowFrom: time.Unix(90, 0),
		WindowTo:   time.Unix(120, 0),
	})
	controller.OnDecision(monitor.Decision{
		State:      monitor.Breach,
		WindowFrom: time.Unix(120, 0),
		WindowTo:   time.Unix(150, 0),
	})

	require.Eventually(t, func() bool {
		return controller.Mode() == EgressForwarding
	}, time.Second, time.Millisecond)
	require.Equal(t, []TimeRange{{}}, controller.policy.ForwardingRanges())
}

func TestEgressControllerLogsMonitorStateTransitionsRegardlessOfDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry_run_%t", dryRun), func(t *testing.T) {
			retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
			serializer := serializermocks.NewMetricSerializer(t)
			var transitions []MonitorStateTransition
			controller := NewEgressController(retention, serializer, EgressControllerOptions{
				DryRun: dryRun,
				MonitorStateTransitionLogger: func(transition MonitorStateTransition) {
					transitions = append(transitions, transition)
				},
				Now: func() time.Time { return time.Unix(100, 0) },
			})

			controller.OnDecision(monitor.Decision{MetricName: "target", State: monitor.Unknown, WindowFrom: time.Unix(0, 0), WindowTo: time.Unix(30, 0)})
			controller.OnDecision(monitor.Decision{MetricName: "target", State: monitor.Healthy, WindowFrom: time.Unix(30, 0), WindowTo: time.Unix(60, 0)})
			controller.OnDecision(monitor.Decision{MetricName: "target", State: monitor.Healthy, WindowFrom: time.Unix(60, 0), WindowTo: time.Unix(90, 0)})
			controller.OnDecision(monitor.Decision{MetricName: "target", State: monitor.Breach, WindowFrom: time.Unix(90, 0), WindowTo: time.Unix(120, 0)})

			require.Len(t, transitions, 3)
			require.True(t, transitions[0].Initial)
			require.Equal(t, monitor.Unknown, transitions[0].To)
			require.False(t, transitions[1].Initial)
			require.Equal(t, monitor.Unknown, transitions[1].From)
			require.Equal(t, monitor.Healthy, transitions[1].To)
			require.Equal(t, monitor.Healthy, transitions[2].From)
			require.Equal(t, monitor.Breach, transitions[2].To)
			for _, transition := range transitions {
				require.Equal(t, dryRun, transition.DryRun)
			}
		})
	}
}
