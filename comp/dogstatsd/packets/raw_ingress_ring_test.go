// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRawIngressShardReserveCommitReadRelease(t *testing.T) {
	shard := NewRawIngressShard(2, 64, nil, "0")

	reservation, ok := shard.Reserve()
	require.True(t, ok)
	copy(reservation.Buffer(), []byte("metric:1|c"))
	reservation.Commit(len("metric:1|c"), RawPacketMeta{Source: UDS, ListenerID: "uds-test", Origin: "origin", ProcessID: 42})

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "metric:1|c", string(packet.Contents))
	require.Equal(t, UDS, packet.Source)
	require.Equal(t, "uds-test", packet.ListenerID)
	require.Equal(t, "origin", packet.Origin)
	require.Equal(t, uint32(42), packet.ProcessID)

	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestRawIngressShardBlocksWhenFull(t *testing.T) {
	shard := NewRawIngressShard(1, 64, nil, "0")
	first, ok := shard.Reserve()
	require.True(t, ok)
	first.Commit(1, RawPacketMeta{Source: UDS})

	reserved := make(chan RawPacketReservation, 1)
	go func() {
		reservation, ok := shard.Reserve()
		require.True(t, ok)
		reserved <- reservation
	}()

	select {
	case <-reserved:
		t.Fatal("reserve completed while the ring was full")
	case <-time.After(25 * time.Millisecond):
	}

	packet, ok := shard.TryNext()
	require.True(t, ok)
	packet.Release()

	select {
	case second := <-reserved:
		second.Abort()
	case <-time.After(time.Second):
		t.Fatal("reserve did not unblock after release")
	}
}

func TestRawIngressShardsDistributeReservations(t *testing.T) {
	shards := NewRawIngressShards(2, 128, 64, nil)

	first, ok := shards.Reserve()
	require.True(t, ok)
	first.Commit(1, RawPacketMeta{Source: UDS, ListenerID: "first"})

	second, ok := shards.Reserve()
	require.True(t, ok)
	second.Commit(1, RawPacketMeta{Source: UDS, ListenerID: "second"})

	packet, ok := shards.Shard(0).TryNext()
	require.True(t, ok)
	require.Equal(t, "first", packet.ListenerID)
	packet.Release()

	packet, ok = shards.Shard(1).TryNext()
	require.True(t, ok)
	require.Equal(t, "second", packet.ListenerID)
	packet.Release()
}

func TestRawIngressShardLagTelemetry(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryMock := telemetryComponent.(telemetry.Mock)
	shard := NewRawIngressShard(2, 64, telemetryComponent, "0")

	reservation, ok := shard.Reserve()
	require.True(t, ok)
	copy(reservation.Buffer(), []byte("metric:1|c"))
	reservation.Commit(len("metric:1|c"), RawPacketMeta{Source: UDS})
	time.Sleep(time.Millisecond)

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, float64(1), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_records", map[string]string{"shard": "0"}))
	require.Equal(t, float64(len("metric:1|c")), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_bytes", map[string]string{"shard": "0"}))
	require.Greater(t, gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "oldest_record_timestamp_ns", map[string]string{"shard": "0"}), float64(0))
	require.Greater(t, gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "oldest_record_age_ns", map[string]string{"shard": "0"}), float64(0))

	packet.Release()
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_records", map[string]string{"shard": "0"}))
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_bytes", map[string]string{"shard": "0"}))
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "oldest_record_timestamp_ns", map[string]string{"shard": "0"}))
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "oldest_record_age_ns", map[string]string{"shard": "0"}))
}

func TestRawIngressShardTryNextBatchReleaseBatch(t *testing.T) {
	shard := NewRawIngressShard(4, 16, nil, "0")
	for _, payload := range []string{"a", "bb", "ccc"} {
		reservation, ok := shard.Reserve()
		require.True(t, ok)
		copy(reservation.Buffer(), []byte(payload))
		reservation.Commit(len(payload), RawPacketMeta{Source: UDS, ListenerID: payload})
	}

	batch := shard.TryNextBatch(make([]RawPacket, 0, 2))
	require.Len(t, batch, 2)
	require.Equal(t, "a", string(batch[0].Contents))
	require.Equal(t, "bb", string(batch[1].Contents))
	shard.ReleaseBatch(len(batch))
	require.Equal(t, 1, shard.Len())

	batch = shard.TryNextBatch(batch[:0])
	require.Len(t, batch, 1)
	require.Equal(t, "ccc", string(batch[0].Contents))
	shard.ReleaseBatch(len(batch))
	require.Equal(t, 0, shard.Len())
}

func TestCompactRawIngressShardReserveCommitReadRelease(t *testing.T) {
	shard := NewCompactRawIngressShard(128, 64, nil, "0")

	reservation, ok := shard.Reserve()
	require.True(t, ok)
	copy(reservation.Buffer(), []byte("metric:1|c"))
	reservation.Commit(len("metric:1|c"), RawPacketMeta{Source: UDS, ListenerID: "uds-test", Origin: "origin", ProcessID: 42})

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "metric:1|c", string(packet.Contents))
	require.Equal(t, UDS, packet.Source)
	require.Equal(t, "uds-test", packet.ListenerID)
	require.Equal(t, "origin", packet.Origin)
	require.Equal(t, uint32(42), packet.ProcessID)

	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestCompactRawIngressShardBlocksCommitWhenFull(t *testing.T) {
	shard := NewCompactRawIngressShard(8, 8, nil, "0")
	first, ok := shard.Reserve()
	require.True(t, ok)
	copy(first.Buffer(), []byte("abcdefgh"))
	first.Commit(8, RawPacketMeta{Source: UDS})

	second, ok := shard.Reserve()
	require.True(t, ok)
	copy(second.Buffer(), []byte("ijkl"))
	committed := make(chan struct{})
	go func() {
		second.Commit(4, RawPacketMeta{Source: UDS})
		close(committed)
	}()

	select {
	case <-committed:
		t.Fatal("commit completed while the compact ring was full")
	case <-time.After(25 * time.Millisecond):
	}

	packet, ok := shard.TryNext()
	require.True(t, ok)
	packet.Release()

	select {
	case <-committed:
	case <-time.After(time.Second):
		t.Fatal("commit did not unblock after release")
	}
}

func TestCompactRawIngressShardBackpressureTelemetry(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryMock := telemetryComponent.(telemetry.Mock)
	shard := NewCompactRawIngressShard(8, 8, telemetryComponent, "0")

	first, ok := shard.Reserve()
	require.True(t, ok)
	copy(first.Buffer(), []byte("abcdefgh"))
	first.Commit(8, RawPacketMeta{Source: UDS})

	second, ok := shard.Reserve()
	require.True(t, ok)
	copy(second.Buffer(), []byte("ijkl"))
	committed := make(chan struct{})
	go func() {
		second.Commit(4, RawPacketMeta{Source: UDS})
		close(committed)
	}()

	select {
	case <-committed:
		t.Fatal("commit completed while the compact ring was full")
	case <-time.After(25 * time.Millisecond):
	}

	packet, ok := shard.TryNext()
	require.True(t, ok)
	packet.Release()

	select {
	case <-committed:
	case <-time.After(time.Second):
		t.Fatal("commit did not unblock after release")
	}

	require.Greater(t, countValue(t, telemetryMock, "dogstatsd_ingress_ring", "blocked_ns", map[string]string{"shard": "0"}), float64(0))
	require.Equal(t, float64(1), countValue(t, telemetryMock, "dogstatsd_ingress_ring", "stats", map[string]string{"shard": "0", "stat": "blocked_appends"}))
	require.Equal(t, float64(1), countValue(t, telemetryMock, "dogstatsd_ingress_ring", "stats", map[string]string{"shard": "0", "stat": "backpressure_events"}))
}

func TestDirectCompactRawIngressShardReserveCommitReadRelease(t *testing.T) {
	shard := NewDirectCompactRawIngressShard(128, 64, nil, "0")

	reservation, ok := shard.Reserve()
	require.True(t, ok)
	copy(reservation.Buffer(), []byte("metric:1|c"))
	reservation.Commit(len("metric:1|c"), RawPacketMeta{Source: UDS, ListenerID: "uds-test", Origin: "origin", ProcessID: 42})

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "metric:1|c", string(packet.Contents))
	require.Equal(t, UDS, packet.Source)
	require.Equal(t, "uds-test", packet.ListenerID)
	require.Equal(t, "origin", packet.Origin)
	require.Equal(t, uint32(42), packet.ProcessID)

	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestDirectCompactRawIngressShardLagTelemetryExcludesInFlightReservation(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryMock := telemetryComponent.(telemetry.Mock)
	shard := NewDirectCompactRawIngressShard(128, 64, telemetryComponent, "0")

	reservation, ok := shard.Reserve()
	require.True(t, ok)
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_records", map[string]string{"shard": "0"}))
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_bytes", map[string]string{"shard": "0"}))

	copy(reservation.Buffer(), []byte("metric:1|c"))
	reservation.Commit(len("metric:1|c"), RawPacketMeta{Source: UDS})
	time.Sleep(time.Millisecond)

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, float64(1), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_records", map[string]string{"shard": "0"}))
	require.Equal(t, float64(len("metric:1|c")), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_bytes", map[string]string{"shard": "0"}))
	require.Greater(t, gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "oldest_record_age_ns", map[string]string{"shard": "0"}), float64(0))

	packet.Release()
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_records", map[string]string{"shard": "0"}))
	require.Equal(t, float64(0), gaugeValue(t, telemetryMock, "dogstatsd_ingress_ring", "consumer_lag_bytes", map[string]string{"shard": "0"}))
}

func TestDirectCompactRawIngressShardReclaimsUnusedReservationBytes(t *testing.T) {
	shard := NewDirectCompactRawIngressShard(20, 16, nil, "0")

	first, ok := shard.Reserve()
	require.True(t, ok)
	copy(first.Buffer(), []byte("aaaa"))
	first.Commit(4, RawPacketMeta{Source: UDS, ListenerID: "first"})

	second, ok := shard.Reserve()
	require.True(t, ok)
	copy(second.Buffer(), []byte("bbbb"))
	second.Commit(4, RawPacketMeta{Source: UDS, ListenerID: "second"})

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "aaaa", string(packet.Contents))
	packet.Release()

	packet, ok = shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "bbbb", string(packet.Contents))
	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestDirectCompactRawIngressShardAbortReclaimsReservation(t *testing.T) {
	shard := NewDirectCompactRawIngressShard(16, 16, nil, "0")

	first, ok := shard.Reserve()
	require.True(t, ok)
	first.Abort()

	second, ok := shard.Reserve()
	require.True(t, ok)
	copy(second.Buffer(), []byte("ok"))
	second.Commit(2, RawPacketMeta{Source: UDS})

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "ok", string(packet.Contents))
	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestDirectCompactRawIngressShardCommitAfterOlderRecordRelease(t *testing.T) {
	shard := NewDirectCompactRawIngressShard(32, 16, nil, "0")

	first, ok := shard.Reserve()
	require.True(t, ok)
	copy(first.Buffer(), []byte("aaaa"))
	first.Commit(4, RawPacketMeta{Source: UDS})

	second, ok := shard.Reserve()
	require.True(t, ok)
	copy(second.Buffer(), []byte("bbbb"))

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "aaaa", string(packet.Contents))
	packet.Release()

	second.Commit(4, RawPacketMeta{Source: UDS})
	packet, ok = shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "bbbb", string(packet.Contents))
	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestCompactRawIngressShardWrapsAndPreservesOrder(t *testing.T) {
	shard := NewCompactRawIngressShard(10, 8, nil, "0")
	appendPayload := func(payload string) {
		reservation, ok := shard.Reserve()
		require.True(t, ok)
		copy(reservation.Buffer(), []byte(payload))
		reservation.Commit(len(payload), RawPacketMeta{Source: UDS, ListenerID: payload})
	}

	appendPayload("aaaa")
	appendPayload("bbbb")

	packet, ok := shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "aaaa", string(packet.Contents))
	packet.Release()

	appendPayload("cccc")

	packet, ok = shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "bbbb", string(packet.Contents))
	packet.Release()

	packet, ok = shard.TryNext()
	require.True(t, ok)
	require.Equal(t, "cccc", string(packet.Contents))
	packet.Release()
	require.Equal(t, 0, shard.Len())
}

func TestCompactRawIngressShardsDistributeReservations(t *testing.T) {
	shards := NewCompactRawIngressShards(2, 128, 64, nil)

	first, ok := shards.Reserve()
	require.True(t, ok)
	first.Commit(1, RawPacketMeta{Source: UDS, ListenerID: "first"})

	second, ok := shards.Reserve()
	require.True(t, ok)
	second.Commit(1, RawPacketMeta{Source: UDS, ListenerID: "second"})

	packet, ok := shards.Shard(0).TryNext()
	require.True(t, ok)
	require.Equal(t, "first", packet.ListenerID)
	packet.Release()

	packet, ok = shards.Shard(1).TryNext()
	require.True(t, ok)
	require.Equal(t, "second", packet.ListenerID)
	packet.Release()
}

func TestCompactRawIngressShardTryNextBatchReleaseBatch(t *testing.T) {
	shard := NewCompactRawIngressShard(128, 16, nil, "0")
	for _, payload := range []string{"a", "bb", "ccc"} {
		reservation, ok := shard.Reserve()
		require.True(t, ok)
		copy(reservation.Buffer(), []byte(payload))
		reservation.Commit(len(payload), RawPacketMeta{Source: UDS, ListenerID: payload})
	}

	batch := shard.TryNextBatch(make([]RawPacket, 0, 2))
	require.Len(t, batch, 2)
	require.Equal(t, "a", string(batch[0].Contents))
	require.Equal(t, "bb", string(batch[1].Contents))
	shard.ReleaseBatch(len(batch))
	require.Equal(t, 1, shard.Len())

	batch = shard.TryNextBatch(batch[:0])
	require.Len(t, batch, 1)
	require.Equal(t, "ccc", string(batch[0].Contents))
	shard.ReleaseBatch(len(batch))
	require.Equal(t, 0, shard.Len())
}

func gaugeValue(t *testing.T, telemetryMock telemetry.Mock, subsystem string, name string, tags map[string]string) float64 {
	t.Helper()
	metrics, err := telemetryMock.GetGaugeMetric(subsystem, name)
	require.NoError(t, err)
	return metricValue(t, metrics, tags)
}

func countValue(t *testing.T, telemetryMock telemetry.Mock, subsystem string, name string, tags map[string]string) float64 {
	t.Helper()
	metrics, err := telemetryMock.GetCountMetric(subsystem, name)
	require.NoError(t, err)
	return metricValue(t, metrics, tags)
}

func metricValue(t *testing.T, metrics []telemetry.Metric, tags map[string]string) float64 {
	t.Helper()
	for _, metric := range metrics {
		if metricTagsMatch(metric.Tags(), tags) {
			return metric.Value()
		}
	}
	require.Failf(t, "metric not found", "tags=%v metrics=%v", tags, metrics)
	return 0
}

func metricTagsMatch(actual map[string]string, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}
