// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// flakyClient opens streams that answer responses times and then fail, as a
// server lacking the service does, and records when it opens them.
type flakyClient struct {
	responses int

	mu     sync.Mutex
	opened []time.Time
}

func (c *flakyClient) StreamEntities(ctx context.Context) (Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// past a spin, let the fake time run out
	if len(c.opened) == 1000 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.opened = append(c.opened, time.Now())
	return &flakyStream{responses: c.responses}, nil
}

func (c *flakyClient) gaps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	var gaps []time.Duration
	for i := 1; i < len(c.opened); i++ {
		gaps = append(gaps, c.opened[i].Sub(c.opened[i-1]))
	}
	return gaps
}

type flakyStream struct {
	responses int
}

func (s *flakyStream) Recv() (interface{}, error) {
	if s.responses == 0 {
		return nil, status.Error(codes.Unimplemented, "unknown service")
	}
	s.responses--
	return struct{}{}, nil
}

type nopHandler struct{}

func (nopHandler) Port() int                                                          { return 0 }
func (nopHandler) Address() string                                                    { return "" }
func (nopHandler) IsEnabled() bool                                                    { return true }
func (nopHandler) Credentials() credentials.TransportCredentials                      { return nil }
func (nopHandler) NewClient(grpc.ClientConnInterface) GrpcClient                      { return nil }
func (nopHandler) IsResyncComplete(interface{}) bool                                  { return true }
func (nopHandler) HandleResync(workloadmeta.Component, []workloadmeta.CollectorEvent) {}

func (nopHandler) HandleResponse(workloadmeta.Component, interface{}) ([]workloadmeta.CollectorEvent, error) {
	return nil, nil
}

// runFor runs a collector streaming from client for d of fake time and
// returns the delays between the streams it opened.
func runFor(t *testing.T, client *flakyClient, d time.Duration) []time.Duration {
	cfg := configmock.New(t)
	ipc := ipcmock.New(t)
	var gaps []time.Duration
	synctest.Test(t, func(*testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		collector := &GenericCollector{
			CollectorID:   "test",
			StreamHandler: nopHandler{},
			Config:        cfg,
			IPC:           ipc,
			client:        client,
			ctx:           ctx,
			cancel:        cancel,
			// as after a reconnect, so that responses go to HandleResync
			resyncNeeded: true,
		}
		done := make(chan struct{})
		go func() {
			collector.Run()
			close(done)
		}()

		time.Sleep(d)
		cancel()
		<-done
		gaps = client.gaps()
	})
	return gaps
}

func TestRunBacksOffFailingStreams(t *testing.T) {
	gaps := runFor(t, &flakyClient{}, time.Minute)

	require.GreaterOrEqual(t, len(gaps), 4)
	assert.GreaterOrEqual(t, slices.Min(gaps), 250*time.Millisecond)
	assert.Greater(t, gaps[len(gaps)-1], gaps[0])
}

func TestRunResetsBackoffOnResponse(t *testing.T) {
	gaps := runFor(t, &flakyClient{responses: 1}, time.Minute)

	require.GreaterOrEqual(t, len(gaps), 60)
	assert.LessOrEqual(t, slices.Max(gaps), 750*time.Millisecond)
}
