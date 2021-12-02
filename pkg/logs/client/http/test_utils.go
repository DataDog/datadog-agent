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

type StatusCodeContainer struct {
	sync.Mutex
	statusCode int
}
type HTTPServerTest struct {
	httpServer          *httptest.Server
	destCtx             *client.DestinationsContext
	destination         *Destination
	Endpoint            config.Endpoint
	request             *http.Request
	statusCodeContainer *StatusCodeContainer
}

func NewHTTPServerTest(statusCode int) *HTTPServerTest {
	statusCodeContainer := &StatusCodeContainer{statusCode: statusCode}
	var request http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCodeContainer.Lock()
		w.WriteHeader(statusCodeContainer.statusCode)
		statusCodeContainer.Unlock()
		request = *r
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
	dest := NewDestination(endpoint, JSONContentType, destCtx, 0, true, 0)
	return &HTTPServerTest{
		httpServer:          ts,
		destCtx:             destCtx,
		destination:         dest,
		Endpoint:            endpoint,
		request:             &request,
		statusCodeContainer: statusCodeContainer,
	}
}

func (s *HTTPServerTest) stop() {
	s.destCtx.Start()
	s.httpServer.Close()
}

func (s *HTTPServerTest) ChangeStatus(statusCode int) {
	s.statusCodeContainer.Lock()
	s.statusCodeContainer.statusCode = statusCode
	s.statusCodeContainer.Unlock()
}
