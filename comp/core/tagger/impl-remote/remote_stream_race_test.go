// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// hangingStream's Recv() only returns once its context is canceled, like a
// real gRPC stream. unblockedAt records when that happened.
type hangingStream struct {
	grpc.ClientStream

	ctx         context.Context
	unblockedAt chan time.Time
}

func (s *hangingStream) Recv() (*pb.StreamTagsResponse, error) {
	<-s.ctx.Done()
	s.unblockedAt <- time.Now()
	return nil, s.ctx.Err()
}

type hangingStreamClient struct {
	pb.AgentSecureClient

	streams chan *hangingStream
}

func (c *hangingStreamClient) TaggerStreamEntities(ctx context.Context, _ *pb.StreamTagsRequest, _ ...grpc.CallOption) (pb.AgentSecure_TaggerStreamEntitiesClient, error) {
	s := &hangingStream{ctx: ctx, unblockedAt: make(chan time.Time, 1)}
	c.streams <- s
	return s, nil
}

// TestRun_StreamRecvTimeoutUnblocksAbandonedRecv checks that once run() gives
// up on a Recv() call after streamRecvTimeout, the abandoned call actually
// unblocks promptly (via streamCancel) instead of hanging forever.
func TestRun_StreamRecvTimeoutUnblocksAbandonedRecv(t *testing.T) {
	originalTimeout := streamRecvTimeout
	streamRecvTimeout = 50 * time.Millisecond
	t.Cleanup(func() { streamRecvTimeout = originalTimeout })

	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)

	client := &hangingStreamClient{streams: make(chan *hangingStream, 4)}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rt := &remoteTagger{
		store:           newTagStore(telemetryStore),
		telemetryStore:  telemetryStore,
		filter:          types.NewMatchAllFilter(),
		log:             logmock.New(t),
		client:          client,
		ctx:             ctx,
		cancel:          cancel,
		telemetryTicker: time.NewTicker(time.Hour),
	}
	t.Cleanup(rt.telemetryTicker.Stop)

	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.run()
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	var stream *hangingStream
	select {
	case stream = <-client.streams:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the tagger stream to be established")
	}

	start := time.Now()
	select {
	case unblockedAt := <-stream.unblockedAt:
		assert.Less(t, unblockedAt.Sub(start), 5*time.Second,
			"the abandoned Recv() call should unblock promptly once its stream context is canceled")
	case <-time.After(5 * time.Second):
		t.Fatal("Recv() never unblocked after run() should have canceled the stream context on timeout")
	}
}

// foreverBlockingStream's Recv() never returns and never touches ctx, so the
// only shared state its goroutine can race on is t.stream itself. (Using
// ctx.Done() here would introduce incidental synchronization via context's
// internal mutex and mask the race.)
type foreverBlockingStream struct {
	grpc.ClientStream
}

func (*foreverBlockingStream) Recv() (*pb.StreamTagsResponse, error) {
	<-make(chan struct{})
	return nil, errors.New("unreachable")
}

type foreverBlockingClient struct {
	pb.AgentSecureClient
}

func (*foreverBlockingClient) TaggerStreamEntities(context.Context, *pb.StreamTagsRequest, ...grpc.CallOption) (pb.AgentSecure_TaggerStreamEntitiesClient, error) {
	return &foreverBlockingStream{}, nil
}

// Regression test for the t.stream data race found in staging. Detection is
// timing-dependent, so it runs many trials to make a miss on unfixed code unlikely.
func TestRun_AbandonedRecvDoesNotRaceWithStreamField(t *testing.T) {
	originalTimeout := streamRecvTimeout
	streamRecvTimeout = 5 * time.Millisecond
	t.Cleanup(func() { streamRecvTimeout = originalTimeout })

	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	logger := logmock.New(t)

	const (
		batches      = 25
		trialsPerRun = 40
	)
	for b := 0; b < batches; b++ {
		var wg sync.WaitGroup
		for i := 0; i < trialsPerRun; i++ {
			wg.Go(func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				rt := &remoteTagger{
					store:           newTagStore(telemetryStore),
					telemetryStore:  telemetryStore,
					filter:          types.NewMatchAllFilter(),
					log:             logger,
					client:          &foreverBlockingClient{},
					ctx:             ctx,
					cancel:          cancel,
					telemetryTicker: time.NewTicker(time.Hour),
				}
				defer rt.telemetryTicker.Stop()

				go rt.run()
				// Let run() hit the Recv() timeout and write to
				// t.stream/t.ready, leaving the Recv() goroutine running.
				time.Sleep(15 * time.Millisecond)
			})
		}
		wg.Wait()
	}
}
