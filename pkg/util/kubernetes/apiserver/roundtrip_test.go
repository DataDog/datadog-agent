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

func TestExtractResource(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"core resource list", "/api/v1/pods", "pods"},
		{"core resource get", "/api/v1/pods/my-pod", "pods"},
		{"namespaced resource list", "/api/v1/namespaces/default/pods", "pods"},
		{"namespaced resource get", "/api/v1/namespaces/default/pods/my-pod", "pods"},
		{"namespaces list", "/api/v1/namespaces", "namespaces"},
		{"namespace get", "/api/v1/namespaces/kube-system", "namespaces"},
		{"services", "/api/v1/namespaces/default/services", "services"},
		{"apps group deployment", "/apis/apps/v1/namespaces/default/deployments", "deployments"},
		{"apps group deployment get", "/apis/apps/v1/namespaces/default/deployments/nginx", "deployments"},
		{"cluster-scoped custom resource", "/apis/rbac.authorization.k8s.io/v1/clusterroles", "clusterroles"},
		{"nodes list", "/api/v1/nodes", "nodes"},
		{"node get", "/api/v1/nodes/node-1", "nodes"},
		{"configmaps", "/api/v1/namespaces/default/configmaps/my-cm", "configmaps"},
		{"subresource status", "/api/v1/namespaces/default/pods/my-pod/status", "pods"},
		{"empty path", "", "unknown"},
		{"root only", "/", "unknown"},
		{"non-api path", "/healthz", "unknown"},
		{"api with no resource", "/api/v1", "unknown"},
		{"trailing slash", "/api/v1/pods/", "pods"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractResource(tt.path))
		})
	}
}

func TestKubeVerb(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		query    string
		expected string
	}{
		{"list pods", http.MethodGet, "/api/v1/pods", "", "list"},
		{"get pod", http.MethodGet, "/api/v1/namespaces/default/pods/my-pod", "", "get"},
		{"watch pods", http.MethodGet, "/api/v1/pods", "watch=true", "watch"},
		{"watch with other params", http.MethodGet, "/api/v1/pods", "resourceVersion=123&watch=true", "watch"},
		{"watch=1", http.MethodGet, "/api/v1/pods", "watch=1", "watch"},
		{"create pod", http.MethodPost, "/api/v1/namespaces/default/pods", "", "create"},
		{"update pod", http.MethodPut, "/api/v1/namespaces/default/pods/my-pod", "", "update"},
		{"patch pod", http.MethodPatch, "/api/v1/namespaces/default/pods/my-pod", "", "patch"},
		{"delete pod", http.MethodDelete, "/api/v1/namespaces/default/pods/my-pod", "", "delete"},
		{"list namespaces", http.MethodGet, "/api/v1/namespaces", "", "list"},
		{"get namespace", http.MethodGet, "/api/v1/namespaces/default", "", "get"},
		{"list deployments", http.MethodGet, "/apis/apps/v1/namespaces/default/deployments", "", "list"},
		{"get deployment", http.MethodGet, "/apis/apps/v1/namespaces/default/deployments/nginx", "", "get"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, kubeVerb(tt.method, tt.path, tt.query))
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
	assert.Equal(t, "GET /api/v1/namespaces/default/pods", span.Tag("resource.name"))
	assert.Equal(t, "http", span.Tag("span.type"))
	assert.Equal(t, "GET", span.Tag("http.method"))
	assert.Equal(t, 200, span.Tag("http.status_code"))
	assert.Equal(t, "pods", span.Tag("kube.resource_kind"))
	assert.Equal(t, "list", span.Tag("kube.verb"))
	assert.Nil(t, span.Tag("error"))
}

func TestRoundTrip_ErrorResponse(t *testing.T) {
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
	span := spans[0]

	assert.Equal(t, 404, span.Tag("http.status_code"))
	assert.Equal(t, true, span.Tag("error"))
	assert.Equal(t, "pods", span.Tag("kube.resource_kind"))
	assert.Equal(t, "get", span.Tag("kube.verb"))
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
	span := spans[0]

	assert.Equal(t, "kubernetes.api.request", span.OperationName())
	assert.Equal(t, "create", span.Tag("kube.verb"))
	// Transport error is captured on span via WithError
	spanErr, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Equal(t, "connection refused", spanErr.Error())
}

func TestRoundTrip_NoopTracer(t *testing.T) {
	// No mocktracer.Start() — tracer returns NoopSpan. Should not panic.
	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusOK},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRoundTrip_WatchRequest(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	rt := NewCustomRoundTripper(&mockRoundTripper{
		response: &http.Response{StatusCode: http.StatusOK},
	}, 30)

	req, _ := http.NewRequest("GET", "https://kube-apiserver:443/api/v1/namespaces/default/pods?watch=true&resourceVersion=100", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "watch", spans[0].Tag("kube.verb"))
	assert.Equal(t, "pods", spans[0].Tag("kube.resource_kind"))
}
