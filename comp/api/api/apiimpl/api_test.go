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
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	// component dependencies
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"

	// package dependencies

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	// third-party dependencies
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type testdeps struct {
	fx.In

	// additional StartServer arguments
	//
	// TODO: remove these in the next PR once StartServer component arguments
	//       are part of the api component dependency struct
	DogstatsdServer       dogstatsdServer.Component
	Capture               replay.Component
	SecretResolver        secrets.Component
	RcService             optional.Option[rcservice.Component]
	RcServiceMRF          optional.Option[rcservicemrf.Component]
	AuthToken             authtoken.Component
	WorkloadMeta          workloadmeta.Component
	Tagger                tagger.Mock
	Autodiscovery         autodiscovery.Mock
	Logs                  optional.Option[logsAgent.Component]
	Collector             optional.Option[collector.Component]
	DiagnoseSenderManager diagnosesendermanager.Component
	Telemetry             telemetry.Component
	EndpointProviders     []api.EndpointProvider `group:"agent_endpoint"`
}

func getComponentDependencies(t *testing.T) testdeps {
	// TODO: this fxutil.Test[T] can take a component and return the component
	return fxutil.Test[testdeps](
		t,
		hostnameimpl.MockModule(),
		dogstatsdServer.MockModule(),
		replaymock.MockModule(),
		secretsimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[rcservice.Component]()),
		fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
		fetchonlyimpl.MockModule(),
		fx.Supply(context.Background()),
		taggerimpl.MockModule(),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[logsAgent.Component]()),
		fx.Supply(optional.NewNoneOption[collector.Component]()),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
	)
}

func getTestAPIServer(deps testdeps) api.Component {
	apideps := dependencies{
		DogstatsdServer:   deps.DogstatsdServer,
		Capture:           deps.Capture,
		SecretResolver:    deps.SecretResolver,
		RcService:         deps.RcService,
		RcServiceMRF:      deps.RcServiceMRF,
		AuthToken:         deps.AuthToken,
		Tagger:            deps.Tagger,
		LogsAgentComp:     deps.Logs,
		WorkloadMeta:      deps.WorkloadMeta,
		Collector:         deps.Collector,
		Telemetry:         deps.Telemetry,
		EndpointProviders: deps.EndpointProviders,
	}
	return newAPIServer(apideps)
}

func TestStartServer(t *testing.T) {
	deps := getComponentDependencies(t)

	srv := getTestAPIServer(deps)
	err := srv.StartServer()
	defer srv.StopServer()

	assert.NoError(t, err, "could not start api component servers: %v", err)
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
	authToken, err := os.CreateTemp("", "auth_token")
	require.NoError(t, err)
	defer os.Remove(authToken.Name())

	authTokenValue := strings.Repeat("a", 64)
	_, err = io.WriteString(authToken, authTokenValue)
	require.NoError(t, err)

	err = authToken.Close()
	require.NoError(t, err)

	deps := getComponentDependencies(t)

	cfg := config.Mock(t)
	cfg.Set("cmd_port", 0, model.SourceFile)
	cfg.Set("agent_ipc.port", 56789, model.SourceFile)
	cfg.Set("auth_token_file_path", authToken.Name(), model.SourceFile)

	srv := getTestAPIServer(deps)
	err = srv.StartServer()
	require.NoError(t, err)
	defer srv.StopServer()

	telemetryMock := deps.Telemetry.(telemetry.Mock)
	registry := telemetryMock.GetRegistry()

	testCases := []struct {
		addr       string
		serverName string
	}{
		{
			addr:       cmdListener.Addr().String(),
			serverName: cmdServerShortName,
		},
		{
			addr:       ipcListener.Addr().String(),
			serverName: ipcServerShortName,
		},
	}

	expectedMetricName := fmt.Sprintf("%s__%s", observability.MetricSubsystem, observability.MetricName)
	for _, tc := range testCases {
		t.Run(tc.serverName, func(t *testing.T) {
			url := fmt.Sprintf("https://%s/this_does_not_exist", tc.addr)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authTokenValue))
			resp, err := util.GetClient(false).Do(req)
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
