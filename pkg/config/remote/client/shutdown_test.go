// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// blockingFetcher answers the first request, then blocks every later request
// until its context is cancelled, the way an in-flight update behaves when the
// client shuts down.
type blockingFetcher struct {
	mu       sync.Mutex
	calls    int
	inFlight chan struct{}
}

func (f *blockingFetcher) ClientGetConfigs(ctx context.Context, _ *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	f.mu.Lock()
	f.calls++
	first := f.calls == 1
	f.mu.Unlock()
	if first {
		return &pbgo.ClientGetConfigsResponse{}, nil
	}
	close(f.inFlight)
	<-ctx.Done()
	return nil, ctx.Err()
}

type stateRecorder struct {
	mu     sync.Mutex
	states []bool
}

func (r *stateRecorder) OnUpdate(map[string]state.RawConfig, func(string, state.ApplyStatus)) {}

func (r *stateRecorder) OnStateChange(s bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, s)
}

func (r *stateRecorder) ShouldIgnoreSignatureExpiration() bool { return false }

func (r *stateRecorder) got() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.states...)
}

// TestPollLoopStopsWithoutStateChangeWhenClosedDuringUpdate checks that closing
// the client while an update is in flight stops the poll loop without reporting
// a lost connection to the listeners.
func TestPollLoopStopsWithoutStateChangeWhenClosedDuringUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fetcher := &blockingFetcher{inFlight: make(chan struct{})}
		client, err := newClient(fetcher, WithoutTufVerification(), WithPollInterval(time.Millisecond))
		require.NoError(t, err)
		recorder := &stateRecorder{}
		client.SubscribeAll(state.ProductAgentConfig, recorder)

		client.Start()
		<-fetcher.inFlight // the first update succeeded; the second one is in flight
		client.Close()
		synctest.Wait() // the poll loop has returned

		assert.Empty(t, recorder.got(), "closing the client must not report a state change")
	})
}
