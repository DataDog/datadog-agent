// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/config/remote/rcecho"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EchoTestActor periodically calls rcecho.RunTransportTests() in a background
// task, exercising WebSocket, gRPC and TCP connectivity to the RC backend.
type EchoTestActor struct {
	client *api.HTTPClient
	// Callback to run the test.
	fn func(context.Context, *api.HTTPClient, uint64)

	stopCh   chan struct{} // nil until Start() called
	stopOnce sync.Once
}

// NewEchoTestActor constructs an EchoTestActor that uses client to run echo
// tests against the RC backend over all supported transports.
func NewEchoTestActor(client *api.HTTPClient) *EchoTestActor {
	return &EchoTestActor{
		client: client,
		fn:     rcecho.RunTransportTests,
	}
}

// Start runs this actor, spawning a background task to execute the test
// periodically.
//
// This method is not concurrency safe, and panics if Start() has previously
// been called.
func (s *EchoTestActor) Start() {
	if s.stopCh != nil {
		panic("attempt to start EchoTestActor more than once")
	}

	s.stopCh = make(chan struct{})

	go func() {
		s.run()
	}()
}

// Stop signals the background task to stop asynchronously.
//
// This method is not concurrency safe, and panics if Start() has not previously
// been called. It is safe to call Stop() repeatedly.
func (s *EchoTestActor) Stop() {
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
}

func (s *EchoTestActor) run() {
	// This test loop is best effort - it SHOULD never kill the Agent process,
	// even if it encounters a bug or protocol error.
	//
	// If the test panics, this run() call exits gracefully and the test is not
	// performed again until the agent is restarted. Stop() may be called safely
	// after recovery has occurred.
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected echo connectivity test failure: %s", err)
		}
	}()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	runCount := uint64(0)
	for {
		func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Cancel ctx when the actor is stopped, aborting the test.
			go func() {
				select {
				case <-s.stopCh:
					cancel()
				case <-ctx.Done():
				}
			}()

			s.fn(ctx, s.client, runCount)
		}()

		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}

		runCount++
	}
}
