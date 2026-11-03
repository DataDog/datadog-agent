// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type recordingPipelineMonitor struct {
	*metrics.NoopPipelineMonitor
	stopped chan struct{}
}

func (m *recordingPipelineMonitor) Stop() { close(m.stopped) }

func TestSenderStopHaltsMonitorBeforeWorkerJoin(t *testing.T) {
	cfg := configmock.New(t)
	respond := make(chan int)
	server := http.NewTestServerWithOptions(500, 1, true, respond, cfg)
	destinationFactory := func(_ string) *client.Destinations {
		return client.NewDestinations([]client.Destination{server.Destination}, nil)
	}

	mon := &recordingPipelineMonitor{NoopPipelineMonitor: metrics.NewNoopPipelineMonitor(""), stopped: make(chan struct{})}
	s := NewSender(cfg, &testAuditor{output: make(chan *message.Payload, 1)}, destinationFactory, 10, NewMockServerlessMeta(false), 1, 1, mon)
	s.Start()

	s.In() <- &message.Payload{}
	<-respond
	<-respond // looping on 500: the worker is wedged, so the worker join would block

	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()

	select {
	case <-mon.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline monitor must be stopped before the wedged worker join")
	}

	server.ChangeStatus(200)
	for {
		if <-respond == 200 {
			break
		}
	}
	<-done
	server.Stop()
}

func TestNewSenderWorkerDistribution(t *testing.T) {
	tests := []struct {
		name            string
		workersPerQueue int
		queuesCount     int
		expectedWorkers int
	}{
		{
			name:            "default worker count",
			workersPerQueue: DefaultWorkersPerQueue,
			queuesCount:     DefaultQueuesCount,
			expectedWorkers: 1,
		},
		{
			name:            "custom worker count",
			workersPerQueue: 3,
			queuesCount:     2,
			expectedWorkers: 6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config := configmock.New(t)
			destinations := &client.Destinations{}
			destFactory := func(_ string) *client.Destinations { return destinations }
			bufferSize := 100
			pipelineMonitor := metrics.NewNoopPipelineMonitor("test")

			// Create sender
			sender := NewSender(
				config,
				&NoopSink{},
				destFactory,
				bufferSize,
				NewMockServerlessMeta(false),
				tc.queuesCount,
				tc.workersPerQueue,
				pipelineMonitor,
			)

			assert.Equal(t, tc.expectedWorkers, len(sender.workers))

			chanMap := make(map[chan *message.Payload]struct{})
			for range 20 {
				chanMap[sender.In()] = struct{}{}
			}

			assert.Equal(t, tc.queuesCount, len(chanMap))
		})
	}
}

func TestPendingSendsWaitEndingEarlyDoesNotBlockAdds(t *testing.T) {
	p := newPendingSends()
	// Nothing is pending, so the wait returns right away
	p.WaitPendingSends(context.Background())

	p.AddPendingSend()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// A send is pending, so the wait returns only when ctx is done
	p.WaitPendingSends(ctx)

	added := make(chan struct{})
	go func() {
		p.AddPendingSend()
		close(added)
	}()
	select {
	case <-added:
	case <-time.After(time.Second):
		assert.Fail(t, "AddPendingSend blocked after a wait ended early")
	}

	waited := make(chan struct{})
	go func() {
		p.WaitPendingSends(context.Background())
		close(waited)
	}()
	p.PendingSendDone()
	select {
	case <-waited:
		assert.Fail(t, "the wait returned while a send was still pending")
	case <-time.After(50 * time.Millisecond):
	}
	p.PendingSendDone()
	select {
	case <-waited:
	case <-time.After(time.Second):
		assert.Fail(t, "the wait did not return once all sends were done")
	}
}

func TestDisabledServerlessMetaPendingSendsAreHarmless(t *testing.T) {
	for _, meta := range []ServerlessMeta{NewServerlessMeta(false), NewMockServerlessMeta(false)} {
		assert.NotPanics(t, func() {
			meta.AddPendingSend()
			meta.PendingSendDone()
			meta.WaitPendingSends(context.Background())
		})
	}
}
