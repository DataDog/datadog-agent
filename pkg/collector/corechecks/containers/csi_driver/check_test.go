// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const fixtureMetrics = `# HELP datadog_csi_driver_node_publish_volume_attempts Counts the number of publish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6

# HELP datadog_csi_driver_node_unpublish_volume_attempts Counts the number of unpublish volume requests received by the csi node server
# TYPE datadog_csi_driver_node_unpublish_volume_attempts counter
datadog_csi_driver_node_unpublish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6

# HELP datadog_csi_driver_library_resolutions_total Counts the outcome of attempts to resolve a library for a volume
# TYPE datadog_csi_driver_library_resolutions_total counter
datadog_csi_driver_library_resolutions_total{library="dd-lib-java-init",result="resolved"} 4

# HELP datadog_csi_driver_library_cleanup_total Counts cleanup attempts for unused libraries
# TYPE datadog_csi_driver_library_cleanup_total counter
datadog_csi_driver_library_cleanup_total{library="dd-lib-java-init",status="success",strategy="immediate"} 2

# HELP datadog_csi_driver_libraries_cached Number of library versions currently stored on disk, per package
# TYPE datadog_csi_driver_libraries_cached gauge
datadog_csi_driver_libraries_cached{library="dd-lib-java-init"} 3

# HELP datadog_csi_driver_libraries_cached_bytes Cumulative on-disk size of cached libraries, in bytes, per package
# TYPE datadog_csi_driver_libraries_cached_bytes gauge
datadog_csi_driver_libraries_cached_bytes{library="dd-lib-java-init"} 12345

# HELP datadog_csi_driver_library_volume_links Number of volumes currently linked to a library
# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 7
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
	tm := nooptelemetry.GetCompatComponent()
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
}

func newTestState() *sharedState {
	return &sharedState{
		prevValues: make(map[string]float64),
		gaugeKeys:  make(map[string]gaugeSeries),
	}
}

func TestFactorySharesCOATCollectorsAcrossInstances(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	factoryOption := Factory(tm)
	factory, ok := factoryOption.Get()
	require.True(t, ok)

	first, ok := factory().(*Check)
	require.True(t, ok)
	second, ok := factory().(*Check)
	require.True(t, ok)

	assert.Same(t, first.metrics["datadog_csi_driver_node_publish_volume_attempts"].coat.counter, second.metrics["datadog_csi_driver_node_publish_volume_attempts"].coat.counter)
	assert.Same(t, first.state, second.state)
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

	libraryTags := []string{"library:dd-lib-java-init", "result:resolved"}
	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.library_resolutions.count",
		4.0, "", mock.MatchedBy(func(tags []string) bool {
			sorted := slices.Clone(tags)
			slices.Sort(sorted)
			expected := slices.Clone(libraryTags)
			slices.Sort(expected)
			return slices.Equal(sorted, expected)
		}))

	mockSender.AssertCalled(t, "Gauge",
		"datadog.csi_driver.libraries_cached",
		3.0, "", mock.MatchedBy(func(tags []string) bool {
			return slices.Equal(tags, []string{"library:dd-lib-java-init"})
		}))

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

// TestCOATCountersDeltaOnly verifies that COAT telemetry counters receive only
// the delta between consecutive scrapes, not the full cumulative Prometheus
// counter value. Prometheus counters are monotonically increasing; if we blindly
// Add(sample.Value) on every run, the COAT counter inflates proportionally to
// the number of check runs rather than actual CSI operations.
func TestCOATCountersDeltaOnly(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		// Simulate a Prometheus counter that increases by 3 each scrape interval.
		publishValue := 3 * int(n)
		unpublishValue := 1 * int(n)
		body := fmt.Sprintf(`# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} %d
# TYPE datadog_csi_driver_node_unpublish_volume_attempts counter
datadog_csi_driver_node_unpublish_volume_attempts{status="success"} %d
`, publishValue, unpublishValue)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	// Run 1: endpoint returns publish=3, unpublish=1
	require.NoError(t, chk.Run())
	// Run 2: endpoint returns publish=6, unpublish=2
	require.NoError(t, chk.Run())
	// Run 3: endpoint returns publish=9, unpublish=3
	require.NoError(t, chk.Run())

	// The COAT counters should reflect the LATEST cumulative value (9 and 3),
	// not the sum of all scraped values (3+6+9=18 and 1+2+3=6).
	publishMetrics, err := tm.GetCountMetric(CheckName, "node_publish_volume_attempts")
	require.NoError(t, err)
	require.Len(t, publishMetrics, 1)
	assert.Equal(t, 9.0, publishMetrics[0].Value(),
		"COAT publish counter should equal the latest cumulative value, not the sum of all scrapes")

	unpublishMetrics, err := tm.GetCountMetric(CheckName, "node_unpublish_volume_attempts")
	require.NoError(t, err)
	require.Len(t, unpublishMetrics, 1)
	assert.Equal(t, 3.0, unpublishMetrics[0].Value(),
		"COAT unpublish counter should equal the latest cumulative value, not the sum of all scrapes")
}

func TestCOATCounterDeltasSurviveCheckReload(t *testing.T) {
	tm := telemetryimpl.NewMock(t)
	factoryOption := Factory(tm)
	factory, ok := factoryOption.Get()
	require.True(t, ok)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		body := fmt.Sprintf(`# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} %d
`, 3*int(n))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)

	first := factory().(*Check)
	require.NoError(t, first.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))
	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()
	require.NoError(t, first.Run())

	second := factory().(*Check)
	require.NoError(t, second.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))
	require.NoError(t, second.Run())

	publishMetrics, err := tm.GetCountMetric(CheckName, "node_publish_volume_attempts")
	require.NoError(t, err)
	require.Len(t, publishMetrics, 1)
	assert.Equal(t, 6.0, publishMetrics[0].Value(),
		"COAT counter should add only the delta after a check reload")
}

func TestCOATGaugesSetLatestValue(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		body := fmt.Sprintf(`# TYPE datadog_csi_driver_libraries_cached gauge
datadog_csi_driver_libraries_cached{library="dd-lib-java-init"} %d
# TYPE datadog_csi_driver_libraries_cached_bytes gauge
datadog_csi_driver_libraries_cached_bytes{library="dd-lib-java-init"} %d
# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} %d
`, 2*int(n), 100*int(n), 3*int(n))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())
	require.NoError(t, chk.Run())

	cachedMetrics, err := tm.GetGaugeMetric(CheckName, "libraries_cached")
	require.NoError(t, err)
	require.Len(t, cachedMetrics, 1)
	assert.Equal(t, 4.0, cachedMetrics[0].Value())

	cachedBytesMetrics, err := tm.GetGaugeMetric(CheckName, "libraries_cached_bytes")
	require.NoError(t, err)
	require.Len(t, cachedBytesMetrics, 1)
	assert.Equal(t, 200.0, cachedBytesMetrics[0].Value())

	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)
	assert.Equal(t, 6.0, linksMetrics[0].Value())
}

func TestCOATGaugesDeleteMissingSeries(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
`
		if n == 1 {
			body += `datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 7
`
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())
	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)
	assert.Equal(t, 7.0, linksMetrics[0].Value())

	require.NoError(t, chk.Run())
	linksMetrics, err = tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.ErrorContains(t, err, "not found")
	require.Empty(t, linksMetrics)
}

// TestCOATGaugesLatestScrapeWinsForSameSeries guards the intentional
// single-endpoint design (see sharedState): gauges are not aggregated across
// endpoints, so for a shared COAT series the latest scrape wins rather than summing.
func TestCOATGaugesLatestScrapeWinsForSameSeries(t *testing.T) {
	tm := telemetryimpl.NewMock(t)
	factoryOption := Factory(tm)
	factory, ok := factoryOption.Get()
	require.True(t, ok)

	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 7
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer firstServer.Close()

	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 3
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer secondServer.Close()

	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	first := factory().(*Check)
	require.NoError(t, first.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+firstServer.URL), []byte(``), "test", "provider"))
	second := factory().(*Check)
	require.NoError(t, second.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+secondServer.URL), []byte(``), "test", "provider"))

	require.NoError(t, first.Run())
	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)
	assert.Equal(t, 7.0, linksMetrics[0].Value())

	// The second endpoint reports the same series; the value is overwritten
	// (last scrape wins) rather than aggregated to 10.
	require.NoError(t, second.Run())
	linksMetrics, err = tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)
	assert.Equal(t, 3.0, linksMetrics[0].Value())
}

func TestCOATGaugesDeleteOnScrapeFailure(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		if n > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 7
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())
	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)

	require.Error(t, chk.Run())
	linksMetrics, err = tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.ErrorContains(t, err, "not found")
	require.Empty(t, linksMetrics)
}

func TestCOATGaugesDeleteOnCancel(t *testing.T) {
	tm := telemetryimpl.NewMock(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 7
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())
	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)

	chk.Cancel()
	linksMetrics, err = tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.ErrorContains(t, err, "not found")
	require.Empty(t, linksMetrics)
}

// TestCOATCountersHandleReset verifies that when the CSI driver restarts and its
// cumulative Prometheus counter drops (delta < 0), the COAT counter treats the
// new value as a fresh increment instead of subtracting, so it never rewinds.
func TestCOATCountersHandleReset(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		// Run 1 reports 10; run 2 reports 3 (driver restarted, counter reset).
		value := 10
		if n >= 2 {
			value = 3
		}
		body := fmt.Sprintf(`# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{status="success",type="DSDSocketDirectory"} %d
`, value)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run()) // delta 10 -> counter 10
	require.NoError(t, chk.Run()) // reset: delta becomes 3 (not -7) -> counter 13

	publishMetrics, err := tm.GetCountMetric(CheckName, "node_publish_volume_attempts")
	require.NoError(t, err)
	require.Len(t, publishMetrics, 1)
	assert.Equal(t, 13.0, publishMetrics[0].Value(),
		"after a counter reset the post-reset value should be added, not subtracted")
}

// TestCOATGaugesDeleteMissingSeriesSelectively verifies that when only one of
// several gauge series disappears, that series is deleted while the others keep
// their values (the stale-deletion is per-series, not all-or-nothing).
func TestCOATGaugesDeleteMissingSeriesSelectively(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := scrapeCount.Add(1)
		body := `# TYPE datadog_csi_driver_library_volume_links gauge
datadog_csi_driver_library_volume_links{library="dd-lib-java-init"} 5
`
		if n == 1 {
			body += `datadog_csi_driver_library_volume_links{library="dd-lib-python-init"} 4
`
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())
	linksMetrics, err := tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 2)

	// Second scrape drops the python series but keeps java.
	require.NoError(t, chk.Run())
	linksMetrics, err = tm.GetGaugeMetric(CheckName, "library_volume_links")
	require.NoError(t, err)
	require.Len(t, linksMetrics, 1)
	assert.Equal(t, "dd-lib-java-init", linksMetrics[0].Tags()["library"])
	assert.Equal(t, 5.0, linksMetrics[0].Value())
}

// TestCOATMetricsPreserveExpectedTags verifies that the COAT metrics carry
// exactly the preserved tag set declared in buildMetricDefs (and drop the extra
// Prometheus labels), keeping the check in sync with the COAT profile allowlist.
func TestCOATMetricsPreserveExpectedTags(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := `# TYPE datadog_csi_driver_library_cleanup_total counter
datadog_csi_driver_library_cleanup_total{library="dd-lib-java-init",status="success",strategy="delayed"} 2
# TYPE datadog_csi_driver_libraries_cached gauge
datadog_csi_driver_libraries_cached{library="dd-lib-java-init"} 3
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())

	cleanupMetrics, err := tm.GetCountMetric(CheckName, "library_cleanup")
	require.NoError(t, err)
	require.Len(t, cleanupMetrics, 1)
	assert.Equal(t, map[string]string{
		"library":  "dd-lib-java-init",
		"status":   "success",
		"strategy": "delayed",
	}, cleanupMetrics[0].Tags())

	cachedMetrics, err := tm.GetGaugeMetric(CheckName, "libraries_cached")
	require.NoError(t, err)
	require.Len(t, cachedMetrics, 1)
	assert.Equal(t, map[string]string{"library": "dd-lib-java-init"}, cachedMetrics[0].Tags())
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
