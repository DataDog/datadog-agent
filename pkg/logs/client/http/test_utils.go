// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package http

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"

	"golang.org/x/net/http2"
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
func NewTestServer(statusCode int, cfg pkgconfigmodel.Reader) *TestServer {
	return NewTestServerWithOptions(statusCode, 1, true, nil, cfg)
}

// NewTestServerWithOptions creates a new test server with concurrency and response control
func NewTestServerWithOptions(statusCode int, concurrentSends int, retryDestination bool, respondChan chan int, cfg pkgconfigmodel.Reader) *TestServer {
	statusCodeContainer := &StatusCodeContainer{statusCode: statusCode}
	var request http.Request
	var mu = sync.Mutex{}
	var stopChan = make(chan struct{}, 1)
	stopped := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCodeContainer.Lock()
		code := statusCodeContainer.statusCode
		w.WriteHeader(statusCodeContainer.statusCode)
		statusCodeContainer.Unlock()
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}

		request = *r
		if respondChan != nil {
			select {
			case respondChan <- code:
			case <-stopChan:
				stopped = true
			}
		}
		mu.Unlock()
	}))
	url := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(url[2])
	destCtx := client.NewDestinationsContext()
	destCtx.Start()

	endpoint := config.NewEndpoint("test", "", strings.ReplaceAll(url[1], "/", ""), port, config.EmptyPathPrefix, false)
	endpoint.BackoffFactor = 1
	endpoint.BackoffBase = 0.01
	endpoint.BackoffMax = 10
	endpoint.RecoveryInterval = 1

	dest := NewDestination(endpoint, JSONContentType, destCtx, retryDestination, client.NewNoopDestinationMetadata(), cfg, concurrentSends, concurrentSends, metrics.NewNoopPipelineMonitor(""), "test")
	return &TestServer{
		httpServer:          ts,
		DestCtx:             destCtx,
		Destination:         dest,
		Endpoint:            endpoint,
		request:             &request,
		statusCodeContainer: statusCodeContainer,
		stopChan:            stopChan,
	}
}

// NewTestHTTPSServer creates a new test server that can support HTTP/2
func NewTestHTTPSServer(forceHTTP1 bool) *httptest.Server {
	// Create an HTTP/2 test server
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("protocol", r.Proto)
		w.WriteHeader(http.StatusOK)
	}))

	// Configure the server to support HTTP/2
	if !forceHTTP1 {
		err := http2.ConfigureServer(testServer.Config, &http2.Server{})
		if err != nil {
			panic(err)
		}
		testServer.TLS = testServer.Config.TLSConfig
	}
	// Start the server with TLS
	testServer.StartTLS()

	return testServer
}

// Stop stops the server
func (s *TestServer) Stop() {
	s.stopChan <- struct{}{}
	s.DestCtx.Stop()
	s.httpServer.Close()
}

// ChangeStatus changes the status to return
func (s *TestServer) ChangeStatus(statusCode int) {
	s.statusCodeContainer.Lock()
	s.statusCodeContainer.statusCode = statusCode
	s.statusCodeContainer.Unlock()
}
