// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
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
}
