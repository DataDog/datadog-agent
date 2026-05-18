// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
