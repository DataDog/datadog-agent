// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	agenttracing "github.com/DataDog/datadog-agent/pkg/util/tracing"
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

// RoundTrip implements http.RoundTripper. It adds APM tracing and logging on request errors.
func (rt *CustomRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	span, ctx := agenttracing.StartSpanFromContext(request.Context(), "kubernetes.api.request",
		tracer.SpanType(ext.SpanTypeHTTP),
		tracer.Tag(agenttracing.TagComponent, agenttracing.ComponentKubernetesAPI),
		tracer.Tag(ext.HTTPMethod, request.Method),
		tracer.Tag(ext.HTTPURL, request.URL.Path),
	)
	request = request.WithContext(ctx)

	start := time.Now()

	response, err := rt.rt.RoundTrip(request)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() || errors.Is(err, context.DeadlineExceeded) {
			clientTimeouts.Inc()
			log.Warnf("timeout trying to make the request in %v (kubernetes_apiserver_client_timeout: %v)", time.Since(start), rt.timeout)
			span.SetTag(agenttracing.TagErrorType, "timeout")
		} else {
			span.SetTag(agenttracing.TagErrorType, fmt.Sprintf("%T", err))
		}
		span.Finish(tracer.WithError(err))
		return response, err
	}

	span.SetTag(ext.HTTPCode, strconv.Itoa(response.StatusCode))
	if response.StatusCode >= 400 {
		span.Finish(tracer.WithError(fmt.Errorf("HTTP %d from kubernetes API", response.StatusCode)))
	} else {
		span.Finish()
	}

	return response, err
}

// WrappedRoundTripper implements http.RoundTripperWrapper.
func (rt *CustomRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }
