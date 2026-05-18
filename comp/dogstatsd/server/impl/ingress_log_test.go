// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/stretchr/testify/require"
)

func TestPacketIngressLogPreservesBatchOrder(t *testing.T) {
	log := newPacketIngressLog(1024*1024, nil)
	first := testPacketBatch("first")
	second := testPacketBatch("second")

	require.True(t, log.append(first))
	require.True(t, log.append(second))

	got, ok := log.next()
	require.True(t, ok)
	require.Equal(t, "first", string(got[0].Contents))

	got, ok = log.next()
	require.True(t, ok)
	require.Equal(t, "second", string(got[0].Contents))
}

func TestPacketIngressLogBlocksWhenByteBoundIsReached(t *testing.T) {
	first := testPacketBatch("first")
	second := testPacketBatch("second")
	log := newPacketIngressLog(packetBatchSizeBytes(first), nil)

	require.True(t, log.append(first))

	appended := make(chan struct{})
	go func() {
		require.True(t, log.append(second))
		close(appended)
	}()

	select {
	case <-appended:
		t.Fatal("append completed while the ingress log was still full")
	case <-time.After(25 * time.Millisecond):
	}

	got, ok := log.next()
	require.True(t, ok)
	require.Equal(t, "first", string(got[0].Contents))

	select {
	case <-appended:
	case <-time.After(time.Second):
		t.Fatal("append did not unblock after a batch was consumed")
	}

	got, ok = log.next()
	require.True(t, ok)
	require.Equal(t, "second", string(got[0].Contents))
}

func testPacketBatch(payload string) packets.Packets {
	buf := []byte(payload)
	return packets.Packets{{
		Contents:   buf,
		Buffer:     buf,
		ListenerID: "test",
		Source:     packets.UDS,
	}}
}
