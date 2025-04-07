// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"golang.org/x/net/http2"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"
	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
	grpcNonefx "github.com/DataDog/datadog-agent/comp/api/grpcserver/fx-none"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"

	// package dependencies

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	// third-party dependencies
	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type testdeps struct {
	fx.In

	API       api.Component
	Telemetry telemetry.Mock
	AuthToken authtoken.Component
}

func getAPIServer(t *testing.T, params config.MockParams, fxOptions ...fx.Option) testdeps {
	return fxutil.Test[testdeps](
		t,
		Module(),
		fx.Replace(params),
		hostnameimpl.MockModule(),
		secretsimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		fx.Provide(func(t testing.TB) authtoken.Component { return authtokenmock.New(t) }),
		fx.Supply(context.Background()),
		taggerfxmock.MockModule(),
		fx.Provide(func(mock taggermock.Mock) tagger.Component {
			return mock
		}),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Provide(func(mock autodiscovery.Mock) autodiscovery.Component {
			return mock
		}),
		fx.Supply(option.None[collector.Component]()),
		pidmapimpl.Module(),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
		telemetryimpl.MockModule(),
		config.MockModule(),
		grpcNonefx.Module(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Options(fxOptions...),
	)
}

func testAPIServer(params config.MockParams, fxOptions ...fx.Option) (*fx.App, testdeps, error) {
	return fxutil.TestApp[testdeps](
		Module(),
		fx.Replace(params),
		hostnameimpl.MockModule(),
		secretsimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		fx.Provide(func(t *testing.T) authtoken.Component { return authtokenmock.New(t) }),
		fx.Supply(context.Background()),
		taggerfxmock.MockModule(),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Provide(func(mock autodiscovery.Mock) autodiscovery.Component {
			return mock
		}),
		fx.Supply(option.None[collector.Component]()),
		pidmapimpl.Module(),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
		telemetryimpl.MockModule(),
		config.MockModule(),
		grpcNonefx.Module(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
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

			resp, err := util.GetClient().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// for debug purpose
			if content, err := io.ReadAll(resp.Body); assert.NoError(t, err) {
				t.Log(string(content))
			}

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)

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
	gateway    bool
}

func (grpc *grpcServer) BuildServer() http.Handler {
	if grpc.grpcServer {
		return &s{
			body: "GRPC SERVER OK",
		}
	}
	return nil
}

func (grpc *grpcServer) BuildGatewayMux(string) (http.Handler, error) {
	if grpc.gateway {
		return &s{
			body: "GRPC GATEWAY OK",
		}, nil
	}

	return nil, fmt.Errorf("error")
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
				gateway:    true,
			}, fx.As(new(grpc.Component))),
		)))

	addr := deps.API.CMDServerAddress().String()

	url := fmt.Sprintf("https://%s", addr)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := util.GetClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	t.Log(string(content))

	// test the gateway is monted at the root
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GRPC GATEWAY OK", string(content))

	req, err = http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")

	transport := &http.Transport{
		TLSClientConfig: deps.AuthToken.GetTLSClientConfig(),
	}

	http2.ConfigureTransport(transport)
	http2Client := &http.Client{
		Transport: transport,
	}

	resp, err = http2Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	content, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	t.Log(string(content))

	// test the api routes grpc request to the grpc server
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GRPC SERVER OK", string(content))
}

func TestStartServerWithGrpcServerFailGateway(t *testing.T) {
	cfgOverride := config.MockParams{Overrides: map[string]interface{}{
		"cmd_port": 0,
		// doesn't test agent_ipc because it would try to register an already registered expvar in TestStartBothServersWithObservability
		"agent_ipc.port": 0,
	}}

	_, _, errApp := testAPIServer(cfgOverride, fx.Options(
		fx.Replace(
			fx.Annotate(&grpcServer{
				grpcServer: true,
				gateway:    false,
			}, fx.As(new(grpc.Component))),
		)))

	assert.Error(t, errApp)
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
				gateway:    true,
			}, fx.As(new(grpc.Component))),
		)))

	addr := deps.API.CMDServerAddress().String()

	url := fmt.Sprintf("https://%s", addr)

	// test the api routes does not routes grpc request to the grpc server
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")

	transport := &http.Transport{
		TLSClientConfig: deps.AuthToken.GetTLSClientConfig(),
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

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GRPC GATEWAY OK", string(content))
}
