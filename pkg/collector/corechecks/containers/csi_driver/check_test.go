// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const fixtureMetrics = `# HELP datadog_csi_driver_node_publish_volume_attempts Counts the number of publish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6

# HELP datadog_csi_driver_node_unpublish_volume_attempts Counts the number of unpublish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_unpublish_volume_attempts counter
datadog_csi_driver_node_unpublish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6
`

// Real Prometheus client libraries append _total to counter names.
const fixtureMetricsWithTotal = `# HELP datadog_csi_driver_node_publish_volume_attempts_total Counts the number of publish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_publish_volume_attempts_total counter
datadog_csi_driver_node_publish_volume_attempts_total{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6

# HELP datadog_csi_driver_node_unpublish_volume_attempts_total Counts the number of unpublish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_unpublish_volume_attempts_total counter
datadog_csi_driver_node_unpublish_volume_attempts_total{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6
`

func newTestCheck() *Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   metricDefs,
	}
}

func TestConfigureDefault(t *testing.T) {
	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := chk.Configure(senderManager, integration.FakeConfigHash, []byte(`{}`), []byte(``), "test", "provider")
	require.NoError(t, err)
	assert.Equal(t, defaultEndpoint, chk.config.OpenmetricsEndpoint)
}

func TestConfigureCustomEndpoint(t *testing.T) {
	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	instanceCfg := []byte(`openmetrics_endpoint: http://custom:9090/metrics`)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)
	assert.Equal(t, "http://custom:9090/metrics", chk.config.OpenmetricsEndpoint)
}

func TestRunSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixtureMetrics))
	}))
	defer ts.Close()

	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.NoError(t, err)

	expectedTags := []string{
		"status:success",
		"path:/var/run/datadog",
		"type:DSDSocketDirectory",
	}
	matchTags := func(tags []string) bool {
		sorted := slices.Clone(tags)
		slices.Sort(sorted)
		expected := slices.Clone(expectedTags)
		slices.Sort(expected)
		return slices.Equal(sorted, expected)
	}

	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_publish_volume_attempts.count",
		6.0, "", mock.MatchedBy(matchTags))

	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_unpublish_volume_attempts.count",
		6.0, "", mock.MatchedBy(matchTags))

	mockSender.AssertCalled(t, "ServiceCheck",
		"datadog.csi_driver.openmetrics.health",
		mock.Anything, "", mock.Anything, "")

	mockSender.AssertCalled(t, "Commit")
}

func TestRunSuccessWithTotalSuffix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixtureMetricsWithTotal))
	}))
	defer ts.Close()

	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.NoError(t, err)

	expectedTags := []string{
		"status:success",
		"path:/var/run/datadog",
		"type:DSDSocketDirectory",
	}
	matchTags := func(tags []string) bool {
		sorted := slices.Clone(tags)
		slices.Sort(sorted)
		expected := slices.Clone(expectedTags)
		slices.Sort(expected)
		return slices.Equal(sorted, expected)
	}

	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_publish_volume_attempts.count",
		6.0, "", mock.MatchedBy(matchTags))

	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_unpublish_volume_attempts.count",
		6.0, "", mock.MatchedBy(matchTags))

	mockSender.AssertCalled(t, "Commit")
}

func TestRunEndpointDown(t *testing.T) {
	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	instanceCfg := []byte(`openmetrics_endpoint: http://127.0.0.1:1/bad`)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.Error(t, err)

	mockSender.AssertCalled(t, "ServiceCheck",
		"datadog.csi_driver.openmetrics.health",
		mock.Anything, "", mock.Anything, mock.Anything)

	mockSender.AssertCalled(t, "Commit")
}

func TestRunEmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.NoError(t, err)

	mockSender.AssertNotCalled(t, "MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertCalled(t, "Commit")
}
