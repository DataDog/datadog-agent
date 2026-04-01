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

	path := request.URL.Path
	span, ctx := tracer.StartSpanFromContext(request.Context(), "kubernetes.api.request",
		tracer.ResourceName(request.Method+" "+path),
		tracer.SpanType("http"),
		tracer.Tag("http.method", request.Method),
		tracer.Tag("kube.resource_kind", extractResource(path)),
		tracer.Tag("kube.verb", kubeVerb(request.Method, path, request.URL.RawQuery)),
	)
	request = request.WithContext(ctx)

	response, err := rt.rt.RoundTrip(request)
	if err, ok := err.(net.Error); ok && err.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		clientTimeouts.Inc()
		log.Warnf("timeout trying to make the request in %v (kubernetes_apiserver_client_timeout: %v)", time.Since(start), rt.timeout)
	}

	if response != nil {
		span.SetTag("http.status_code", response.StatusCode)
		if response.StatusCode >= 400 {
			span.SetTag("error", true)
		}
	}
	span.Finish(tracer.WithError(err))

	return response, err
}

// WrappedRoundTripper implements http.RoundTripperWrapper.
func (rt *CustomRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }

// extractResource parses a Kubernetes API URL path and returns the resource kind
// (e.g. "pods", "services", "namespaces"). Returns "unknown" if the path cannot be parsed.
//
// Kubernetes URL patterns:
//   /api/v1/pods                           → "pods"
//   /api/v1/namespaces/{ns}/pods           → "pods"
//   /api/v1/namespaces/{ns}/pods/{name}    → "pods"
//   /apis/apps/v1/namespaces/{ns}/deployments → "deployments"
//   /api/v1/namespaces                     → "namespaces"
func extractResource(path string) string {
	// Trim leading/trailing slashes and split
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Skip the api prefix: "api/v1/..." or "apis/{group}/v1/..."
	var i int
	if len(parts) == 0 {
		return "unknown"
	}
	if parts[0] == "api" {
		// /api/v1/...
		i = 2 // skip "api", "v1"
	} else if parts[0] == "apis" {
		// /apis/{group}/v1/...
		i = 3 // skip "apis", group, version
	} else {
		return "unknown"
	}

	if i >= len(parts) {
		return "unknown"
	}

	// After the prefix, pattern is: [namespaces/{ns}/] resource [/{name}] [/{subresource}]
	// The resource is always at an even offset from current position
	// if parts[i] == "namespaces", skip "namespaces/{ns}"
	if parts[i] == "namespaces" && i+2 < len(parts) {
		i += 2 // skip "namespaces", namespace name
	}

	if i >= len(parts) {
		return "unknown"
	}

	return parts[i]
}

// kubeVerb maps an HTTP method, URL path, and query string to a Kubernetes API verb.
// Watch requests (GET with ?watch=true) are detected via the query string.
func kubeVerb(method, path, rawQuery string) string {
	switch method {
	case http.MethodGet:
		// Check for watch requests in query string
		if strings.Contains(rawQuery, "watch=true") || strings.Contains(rawQuery, "watch=1") {
			return "watch"
		}
		// Distinguish list vs get: if the path ends with a resource collection (no name), it's list
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			resource := extractResource(path)
			if resource != "unknown" && last != resource {
				return "get"
			}
		}
		return "list"
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		// Could be delete or deletecollection, but deletecollection is rare
		return "delete"
	default:
		return strings.ToLower(method)
	}
}
