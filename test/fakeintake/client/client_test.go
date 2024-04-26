// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	_ "embed"

	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/fixtures"
)

//go:embed fixtures/api_v2_series_response
var apiV2SeriesResponse []byte

//go:embed fixtures/api_v1_check_run_response
var apiV1CheckRunResponse []byte

//go:embed fixtures/api_v2_logs_response
var apiV2LogsResponse []byte

//go:embed fixtures/api_support_flare_response
var supportFlareResponse []byte

//go:embed fixtures/api_v2_contimage_response
var apiV2ContainerImage []byte

//go:embed fixtures/api_v2_contlcycle_response
var apiV2ContainerLifecycle []byte

//go:embed fixtures/api_v2_sbom_response
var apiV2SBOM []byte

//go:embed fixtures/api_v02_trace_response
var apiV02Trace []byte

//go:embed fixtures/api_v02_apm_stats_response
var apiV02APMStats []byte

//go:embed fixtures/api_v1_metadata_response
var apiV1Metadata []byte

//go:embed fixtures/api_v2_ndmflow_response
var apiV2NDMFlow []byte

func TestClient(t *testing.T) {
	t.Run("getFakePayloads should properly format the request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow requests only to "/foo/bar"
			routes := r.URL.Query()["endpoint"]

			payloads := []api.Payload{
				{
					Data: []byte(r.URL.Path),
				},
				{
					Data: []byte(fmt.Sprintf("%d", len(routes))),
				},
				{
					Data: []byte(routes[0]),
				},
			}
			// create fake response
			resp, err := json.Marshal(api.APIFakeIntakePayloadsRawGETResponse{
				Payloads: payloads,
			})
			require.NoError(t, err)
			w.Write(resp)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		require.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 3, len(payloads))
		assert.Equal(t, "/fakeintake/payloads", string(payloads[0].Data))
		assert.Equal(t, "1", string(payloads[1].Data))
		assert.Equal(t, "/foo/bar", string(payloads[2].Data))
	})

	t.Run("getFakePayloads should handle response with errors", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.Error(t, err, "Expecting error")
		assert.Nil(t, payloads)
	})

	t.Run("getMetrics", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getMetrics()
		require.NoError(t, err)
		assert.True(t, client.metricAggregator.ContainsPayloadName("system.load.1"))
		assert.False(t, client.metricAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.metricAggregator.ContainsPayloadNameAndTags("snmp.ifAdminStatus", []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.False(t, client.metricAggregator.ContainsPayloadNameAndTags("snmp.ifAdminStatus", []string{"totoro"}))
	})

	t.Run("getMetric", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		metrics, err := client.getMetric("snmp.ifAdminStatus")
		require.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(metrics, []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.Empty(t, aggregator.FilterByTags(metrics, []string{"totoro"}))
	})

	t.Run("FilterMetrics", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		metrics, err := client.FilterMetrics("snmp.sysUpTimeInstance",
			WithTags[*aggregator.MetricSeries]([]string{"snmp_device:172.25.0.3", "snmp_profile:generic-router"}),
			WithMetricValueHigherThan(4226040),
			WithMetricValueLowerThan(4226042),
			WithMetricValueInRange(4226000, 4226050))
		require.NoError(t, err)
		assert.NotEmpty(t, metrics)
	})

	t.Run("getCheckRun", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV1CheckRunResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getCheckRuns()
		require.NoError(t, err)
		assert.True(t, client.checkRunAggregator.ContainsPayloadName("snmp.can_check"))
		assert.False(t, client.checkRunAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.checkRunAggregator.ContainsPayloadNameAndTags("datadog.agent.check_status", []string{"check:snmp"}))
		assert.False(t, client.checkRunAggregator.ContainsPayloadNameAndTags("datadog.agent.check_status", []string{"totoro"}))
	})

	t.Run("GetCheckRun", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV1CheckRunResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		checks, err := client.GetCheckRun("datadog.agent.check_status")
		require.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(checks, []string{"check:snmp"}))
		assert.Empty(t, aggregator.FilterByTags(checks, []string{"totoro"}))
	})

	t.Run("getLogs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getLogs()
		require.NoError(t, err)
		assert.True(t, client.logAggregator.ContainsPayloadName("testapp"))
		assert.False(t, client.logAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("getLog", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		logs, err := client.getLog("testapp")
		require.NoError(t, err)
		assert.Equal(t, 2, len(logs))
		assert.Equal(t, "hello there, can you hear me", logs[0].Message)
		assert.Equal(t, "info", logs[0].Status)
		assert.Equal(t, "a new line of logs", logs[1].Message)
	})

	t.Run("FilterLogs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		logs, err := client.FilterLogs("testapp", WithMessageMatching(`^hello.*`), WithMessageContaining("hello there, can you hear"))
		require.NoError(t, err)
		assert.Equal(t, 1, len(logs))
	})

	t.Run("GetServerHealth", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/fakeintake/health" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.GetServerHealth()
		require.NoError(t, err)
	})

	t.Run("FlushPayloads", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/fakeintake/flushPayloads" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.FlushServerAndResetAggregators()
		require.NoError(t, err)
	})

	t.Run("ConfigureOverride", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/fakeintake/configure/override" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.ConfigureOverride(api.ResponseOverride{
			StatusCode:  http.StatusOK,
			ContentType: "text/plain",
			Body:        []byte("totoro"),
		})
		require.NoError(t, err)
	})

	t.Run("GetLatestFlare", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(supportFlareResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		flare, err := client.GetLatestFlare()
		require.NoError(t, err)
		assert.Equal(t, flare.GetEmail(), "test")
		assert.Equal(t, flare.GetAgentVersion(), "7.45.1+commit.102cdaf")
		assert.Equal(t, flare.GetHostname(), "test-hostname")
	})

	t.Run("getProcesses", func(t *testing.T) {
		payload := fixtures.CollectorProcPayload(t)
		response := fmt.Sprintf(
			`{
				"payloads": [
					{
						"timestamp": "2023-07-12T11:05:20.847091908Z",
						"data": "%s",
						"encoding": "protobuf"
					}
				]
			}`, base64.StdEncoding.EncodeToString(payload))

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(response))
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getProcesses()
		require.NoError(t, err)
		assert.True(t, client.processAggregator.ContainsPayloadName("i-078e212"))
		assert.False(t, client.processAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("getContainers", func(t *testing.T) {
		payload := fixtures.CollectorContainerPayload(t)
		response := fmt.Sprintf(
			`{
				"payloads": [
					{
						"timestamp": "2023-07-12T11:05:20.847091908Z",
						"data": "%s",
						"encoding": "protobuf"
					}
				]
			}`, base64.StdEncoding.EncodeToString(payload))

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(response))
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getContainers()
		require.NoError(t, err)
		assert.True(t, client.containerAggregator.ContainsPayloadName("i-078e212"))
		assert.False(t, client.containerAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("getProcessDiscoveries", func(t *testing.T) {
		payload := fixtures.CollectorProcDiscoveryPayload(t)
		response := fmt.Sprintf(
			`{
				"payloads": [
					{
						"timestamp": "2023-07-12T11:05:20.847091908Z",
						"data": "%s",
						"encoding": "protobuf"
					}
				]
			}`, base64.StdEncoding.EncodeToString(payload))

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(response))
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getProcessDiscoveries()
		require.NoError(t, err)
		assert.True(t, client.processDiscoveryAggregator.ContainsPayloadName("i-078e212"))
		assert.False(t, client.processDiscoveryAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("getContainerImages", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2ContainerImage)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getContainerImages()
		require.NoError(t, err)
		assert.True(t, client.containerImageAggregator.ContainsPayloadName("gcr.io/datadoghq/agent"))
		assert.False(t, client.containerImageAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.containerImageAggregator.ContainsPayloadNameAndTags("gcr.io/datadoghq/agent", []string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		assert.False(t, client.containerImageAggregator.ContainsPayloadNameAndTags("gcr.io/datadoghq/agent", []string{"totoro"}))
	})

	t.Run("getContainerImage", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2ContainerImage)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		images, err := client.getContainerImage("gcr.io/datadoghq/agent")
		require.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(images, []string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		assert.Empty(t, aggregator.FilterByTags(images, []string{"totoro"}))
	})

	t.Run("FilterContainerImages", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2ContainerImage)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		images, err := client.FilterContainerImages("gcr.io/datadoghq/agent",
			WithTags[*aggregator.ContainerImagePayload]([]string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		require.NoError(t, err)
		assert.NotEmpty(t, images)
	})

	t.Run("getContainerLifecycleEvents", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2ContainerLifecycle)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getContainerLifecycleEvents()
		require.NoError(t, err)
		assert.True(t, client.containerLifecycleAggregator.ContainsPayloadName("container_id://67c96c4c78279a06731198908090eb48bc0b3341d569951e1c4df6d2173cb967"))
		assert.True(t, client.containerLifecycleAggregator.ContainsPayloadName("kubernetes_pod_uid://a530f36b-a60d-4d24-ada1-fcdd0d148976"))
		assert.False(t, client.containerLifecycleAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("GetContainerLifecycleEvents", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2ContainerLifecycle)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		events, err := client.GetContainerLifecycleEvents()
		require.NoError(t, err)
		assert.NotEmpty(t, events)
	})

	t.Run("getSBOMs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SBOM)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getSBOMs()
		require.NoError(t, err)
		assert.True(t, client.sbomAggregator.ContainsPayloadName("gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d"))
		assert.False(t, client.sbomAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.sbomAggregator.ContainsPayloadNameAndTags("gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d", []string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		assert.False(t, client.sbomAggregator.ContainsPayloadNameAndTags("gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d", []string{"totoro"}))
	})

	t.Run("getSBOM", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SBOM)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		sboms, err := client.getSBOM("gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d")
		require.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(sboms, []string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		assert.Empty(t, aggregator.FilterByTags(sboms, []string{"totoro"}))
	})

	t.Run("FilterSBOMs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SBOM)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		sboms, err := client.FilterSBOMs("gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d",
			WithTags[*aggregator.SBOMPayload]([]string{"git.repository_url:https://github.com/DataDog/datadog-agent"}))
		require.NoError(t, err)
		assert.NotEmpty(t, sboms)
	})

	t.Run("getTraces", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV02Trace)
		}))
		defer ts.Close()
		client := NewClient(ts.URL)
		err := client.getTraces()
		require.NoError(t, err)
		assert.True(t, client.traceAggregator.ContainsPayloadName("dev.host"))
		assert.False(t, client.traceAggregator.ContainsPayloadName("not.found"))
	})

	t.Run("getAPMStats", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV02APMStats)
		}))
		defer ts.Close()
		client := NewClient(ts.URL)
		err := client.getAPMStats()
		require.NoError(t, err)
		assert.True(t, client.apmStatsAggregator.ContainsPayloadName("dev.host"))
		assert.False(t, client.apmStatsAggregator.ContainsPayloadName("not.found"))
	})

	t.Run("GetMetadata", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV1Metadata)
		}))
		defer ts.Close()
		client := NewClient(ts.URL)
		payloads, err := client.GetMetadata()
		require.NoError(t, err)
		const expectedHostname = "i-0473fb6c2bd4591b4"
		assert.NotEmpty(t, payloads)
		assert.Len(t, payloads, 3)
		for _, p := range payloads {
			assert.Equal(t, expectedHostname, p.Hostname)
		}
	})

	t.Run("getNDMFlows", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2NDMFlow)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getNDMFlows()
		require.NoError(t, err)
		assert.True(t, client.ndmflowAggregator.ContainsPayloadName("i-028cd2a4530c36887"))
	})

	t.Run("GetNDMFlows", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2NDMFlow)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		ndmflows, err := client.GetNDMFlows()
		require.NoError(t, err)
		assert.Equal(t, len(ndmflows), 992)
		const expectedHostname = "i-028cd2a4530c36887"
		for _, n := range ndmflows {
			assert.Equal(t, expectedHostname, n.Host)
		}

		t.Logf("%+v", ndmflows[0])
		assert.Equal(t, int64(1710375648197), ndmflows[0].FlushTimestamp)
		assert.Equal(t, "netflow5", ndmflows[0].FlowType)
		assert.Equal(t, uint64(0), ndmflows[0].SamplingRate)
		assert.Equal(t, "ingress", ndmflows[0].Direction)
		assert.Equal(t, uint64(1710375646), ndmflows[0].Start)
		assert.Equal(t, uint64(1710375648), ndmflows[0].End)
		assert.Equal(t, uint64(2070), ndmflows[0].Bytes)
		assert.Equal(t, uint64(1884), ndmflows[0].Packets)
		assert.Equal(t, "IPv4", ndmflows[0].EtherType)
		assert.Equal(t, "TCP", ndmflows[0].IPProtocol)
		assert.Equal(t, "default", ndmflows[0].Device.Namespace)
		assert.Equal(t, "172.18.0.3", ndmflows[0].Exporter.IP)
		assert.Equal(t, "192.168.20.10", ndmflows[0].Source.IP)
		assert.Equal(t, "40", ndmflows[0].Source.Port)
		assert.Equal(t, "00:00:00:00:00:00", ndmflows[0].Source.Mac)
		assert.Equal(t, "192.0.0.0/5", ndmflows[0].Source.Mask)
		assert.Equal(t, "202.12.190.10", ndmflows[0].Destination.IP)
		assert.Equal(t, "443", ndmflows[0].Destination.Port)
		assert.Equal(t, "00:00:00:00:00:00", ndmflows[0].Destination.Mac)
		assert.Equal(t, "202.12.188.0/22", ndmflows[0].Destination.Mask)
		assert.Equal(t, uint32(0), ndmflows[0].Ingress.Interface.Index)
		assert.Equal(t, uint32(0), ndmflows[0].Egress.Interface.Index)
		assert.Equal(t, "i-028cd2a4530c36887", ndmflows[0].Host)
		assert.Empty(t, ndmflows[0].TCPFlags)
		assert.Equal(t, "172.199.15.1", ndmflows[0].NextHop.IP)
		assert.Empty(t, ndmflows[0].AdditionalFields)
	})
}
