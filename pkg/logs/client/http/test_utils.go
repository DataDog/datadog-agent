// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/client"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
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
}

// NewTestServer creates a new test server
func NewTestServer(statusCode int) *TestServer {
	return NewTestServerWithOptions(statusCode, 0, true, nil)
}

// NewTestServerWithOptions creates a new test server with concurrency and response control
func NewTestServerWithOptions(statusCode int, senders int, retryDestination bool, respondChan chan struct{}) *TestServer {
	statusCodeContainer := &StatusCodeContainer{statusCode: statusCode}
	var request http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCodeContainer.Lock()
		w.WriteHeader(statusCodeContainer.statusCode)
		request = *r
		if respondChan != nil {
			respondChan <- struct{}{}
		}
		statusCodeContainer.Unlock()
	}))
	url := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(url[2])
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	endpoint := config.Endpoint{
		APIKey: "test",
		Host:   strings.Replace(url[1], "/", "", -1),
		Port:   port,
		UseSSL: false,
	}
	dest := NewDestination(endpoint, JSONContentType, destCtx, senders, retryDestination, 0)
	return &TestServer{
		httpServer:          ts,
		DestCtx:             destCtx,
		Destination:         dest,
		Endpoint:            endpoint,
		request:             &request,
		statusCodeContainer: statusCodeContainer,
	}
}

// Stop stops the server
func (s *TestServer) Stop() {
	s.DestCtx.Stop()
	s.httpServer.Close()
}

// ChangeStatus changes the status to return
func (s *TestServer) ChangeStatus(statusCode int) {
	s.statusCodeContainer.Lock()
	s.statusCodeContainer.statusCode = statusCode
	s.statusCodeContainer.Unlock()
}
