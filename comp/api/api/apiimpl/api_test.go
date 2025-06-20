// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"golang.org/x/net/http2"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
	grpcNonefx "github.com/DataDog/datadog-agent/comp/api/grpcserver/fx-none"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"

	// package dependencies

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	// third-party dependencies
	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

// The following certificate and key are used for testing purposes only.
// They have been generated using the following command:
//
//	openssl req -x509 -newkey ec:<(openssl ecparam -name prime256v1) -keyout key.pem -out cert.pem -days 3650 \
//	  -subj "/O=Datadog, Inc." \
//	  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
//	  -addext "keyUsage=digitalSignature,keyEncipherment" \
//	  -addext "extendedKeyUsage=serverAuth,clientAuth" \
//	  -addext "basicConstraints=CA:TRUE" \
//	  -nodes
var (
	unknownIPCCert = []byte(`-----BEGIN CERTIFICATE-----
MIIByzCCAXKgAwIBAgIUS1FJz1+ha1R1nNhi8E8nZhr5X6YwCgYIKoZIzj0EAwIw
GDEWMBQGA1UECgwNRGF0YWRvZywgSW5jLjAeFw0yNTA2MTYxMzE1MjZaFw0zNTA2
MTQxMzE1MjZaMBgxFjAUBgNVBAoMDURhdGFkb2csIEluYy4wWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAAS7B0LbAe5NsNzPt8swHTTCkXEGL9g1sDivlYOZffXo1wCJ
K1xQo0EcgnYUkiAoVqJXQoA9FFP+KAKEy1HFEcRTo4GZMIGWMB0GA1UdDgQWBBTx
k3F9kxVd6tg8pWPTxl1qxzL9djAfBgNVHSMEGDAWgBTxk3F9kxVd6tg8pWPTxl1q
xzL9djAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCwYDVR0PBAQDAgWgMB0G
A1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMEBTADAQH/MAoGCCqG
SM49BAMCA0cAMEQCIH2ZuPES7+uwxjIF72poM16EJE8F2nG3qKDDPWtOTrUHAiBF
R+jl3j1r8H6k8BatF4eUWagFev35hLz7VMuHVLR5Mw==
-----END CERTIFICATE-----
`)
	unknownIPCKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg0MgVZJY0NrncRALD
GpnnYROePY82rJHHNeVtcG/VsyGhRANCAAS7B0LbAe5NsNzPt8swHTTCkXEGL9g1
sDivlYOZffXo1wCJK1xQo0EcgnYUkiAoVqJXQoA9FFP+KAKEy1HFEcRT
-----END PRIVATE KEY-----
`)
)

type testdeps struct {
	fx.In

	API       api.Component
	Telemetry telemetry.Mock
	IPC       ipc.Component
}

func getAPIServer(t *testing.T, params config.MockParams, fxOptions ...fx.Option) testdeps {
	return fxutil.Test[testdeps](
		t,
		Module(),
		fx.Replace(params),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
		telemetryimpl.MockModule(),
		config.MockModule(),
		grpcNonefx.Module(),
		fx.Options(fxOptions...),
	)
}

func TestStartServer(t *testing.T) {
	cfgOverride := config.MockParams{Overrides: map[string]interface{}{
		"cmd_port": 0,
		// doesn't test agent_ipc because it would try to register an already registered expvar in TestStartBothServersWithObservability
		"agent_ipc.port": 0,
	}}

	getAPIServer(t, cfgOverride)
}

func hasLabelValue(labels []*dto.LabelPair, name string, value string) bool {
	for _, label := range labels {
		if label.GetName() == name && label.GetValue() == value {
			return true
		}
	}
	return false
}

func TestStartBothServersWithObservability(t *testing.T) {
	cfgOverride := config.MockParams{Overrides: map[string]interface{}{
		"cmd_port":       0,
		"agent_ipc.port": 56789,
	}}

	deps := getAPIServer(t, cfgOverride)

	registry := deps.Telemetry.GetRegistry()

	testCases := []struct {
		addr       string
		serverName string
	}{
		{
			addr:       deps.API.CMDServerAddress().String(),
			serverName: cmdServerShortName,
		},
		{
			addr:       deps.API.IPCServerAddress().String(),
			serverName: ipcServerShortName,
		},
	}

	expectedMetricName := fmt.Sprintf("%s__%s", observability.MetricSubsystem, observability.MetricName)
	for _, tc := range testCases {
		t.Run(tc.serverName, func(t *testing.T) {
			url := fmt.Sprintf("https://%s/this_does_not_exist", tc.addr)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			_, err = deps.IPC.GetClient().Do(req)
			require.ErrorContains(t, err, "status code: 404")

			metricFamilies, err := registry.Gather()
			require.NoError(t, err)

			idx := slices.IndexFunc(metricFamilies, func(metric *dto.MetricFamily) bool {
				return metric.GetName() == expectedMetricName
			})
			require.NotEqual(t, -1, idx, "API telemetry metric not found")

			metricFamily := metricFamilies[idx]
			require.Equal(t, dto.MetricType_HISTOGRAM, metricFamily.GetType())

			metrics := metricFamily.GetMetric()
			metricIdx := slices.IndexFunc(metrics, func(metric *dto.Metric) bool {
				return hasLabelValue(metric.GetLabel(), "servername", tc.serverName)
			})
			require.NotEqualf(t, -1, metricIdx, "could not find metric for servername:%s in %v", tc.serverName, metrics)

			metric := metrics[metricIdx]
			assert.EqualValues(t, 1, metric.GetHistogram().GetSampleCount())

			t.Log(metric.GetLabel())
			assert.True(t, hasLabelValue(metric.GetLabel(), "status_code", strconv.Itoa(http.StatusNotFound)))
			assert.True(t, hasLabelValue(metric.GetLabel(), "method", http.MethodGet))
			assert.True(t, hasLabelValue(metric.GetLabel(), "path", "/this_does_not_exist"))
		})
	}

	t.Run("IPC Server Only Accepts mTLS", func(t *testing.T) {
		addr := deps.API.IPCServerAddress().String()
		url := fmt.Sprintf("https://%s/config/v1/", addr)

		// With the IPC HTTP Client this should succeed
		_, err := deps.IPC.GetClient().Get(url)
		require.NoError(t, err)

		// With a client configured with another certificate, it should fail
		tr := &http.Transport{
			TLSClientConfig: deps.IPC.GetTLSClientConfig(),
		}
		unknownCert, err := tls.X509KeyPair(unknownIPCCert, unknownIPCKey)
		require.NoError(t, err)
		tr.TLSClientConfig.Certificates = []tls.Certificate{unknownCert}
		httpClient := &http.Client{
			Transport: tr,
		}

		_, err = httpClient.Get(url) //nolint:bodyclose
		assert.ErrorContains(t, err, "remote error: tls: unknown certificate authority")
	})
}

type s struct {
	body string
}

func (s *s) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(s.body))
}

type grpcServer struct {
	grpcServer bool
}

func (grpc *grpcServer) BuildServer() http.Handler {
	if grpc.grpcServer {
		return &s{
			body: "GRPC SERVER OK",
		}
	}
	return nil
}

func TestStartServerWithGrpcServer(t *testing.T) {
	cfgOverride := config.MockParams{Overrides: map[string]interface{}{
		"cmd_port": 0,
		// doesn't test agent_ipc because it would try to register an already registered expvar in TestStartBothServersWithObservability
		"agent_ipc.port": 0,
	}}

	deps := getAPIServer(t, cfgOverride, fx.Options(
		fx.Replace(
			fx.Annotate(&grpcServer{
				grpcServer: true,
			}, fx.As(new(grpc.Component))),
		)))

	addr := deps.API.CMDServerAddress().String()

	url := fmt.Sprintf("https://%s", addr)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")

	transport := &http.Transport{
		TLSClientConfig: deps.IPC.GetTLSClientConfig(),
	}

	http2.ConfigureTransport(transport)
	http2Client := &http.Client{
		Transport: transport,
	}

	resp, err := http2Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	t.Log(string(content))

	// test the api routes grpc request to the grpc server
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GRPC SERVER OK", string(content))
}

func TestStartServerWithoutGrpcServer(t *testing.T) {
	cfgOverride := config.MockParams{Overrides: map[string]interface{}{
		"cmd_port": 0,
		// doesn't test agent_ipc because it would try to register an already registered expvar in TestStartBothServersWithObservability
		"agent_ipc.port": 0,
	}}

	deps := getAPIServer(t, cfgOverride, fx.Options(
		fx.Replace(
			fx.Annotate(&grpcServer{
				grpcServer: false,
			}, fx.As(new(grpc.Component))),
		)))

	addr := deps.API.CMDServerAddress().String()

	url := fmt.Sprintf("https://%s", addr)

	// test the api routes does not routes grpc request to the grpc server
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")

	transport := &http.Transport{
		TLSClientConfig: deps.IPC.GetTLSClientConfig(),
	}

	http2.ConfigureTransport(transport)
	http2Client := &http.Client{
		Transport: transport,
	}

	resp, err := http2Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// The server does not have a grpc server, so it should return a 404
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
