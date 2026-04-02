// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package apiserver

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
)

func TestParseKubePath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantResource string
		wantTemplate string
	}{
		{"core list", "/api/v1/pods", "pods", "/api/v1/pods"},
		{"core get", "/api/v1/pods/my-pod", "pods", "/api/v1/pods/{name}"},
		{"namespaced list", "/api/v1/namespaces/default/pods", "pods", "/api/v1/namespaces/{namespace}/pods"},
		{"namespaced get", "/api/v1/namespaces/default/pods/my-pod", "pods", "/api/v1/namespaces/{namespace}/pods/{name}"},
		{"namespaces list", "/api/v1/namespaces", "namespaces", "/api/v1/namespaces"},
		{"namespace get", "/api/v1/namespaces/kube-system", "namespaces", "/api/v1/namespaces/{name}"},
		{"grouped namespaced get", "/apis/apps/v1/namespaces/kube-system/deployments/nginx", "deployments", "/apis/apps/v1/namespaces/{namespace}/deployments/{name}"},
		{"cluster-scoped CR", "/apis/rbac.authorization.k8s.io/v1/clusterroles", "clusterroles", "/apis/rbac.authorization.k8s.io/v1/clusterroles"},
		{"subresource", "/api/v1/namespaces/default/pods/my-pod/status", "pods", "/api/v1/namespaces/{namespace}/pods/{name}/status"},
		{"nodes get", "/api/v1/nodes/node-1", "nodes", "/api/v1/nodes/{name}"},
		{"trailing slash", "/api/v1/pods/", "pods", "/api/v1/pods"},
		{"empty", "", "unknown", ""},
		{"non-api", "/healthz", "unknown", "/healthz"},
		{"api no resource", "/api/v1", "unknown", "/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, tmpl := parseKubePath(tt.path)
			assert.Equal(t, tt.wantResource, resource)
			assert.Equal(t, tt.wantTemplate, tmpl)
		})
	}
}

// mockRoundTripper is a test helper that returns a preconfigured response.
type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestRoundTrip_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusOK},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/namespaces/default/pods", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "kubernetes.api.request", span.OperationName())
	assert.Equal(t, "GET /api/v1/namespaces/{namespace}/pods", span.Tag("resource.name"))
	assert.Equal(t, "http", span.Tag("span.type"))
	assert.Equal(t, "GET", span.Tag("http.method"))
	assert.Equal(t, 200, span.Tag("http.status_code"))
	assert.Equal(t, "pods", span.Tag("kube.resource_kind"))
	assert.Nil(t, span.Tag("error"))
}

func TestRoundTrip_4xxNotError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusNotFound},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/namespaces/default/pods/missing-pod", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	assert.Equal(t, 404, spans[0].Tag("http.status_code"))
	assert.Nil(t, spans[0].Tag("error"), "4xx should not be marked as errors")
}

func TestRoundTrip_5xxError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusInternalServerError},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/namespaces/default/pods", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	assert.Equal(t, 500, spans[0].Tag("http.status_code"))
	assert.Equal(t, true, spans[0].Tag("error"))
}

func TestRoundTrip_TransportError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	transportErr := errors.New("connection refused")
	rt := NewCustomRoundTripper(&mockRoundTripper{
		err: transportErr,
	}, 30)

	req, _ := http.NewRequest("POST", "https://kube-apiserver:443/api/v1/namespaces/default/pods", nil)
	_, err := rt.RoundTrip(req)

	require.Error(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	assert.Equal(t, "kubernetes.api.request", spans[0].OperationName())
	spanErr, ok := spans[0].Tag("error").(error)
	require.True(t, ok, "error tag should be an error from WithError")
	assert.Equal(t, "connection refused", spanErr.Error())
}

func TestRoundTrip_NoopTracer(t *testing.T) {
	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusOK},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
