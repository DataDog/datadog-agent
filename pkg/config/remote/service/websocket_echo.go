// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/config/remote/rcwebsocket"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WebSocketTestActor periodically calls rcwebsocket.RunWebSocketTest() in a
// background task.
type WebSocketTestActor struct {
	client *api.HTTPClient
	// Callback to run the test.
	fn func(context.Context, *api.HTTPClient)

	stopCh chan struct{} // nil until Start() called
}

// NewWebSocketTestActor constructs a NewWebSocketTestActor that uses client to
// obtain a WebSocket connection to the RC backend.
func NewWebSocketTestActor(client *api.HTTPClient) *WebSocketTestActor {
	return &WebSocketTestActor{
		client: client,
		fn:     rcwebsocket.RunEchoTest,
	}
}

// Start runs this actor, spawning a background task to execute the test
// periodically.
//
// This method is not concurrency safe, and panics if Start() has previously
// been called.
func (s *WebSocketTestActor) Start() {
	if s.stopCh != nil {
		panic("attempt to start WebSocketTestActor more than once")
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
func (s *WebSocketTestActor) Stop() {
	if s.stopCh != nil { // CoreAgentService.Stop() calls more than once
		close(s.stopCh)
	}
}

func (s *WebSocketTestActor) run() {
	// This test loop is best effort - it SHOULD never kill the Agent process,
	// even if it encounters a bug or protocol error.
	//
	// If the test panics, this run() call exits gracefully and the test is not
	// performed again until the agent is restarted. Stop() may be called safely
	// after recovery has occurred.
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("unexpected websocket connectivity test failure: %s", err)
		}
	}()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

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

			s.fn(ctx, s.client)
		}()

		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
	}
}
