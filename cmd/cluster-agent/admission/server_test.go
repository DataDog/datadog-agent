// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
)

func TestIsProbe(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{
			name:     "object with probe label",
			raw:      `{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`,
			expected: true,
		},
		{
			name:     "object without probe label",
			raw:      `{"metadata":{"labels":{"app":"nginx"}}}`,
			expected: false,
		},
		{
			name:     "object with no labels",
			raw:      `{"metadata":{}}`,
			expected: false,
		},
		{
			name:     "invalid JSON",
			raw:      `not json`,
			expected: false,
		},
		{
			name:     "empty object",
			raw:      `{}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isProbe([]byte(tt.raw)))
		})
	}
}

func TestProbeResponse_NonProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"app":"nginx"}}}`)
	resp := probeResponse(raw)
	assert.Nil(t, resp)
}

func TestProbeResponse_ProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`)
	resp := probeResponse(raw)
	require.NotNil(t, resp)

	assert.True(t, resp.Allowed)
	assert.NotNil(t, resp.PatchType)
	assert.Equal(t, admiv1.PatchTypeJSONPatch, *resp.PatchType)

	var patch []map[string]interface{}
	err := json.Unmarshal(resp.Patch, &patch)
	require.NoError(t, err)
	require.Len(t, patch, 1)
	assert.Equal(t, "add", patch[0]["op"])
	assert.Equal(t, "/metadata/annotations", patch[0]["path"])

	annotations, ok := patch[0]["value"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "true", annotations[admicommon.ProbeReceivedAnnotationKey])
}

// newTestServer skips NewServer to avoid leaking a health.RegisterReadiness
// goroutine that these tests never drain or deregister.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{mux: http.NewServeMux()}
	s.initDecoder()
	return s
}

// countingFiller hands out up to `remaining` bytes without allocating them
// up front, and records how many were actually read.
type countingFiller struct {
	remaining int64
	read      int64
}

func (f *countingFiller) Read(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > f.remaining {
		n = int(f.remaining)
	}
	for i := range p[:n] {
		p[i] = 'a'
	}
	f.remaining -= int64(n)
	f.read += int64(n)
	return n, nil
}

func TestHandleBodyLimits(t *testing.T) {
	// Well over maxRequestBodyBytes (7 MiB), but small enough to not stress the runner.
	const oversizedBodySize = 32 << 20

	validReview := []byte(`{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test-uid"}}`)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           []byte
		oversized      bool
		wantStatus     int
		wantWebhookHit bool
	}{
		{
			name:        "oversized body is rejected before being fully buffered",
			method:      http.MethodPost,
			contentType: jsonContentType,
			oversized:   true,
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "oversized body with wrong content type is rejected without reading",
			method:      http.MethodPost,
			contentType: "text/plain",
			oversized:   true,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized body with wrong method is rejected without reading",
			method:      http.MethodGet,
			contentType: jsonContentType,
			oversized:   true,
			wantStatus:  http.StatusMethodNotAllowed,
		},
		{
			name:           "valid v1 review is accepted",
			method:         http.MethodPost,
			contentType:    jsonContentType,
			body:           validReview,
			wantStatus:     http.StatusOK,
			wantWebhookHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			hit := false
			s.Register("/injectconfig", "test", admicommon.MutatingWebhook, func(_ *Request) *admiv1.AdmissionResponse {
				hit = true
				return &admiv1.AdmissionResponse{Allowed: true}
			}, nil, nil)

			var body io.Reader
			var filler *countingFiller
			if tt.oversized {
				filler = &countingFiller{remaining: oversizedBodySize}
				body = filler
			} else {
				body = bytes.NewReader(tt.body)
			}

			req := httptest.NewRequest(tt.method, "/injectconfig", body)
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			s.mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantWebhookHit, hit)
			if filler != nil {
				// Handler must not buffer meaningfully more than the cap.
				assert.LessOrEqual(t, filler.read, maxRequestBodyBytes()+int64(64<<10),
					"handler buffered more than the configured cap")
			}
		})
	}
}

func TestHandleBodyLimitAppliesToEveryRoute(t *testing.T) {
	s := newTestServer(t)
	for _, uri := range []string{"/injectconfig", "/autoscaling"} {
		s.Register(uri, "test", admicommon.MutatingWebhook, func(*Request) *admiv1.AdmissionResponse {
			t.Fatalf("webhook func should not be called for an oversized request to %s", uri)
			return nil
		}, nil, nil)
	}

	for _, uri := range []string{"/injectconfig", "/autoscaling"} {
		filler := &countingFiller{remaining: 32 << 20}
		req := httptest.NewRequest(http.MethodPost, uri, filler)
		req.Header.Set("Content-Type", jsonContentType)
		w := httptest.NewRecorder()

		s.mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "route %s", uri)
		assert.LessOrEqual(t, filler.read, maxRequestBodyBytes()+int64(64<<10), "route %s", uri)
	}
}
