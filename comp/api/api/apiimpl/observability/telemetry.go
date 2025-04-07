// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	// MetricSubsystem is the subsystem for the metric
	MetricSubsystem = "api_server"
	// MetricName is the name of the metric
	MetricName = "request_duration_seconds"
	metricHelp = "Request duration distribution by server, method, path, and status (in seconds)."
)

type telemetryMiddlewareFactory struct {
	requestDuration telemetry.Histogram
	clock           clock.Clock
	ipcCert         []byte
}

// TelemetryMiddlewareFactory creates a telemetry middleware tagged with a given server name
type TelemetryMiddlewareFactory interface {
	Middleware(serverName string) mux.MiddlewareFunc
}

func (th *telemetryMiddlewareFactory) Middleware(serverName string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var statusCode int
			// next is an argument of the MiddlewareFunc, it is defined outside the HandlerFunc so it is shared between calls,
			// and so it must not be updated otherwise every call of the HandlerFunc will add a new layer of middlewares
			// (and the HandlerFunc is called multiple times)
			next := extractStatusCodeHandler(&statusCode)(next)

			var duration time.Duration
			next = timeHandler(th.clock, &duration)(next)

			next.ServeHTTP(w, r)

			path := extractPath(r)

			durationSeconds := duration.Seconds()

			// We can assert that the auth is at least a token because it have been checked previously by the validateToken middleware
			auth := "token"
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				cert := r.TLS.PeerCertificates[0]

				if bytes.Equal(cert.Raw, th.ipcCert) {
					auth = "mTLS"
				}
			}

			th.requestDuration.Observe(durationSeconds, serverName, strconv.Itoa(statusCode), r.Method, path, auth)
		})
	}
}

func newTelemetryMiddlewareFactory(telemetry telemetry.Component, clock clock.Clock, serverTLSConfig *tls.Config) (TelemetryMiddlewareFactory, error) {
	tags := []string{"servername", "status_code", "method", "path", "auth"}
	var buckets []float64 // use default buckets
	requestDuration := telemetry.NewHistogram(MetricSubsystem, MetricName, tags, metricHelp, buckets)

	// Read the IPC certificate from the server TLS config
	var ipcCert []byte
	if serverTLSConfig == nil || len(serverTLSConfig.Certificates) == 0 {
		return nil, fmt.Errorf("no certificates found in server TLS config")
	}

	if serverTLSConfig.Certificates[0].Leaf != nil {
		ipcCert = serverTLSConfig.Certificates[0].Leaf.Raw
	} else {
		cert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing IPC certificate: %v", err)
		}
		ipcCert = cert.Raw
	}

	return &telemetryMiddlewareFactory{
		requestDuration,
		clock,
		ipcCert,
	}, nil
}

// NewTelemetryMiddlewareFactory creates a new TelemetryMiddlewareFactory
//
// This function must be called only once for a given telemetry Component,
// as it creates a new metric, and Prometheus panics if the same metric is registered twice.
func NewTelemetryMiddlewareFactory(telemetry telemetry.Component, serverTLSConfig *tls.Config) (TelemetryMiddlewareFactory, error) {
	return newTelemetryMiddlewareFactory(telemetry, clock.New(), serverTLSConfig)
}
