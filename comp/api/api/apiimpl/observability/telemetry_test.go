// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTelemetryMiddleware(t *testing.T) {
	at := authtokenmock.New(t)
	// Parse the certificate from the server TLS config
	cert, err := x509.ParseCertificate(at.GetTLSServerConfig().Certificates[0].Certificate[0])
	require.NoError(t, err)

	testCases := []struct {
		method    string
		path      string
		code      int
		duration  time.Duration
		tlsConfig *tls.Config
		auth      string
	}{
		{
			method:   http.MethodGet,
			path:     "/test/1",
			code:     http.StatusOK,
			duration: 0,
			tlsConfig: &tls.Config{
				InsecureSkipVerify: true,
			}, // The client is not providing a certificate, so it is not mTLS
			auth: "token",
		},
		{
			method:    http.MethodPost,
			path:      "/test/2",
			code:      http.StatusInternalServerError,
			duration:  time.Millisecond,
			tlsConfig: at.GetTLSClientConfig(), // The client is providing same certificate as the server, so it is mTLS
			auth:      "mTLS",
		},
		{
			method:    http.MethodHead,
			path:      "/test/3",
			code:      http.StatusNotFound,
			duration:  time.Second,
			tlsConfig: at.GetTLSClientConfig(),
			auth:      "mTLS",
		},
	}

	serverName := "test"
	for _, tc := range testCases {
		testName := fmt.Sprintf("%s %s %d %s", tc.method, tc.path, tc.code, tc.duration)
		t.Run(testName, func(t *testing.T) {
			clock := clock.NewMock()
			telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
			tmf := newTelemetryMiddlewareFactory(telemetry, clock, cert)
			telemetryHandler := tmf.Middleware(serverName)

			var tcHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
				clock.Add(tc.duration)
				w.WriteHeader(tc.code)
			}

			server := at.NewMockServer(telemetryHandler(tcHandler))

			url := url.URL{
				Scheme: "https",
				Host:   server.Listener.Addr().String(),
				Path:   tc.path,
			}

			req, err := http.NewRequest(tc.method, url.String(), nil)
			require.NoError(t, err)

			client := server.Client()

			client.Transport.(*http.Transport).TLSClientConfig = tc.tlsConfig

			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()

			observabilityMetric, err := telemetry.GetHistogramMetric(MetricSubsystem, MetricName)
			require.NoError(t, err)

			require.Len(t, observabilityMetric, 1)

			metric := observabilityMetric[0]
			assert.EqualValues(t, tc.duration.Seconds(), metric.Value())

			labels := metric.Tags()

			expected := map[string]string{
				"servername":  serverName,
				"status_code": strconv.Itoa(tc.code),
				"method":      tc.method,
				"path":        tc.path,
				"auth":        tc.auth,
			}
			assert.Equal(t, expected, labels)
		})
	}
}

func TestTelemetryMiddlewareDuration(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tmf := NewTelemetryMiddlewareFactory(telemetry, nil)
	telemetryHandler := tmf.Middleware("test")

	var tcHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	server := httptest.NewServer(telemetryHandler(tcHandler))
	defer server.Close()

	start := time.Now()

	resp, err := server.Client().Get(server.URL)
	require.NoError(t, err)
	resp.Body.Close()

	duration := time.Since(start).Milliseconds()
	require.LessOrEqual(t, duration, 100*time.Millisecond)
}

func TestTelemetryMiddlewareTwice(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tmf := NewTelemetryMiddlewareFactory(telemetry, nil)

	// test that we can create multiple middleware instances
	// Prometheus metrics can be registered only once, this test enforces that the metric
	// is not created in the Middleware itself
	_ = tmf.Middleware("test1")
	_ = tmf.Middleware("test2")
}

func TestTelemetryMiddlewareAuthTag(t *testing.T) {
	at := authtokenmock.New(t)

	testCases := []struct {
		name            string
		serverTLSConfig *tls.Config
		clientTLSConfig *tls.Config
		auth            string
	}{
		{
			name:            "secure server & secure client",
			serverTLSConfig: at.GetTLSServerConfig(),
			clientTLSConfig: at.GetTLSClientConfig(),
			auth:            "mTLS",
		},
		{
			name:            "secure server & insecure client",
			serverTLSConfig: at.GetTLSServerConfig(),
			clientTLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			auth: "token",
		},
		{
			name:            "insecure server & secure client",
			serverTLSConfig: nil,
			clientTLSConfig: at.GetTLSClientConfig(),
			auth:            "token",
		},
		{
			name:            "insecure server & insecure client",
			serverTLSConfig: nil,
			clientTLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			auth: "token",
		},
		{
			name:            "secure server & secure client with different certificate",
			serverTLSConfig: at.GetTLSServerConfig(),
			clientTLSConfig: func() *tls.Config {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				cfg := &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{server.TLS.Certificates[0]},
				}
				return cfg
			}(),
			auth: "token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
			// Parse the certificate from the server TLS config if it exists
			var cert *x509.Certificate
			var err error
			if tc.serverTLSConfig != nil {
				cert, err = x509.ParseCertificate(tc.serverTLSConfig.Certificates[0].Certificate[0])
				require.NoError(t, err)
			}
			tmf := NewTelemetryMiddlewareFactory(telemetry, cert)
			telemetryHandler := tmf.Middleware("test")

			var tcHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}

			server := at.NewMockServer(telemetryHandler(tcHandler))

			url := url.URL{
				Scheme: "https",
				Host:   server.Listener.Addr().String(),
				Path:   "/",
			}

			req, err := http.NewRequest(http.MethodGet, url.String(), nil)
			require.NoError(t, err)

			client := server.Client()
			client.Transport.(*http.Transport).TLSClientConfig = tc.clientTLSConfig

			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()

			observabilityMetric, err := telemetry.GetHistogramMetric(MetricSubsystem, MetricName)
			require.NoError(t, err)

			require.Len(t, observabilityMetric, 1)

			metric := observabilityMetric[0]
			labels := metric.Tags()
			assert.Equal(t, tc.auth, labels["auth"])
		})
	}
}
