// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package telemetry

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
)

const (
	//nolint:revive // TODO(TEL) Fix revive linter
	testRcClientId = "YgRPh8NqGkvhXq71FvxVN"
	//nolint:revive // TODO(TEL) Fix revive linter
	testKubernetesClusterId = "2cb68cff-935e-4d09-8e57-7c2c5e0364d6"
)

func getTestApmRemoteConfigEvent() ApmRemoteConfigEvent {
	return ApmRemoteConfigEvent{
		Payload: ApmRemoteConfigEventPayload{
			EventName: "agent.k8s.mutate",
			Tags: ApmRemoteConfigEventTags{
				Env:        "staging",
				RcId:       "24e27a6f0e7c5d5ef303bef2e26e960090f3f54c95fc3543676c0139b287552e",
				RcClientId: testRcClientId,
				RcRevision: 1685457769594355579,
				RcVersion:  1,
			},
			Error: ApmRemoteConfigEventError{
				Code:    0,
				Message: "",
			},
		},
	}

}

func TestTelemetryPath(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	collector := NewCollector(testRcClientId, testKubernetesClusterId)
	collector.SetTestHost(server.URL)
	config.Datadog().SetWithoutSource("api_key", "dummy")

	var reqCount int
	var path string
	server.assertReq = func(req *http.Request) {
		//nolint:revive // TODO(TEL) Fix revive linter
		reqCount += 1
		path = req.URL.Path
	}

	collector.SendRemoteConfigMutateEvent(getTestApmRemoteConfigEvent())

	assert.Equal(t, 1, reqCount)
	assert.Equal(t, "/api/v2/apmtelemetry", path)
}

type testServer struct {
	server    *httptest.Server
	URL       string
	assertReq func(*http.Request)
}

func newTestServer() *testServer {
	srv := &testServer{}
	srv.server = httptest.NewServer(srv)
	srv.URL = srv.server.URL
	return srv
}

// ServeHTTP responds based on the request body.
func (ts *testServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if ts.assertReq != nil {
		ts.assertReq(req)
	}
	_, err := io.ReadAll(req.Body)
	if err != nil {
		panic(fmt.Sprintf("error reading request body: %v", err))
	}
	req.Body.Close()
	w.WriteHeader(202)
}

// Close closes the underlying http.Server.
func (ts *testServer) Close() { ts.server.Close() }
