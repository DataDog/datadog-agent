// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CustomRoundTripper serves as a wrapper around the default http.RoundTripper, allowing us greater flexibility regarding
// the logging of request errors.
type CustomRoundTripper struct {
	rt      http.RoundTripper
	timeout int64
}

// NewCustomRoundTripper creates a new CustomRoundTripper with the apiserver timeout value already populated from the
// agent config, wrapping an existing http.RoundTripper.
func NewCustomRoundTripper(rt http.RoundTripper, timeout time.Duration) *CustomRoundTripper {
	return &CustomRoundTripper{
		rt:      rt,
		timeout: int64(timeout.Seconds()),
	}
}

// RoundTrip implements http.RoundTripper. It adds logging on request timeouts with more context
// and creates APM spans for Kubernetes API requests.
func (rt *CustomRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	start := time.Now()

	resource, tmpl := parseKubePath(request.URL.Path)
	span, ctx := tracer.StartSpanFromContext(request.Context(), "kubernetes.api.request",
		tracer.ResourceName(request.Method+" "+tmpl),
		tracer.SpanType("http"),
		tracer.Tag("http.method", request.Method),
		tracer.Tag("kube.resource_kind", resource),
	)
	request = request.WithContext(ctx)

	response, err := rt.rt.RoundTrip(request)
	if err, ok := err.(net.Error); ok && err.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		clientTimeouts.Inc()
		log.Warnf("timeout trying to make the request in %v (kubernetes_apiserver_client_timeout: %v)", time.Since(start), rt.timeout)
	}

	if response != nil {
		span.SetTag("http.status_code", response.StatusCode)
		// Only mark 5xx as span errors. 4xx responses (404, 409 conflicts, etc.)
		// are expected during normal K8s API usage (informer resyncs, update retries).
		if response.StatusCode >= 500 {
			span.SetTag("error", true)
		}
	}
	span.Finish(tracer.WithError(err))

	return response, err
}

// WrappedRoundTripper implements http.RoundTripperWrapper.
func (rt *CustomRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }

// parseKubePath parses a Kubernetes API URL path and returns the resource kind
// (e.g. "pods") and a templatized path with dynamic segments replaced by placeholders
// (e.g. "/api/v1/namespaces/{namespace}/pods/{name}").
//
// Kubernetes URL patterns:
//
//	/api/v1/pods                              → ("pods",         "/api/v1/pods")
//	/api/v1/namespaces/default/pods/my-pod    → ("pods",         "/api/v1/namespaces/{namespace}/pods/{name}")
//	/apis/apps/v1/namespaces/ns/deployments   → ("deployments",  "/apis/apps/v1/namespaces/{namespace}/deployments")
//	/api/v1/namespaces                        → ("namespaces",   "/api/v1/namespaces")
func parseKubePath(path string) (resource string, templatized string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Determine where the resource portion starts (skip api prefix + version).
	var i int
	if len(parts) == 0 {
		return "unknown", path
	}
	switch parts[0] {
	case "api":
		i = 2 // "api", version
	case "apis":
		i = 3 // "apis", group, version
	default:
		return "unknown", path
	}
	if i >= len(parts) {
		return "unknown", path
	}

	// Pattern after prefix: [namespaces/{ns}/] resource [/{name} [/{subresource}]]
	// "namespaces" is a namespace prefix only when followed by at least two more segments.
	if parts[i] == "namespaces" && i+2 < len(parts) {
		parts[i+1] = "{namespace}"
		i += 2
	}
	if i >= len(parts) {
		return "unknown", "/" + strings.Join(parts, "/")
	}

	resource = parts[i]
	if i+1 < len(parts) {
		parts[i+1] = "{name}"
	}

	return resource, "/" + strings.Join(parts, "/")
}
