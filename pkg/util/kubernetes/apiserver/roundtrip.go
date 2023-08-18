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
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
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
func NewCustomRoundTripper(rt http.RoundTripper) *CustomRoundTripper {
	return &CustomRoundTripper{
		rt:      rt,
		timeout: config.Datadog.GetInt64("kubernetes_apiserver_client_timeout"),
	}
}

// RoundTrip implements http.RoundTripper. It adds logging on request timeouts with more context.
func (rt *CustomRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	start := time.Now()

	response, err := rt.rt.RoundTrip(request)
	if err, ok := err.(net.Error); ok && err.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		clientTimeouts.Inc()
		log.Warnf("timeout trying to make the request in %v (kubernetes_apiserver_client_timeout: %v)", time.Now().Sub(start), rt.timeout)
	}

	return response, err
}

// WrappedRoundTripper implements http.RoundTripperWrapper.
func (rt *CustomRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }
