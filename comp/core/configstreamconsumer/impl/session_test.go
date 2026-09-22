// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamconsumerimpl

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func newTestConsumer(t *testing.T) *consumer {
	t.Helper()
	c := &consumer{
		log:         pkglog.NewWrapper(2),
		telemetry:   telemetrymock.New(t),
		params:      configstreamconsumer.NewParams("test-agent", ""),
		readyCh:     make(chan struct{}),
		sessionKick: make(chan struct{}, 1),
	}
	c.initMetrics()
	return c
}

func TestInvalidateSessionWakesSessionLoop(t *testing.T) {
	c := newTestConsumer(t)
	c.setSession("session-1", 7*time.Second)
	require.Equal(t, "session-1", c.session())
	require.Equal(t, 7*time.Second, c.currentRefreshInterval())

	c.invalidateSession("rejected")
	require.Empty(t, c.session())
	select {
	case <-c.sessionKick:
	default:
		t.Fatal("invalidateSession should wake sessionLoop")
	}

	// Already invalidated, so there is nothing to wake up for.
	c.invalidateSession("rejected again")
	select {
	case <-c.sessionKick:
		t.Fatal("invalidateSession on an empty session should not signal")
	default:
	}
}

func TestCurrentRefreshIntervalFallsBackToDefault(t *testing.T) {
	c := newTestConsumer(t)
	c.setSession("session-1", 0)
	require.Equal(t, defaultRefreshInterval, c.currentRefreshInterval())
}

func TestApplySnapshotAfterStreamReset(t *testing.T) {
	snapshot := func(seqID int32) *pb.ConfigSnapshot {
		return &pb.ConfigSnapshot{SequenceId: seqID}
	}
	apply := func(t *testing.T, c *consumer, seqID int32) bool {
		t.Helper()
		applied, err := c.applySnapshot(snapshot(seqID))
		require.NoError(t, err)
		return applied
	}

	t.Run("lower sequence id is accepted on a new stream", func(t *testing.T) {
		c := newTestConsumer(t)
		require.True(t, apply(t, c, 100))
		require.Equal(t, int32(100), c.lastSeqID.Load())

		// A restarted core agent counts from zero again. connectAndStream resets the
		// sequence ID before every stream, so the fresh snapshot must win.
		c.lastSeqID.Store(seqIDUnset)
		require.True(t, apply(t, c, 3))
		require.Equal(t, int32(3), c.lastSeqID.Load())
	})

	t.Run("sequence id zero is accepted on a new stream", func(t *testing.T) {
		c := newTestConsumer(t)
		c.lastSeqID.Store(seqIDUnset)
		require.NoError(t, c.handleConfigEvent(&pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{Snapshot: snapshot(0)},
		}))
		require.Equal(t, int32(0), c.lastSeqID.Load())
		require.True(t, c.IsActive())
	})

	t.Run("stale snapshot within a stream is dropped", func(t *testing.T) {
		c := newTestConsumer(t)
		require.True(t, apply(t, c, 10))
		require.False(t, apply(t, c, 4))
		require.Equal(t, int32(10), c.lastSeqID.Load())
	})
}

func TestOnlySnapshotsSignalReadiness(t *testing.T) {
	event := func(seqID int32) *pb.ConfigEvent {
		return &pb.ConfigEvent{Event: &pb.ConfigEvent_Update{Update: &pb.ConfigUpdate{
			SequenceId: seqID,
			Setting:    &pb.ConfigSetting{Key: "log_level", Value: structpb.NewStringValue("info"), Source: string(pkgconfigmodel.SourceFile)},
		}}}
	}
	t.Cleanup(func() {
		configstreambootstrap.Config().UnsetForSource("log_level", pkgconfigmodel.SourceFile)
	})

	c := newTestConsumer(t)
	c.lastSeqID.Store(seqIDUnset)
	require.NoError(t, c.handleConfigEvent(event(0)))
	require.NoError(t, c.handleConfigEvent(event(1)))
	require.False(t, c.IsActive())

	require.NoError(t, c.handleConfigEvent(&pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{Snapshot: &pb.ConfigSnapshot{SequenceId: 2}},
	}))
	require.True(t, c.IsActive())
}

// A stale snapshot is not an apply, so it must not release waitForReady.
func TestStaleSnapshotDoesNotSignalReadiness(t *testing.T) {
	c := newTestConsumer(t)
	c.lastSeqID.Store(10)
	require.NoError(t, c.handleConfigEvent(&pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{Snapshot: &pb.ConfigSnapshot{SequenceId: 4}},
	}))
	require.False(t, c.IsActive())
}

func TestSessionRejected(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{status.Error(codes.NotFound, "no remote agent found with session ID"), true},
		{status.Error(codes.PermissionDenied, "session_id not found"), true},
		{status.Error(codes.Unauthenticated, "session_id required"), true},
		{fmt.Errorf("wrapped: %w", status.Error(codes.NotFound, "gone")), true},
		{status.Error(codes.Unavailable, "connection refused"), false},
		{status.Error(codes.DeadlineExceeded, "context deadline exceeded"), false},
		{errors.New("dial tcp: connect: connection refused"), false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, sessionRejected(tt.err), tt.err.Error())
	}
}
