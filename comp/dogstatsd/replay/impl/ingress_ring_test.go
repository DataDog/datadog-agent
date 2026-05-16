// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package replayimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

func TestMilestone5IngressRingKeepsNewestBoundedCopies(t *testing.T) {
	ring := newIngressRing(2)
	now := time.Unix(100, 0)
	payload := []byte("first")
	ancillary := []byte("cred")

	require.True(t, ring.append(replay.IngressEnvelope{
		Timestamp: now,
		Source:    packets.UDS,
		Payload:   payload,
		Ancillary: ancillary,
	}))
	payload[0] = 'F'
	ancillary[0] = 'C'

	require.True(t, ring.append(replay.IngressEnvelope{Timestamp: now.Add(time.Second), Source: packets.UDP, Payload: []byte("second")}))
	require.True(t, ring.append(replay.IngressEnvelope{Timestamp: now.Add(2 * time.Second), Source: packets.NamedPipe, Payload: []byte("third")}))

	stats := ring.stats()
	assert.Equal(t, 2, stats.Capacity)
	assert.Equal(t, 2, stats.Retained)
	assert.Equal(t, uint64(1), stats.Dropped)

	snapshot := ring.snapshot(0)
	require.Len(t, snapshot, 2)
	assert.Equal(t, []byte("second"), snapshot[0].Payload)
	assert.Equal(t, packets.UDP, snapshot[0].Source)
	assert.Equal(t, []byte("third"), snapshot[1].Payload)
	assert.Equal(t, packets.NamedPipe, snapshot[1].Source)

	latest := ring.snapshot(1)
	require.Len(t, latest, 1)
	assert.Equal(t, []byte("third"), latest[0].Payload)
}

func TestMilestone5TrafficCaptureRecordsRecentIngressWhenCaptureStopped(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("dogstatsd_capture_depth", 1)
	cfg.SetWithoutSource("dogstatsd_capture_raw_ring_size", 2)

	tc := &trafficCapture{
		config: cfg,
		tagger: taggerfxmock.SetupFakeTagger(t),
	}
	require.NoError(t, tc.configure(context.Background()))

	payload := []byte("metric:1|c")
	recorded := tc.CaptureIngress(replay.IngressEnvelope{
		Timestamp:  time.Unix(100, 0),
		Source:     packets.UDP,
		ListenerID: "udp",
		Payload:    payload,
		RemoteAddr: "127.0.0.1:1234",
		LocalAddr:  "127.0.0.1:8125",
	})
	require.True(t, recorded)
	payload[0] = 'M'

	recent := tc.RecentIngress(10)
	require.Len(t, recent, 1)
	assert.Equal(t, []byte("metric:1|c"), recent[0].Payload)
	assert.Equal(t, packets.UDP, recent[0].Source)
	assert.Equal(t, "127.0.0.1:1234", recent[0].RemoteAddr)
	assert.Equal(t, replay.IngressStats{Capacity: 2, Retained: 1}, tc.IngressStats())
}

func TestMilestone5TrafficCaptureIngressUsesExistingCaptureMessageShape(t *testing.T) {
	writer := NewTrafficCaptureWriter(1, taggerfxmock.SetupFakeTagger(t))
	writer.Lock()
	writer.accepting = true
	writer.Unlock()

	timestamp := time.Unix(123, 456)
	payload := []byte("metric:2|c")
	ancillary := []byte("ancillary")
	require.True(t, writer.EnqueueIngress(replay.IngressEnvelope{
		Timestamp: timestamp,
		Source:    packets.UDS,
		Payload:   payload,
		ProcessID: 42,
		Origin:    "container_id://abc",
		Ancillary: ancillary,
	}))
	payload[0] = 'M'
	ancillary[0] = 'A'

	msg := <-writer.Traffic
	assert.Equal(t, timestamp.UnixNano(), msg.Pb.Timestamp)
	assert.Equal(t, int32(len("metric:2|c")), msg.Pb.PayloadSize)
	assert.Equal(t, []byte("metric:2|c"), msg.Pb.Payload)
	assert.Equal(t, int32(42), msg.Pb.Pid)
	assert.Equal(t, int32(len("ancillary")), msg.Pb.AncillarySize)
	assert.Equal(t, []byte("ancillary"), msg.Pb.Ancillary)
	assert.Equal(t, int32(42), msg.Pid)
	assert.Equal(t, "container_id://abc", msg.ContainerID)
}

func BenchmarkMilestone5CaptureIngress(b *testing.B) {
	envelope := replay.IngressEnvelope{
		Timestamp: time.Unix(100, 0),
		Source:    packets.UDP,
		Payload:   []byte("metric:1|c"),
	}

	b.Run("disabled_no_ring", func(b *testing.B) {
		tc := &trafficCapture{writer: NewTrafficCaptureWriter(0, nil)}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tc.CaptureIngress(envelope)
		}
	})

	b.Run("raw_ring_enabled", func(b *testing.B) {
		tc := &trafficCapture{
			writer:     NewTrafficCaptureWriter(0, nil),
			rawIngress: newIngressRing(1024),
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tc.CaptureIngress(envelope)
		}
	})
}
