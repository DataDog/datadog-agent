// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/client"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// StatusCodeContainer is a lock around the status code to return
type StatusCodeContainer struct {
	sync.Mutex
	statusCode int
}

// TestServer a test server
type TestServer struct {
	httpServer          *httptest.Server
	DestCtx             *client.DestinationsContext
	Destination         *Destination
	Endpoint            config.Endpoint
	request             *http.Request
	statusCodeContainer *StatusCodeContainer
	stopChan            chan struct{}
}

// NewTestServer creates a new test server
func NewTestServer(statusCode int) *TestServer {
	panic("not called")
}

// NewTestServerWithOptions creates a new test server with concurrency and response control
func NewTestServerWithOptions(statusCode int, senders int, retryDestination bool, respondChan chan int) *TestServer {
	panic("not called")
}

// Stop stops the server
func (s *TestServer) Stop() {
	panic("not called")
}

// ChangeStatus changes the status to return
func (s *TestServer) ChangeStatus(statusCode int) {
	panic("not called")
}
