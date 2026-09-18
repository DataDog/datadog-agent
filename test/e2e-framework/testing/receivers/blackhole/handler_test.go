// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package blackhole

import (
	"bytes"
	process "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type zeros struct{ read int64 }

func (z *zeros) Read(p []byte) (int, error) { clear(p); z.read += int64(len(p)); return len(p), nil }

func TestHandlerStatelessIntake(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/api/v2/series", "/api/beta/sketches", "/api/v1/check_run", "/intake/", "/api/v2/logs", "/api/v0.2/traces", "/api/v2/contimage", "/api/v2/contlcycle", "/api/v2/sbom", "/api/v2/apmtelemetry", "/api/v2/agenthealth"} {
		for _, method := range []string{"POST", "PUT"} {
			req := httptest.NewRequest(method, path, bytes.NewBufferString("payload-not-retained"))
			req.Header.Set("DD-API-KEY", "do-not-log")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != 200 || rr.Body.String() != "{}" {
				t.Fatalf("%s %s: %d %s", method, path, rr.Code, rr.Body.String())
			}
		}
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/api/v1/validate", 200}, {"HEAD", "/health", 200}, {"GET", "/fakeintake/payloads", 404}, {"POST", "/api/v0.1/configurations", 404}, {"DELETE", "/api/v2/series", 405}} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
		if rr.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, rr.Code)
		}
	}
}
func TestHandlerBoundsStreamingBody(t *testing.T) {
	z := &zeros{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/series", io.NopCloser(z))
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(rr.Code)
	}
	if z.read > MaxBodyBytes+1 {
		t.Fatalf("read %d bytes beyond work limit", z.read)
	}
}

// Use the Agent forwarder's actual process-family routes and response decoder,
// not a generic HTTP 200 assertion: JSON is not a process acknowledgement.
func TestHandlerProcessAcknowledgements(t *testing.T) {
	h := Handler()
	for _, route := range []string{endpoints.ProcessesEndpoint.Route, endpoints.ProcessDiscoveryEndpoint.Route, endpoints.RtProcessesEndpoint.Route, endpoints.ContainerEndpoint.Route, endpoints.RtContainerEndpoint.Route, endpoints.ConnectionsEndpoint.Route, endpoints.LegacyOrchestratorEndpoint.Route, endpoints.OrchestratorEndpoint.Route, endpoints.OrchestratorManifestEndpoint.Route} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(method+route, func(t *testing.T) {
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, httptest.NewRequest(method, route, bytes.NewBufferString("discarded process payload")))
				require.Equal(t, http.StatusOK, rr.Code)
				require.Equal(t, "application/x-protobuf", rr.Header().Get("Content-Type"))
				msg, err := process.DecodeMessage(rr.Body.Bytes())
				require.NoError(t, err)
				require.Equal(t, process.MessageType(process.TypeResCollector), msg.Header.Type)
				ack, ok := msg.Body.(*process.ResCollector)
				require.True(t, ok)
				require.Empty(t, ack.Message)
				require.NotNil(t, ack.Status)
				require.Zero(t, ack.Status.ActiveClients)
			})
		}
	}
}
