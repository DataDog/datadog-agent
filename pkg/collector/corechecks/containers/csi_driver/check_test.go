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
	"strings"
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

	err := chk.Configure(senderManager, integration.FakeConfigHash, []byte(`{}`), []byte(``), "test", "provider")
	require.NoError(t, err)
	assert.Equal(t, defaultEndpoint, chk.config.OpenmetricsEndpoint)
}

func TestConfigureCustomEndpoint(t *testing.T) {
	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.NoError(t, err)

	// The high-cardinality `path` label is dropped from client-facing metrics.
	expectedTags := []string{
		"status:success",
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

	instanceCfg := []byte(`openmetrics_endpoint: ` + ts.URL)
	err := chk.Configure(senderManager, integration.FakeConfigHash, instanceCfg, []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	err = chk.Run()
	require.NoError(t, err)

	// The high-cardinality `path` label is dropped from client-facing metrics.
	expectedTags := []string{
		"status:success",
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

// TestClientPublishAggregatesAcrossPathsDroppingPath verifies that client-facing
// (piste A) publish counts drop the high-cardinality `path` label and are summed
// per low-cardinality context (type, status). This must hold for every volume
// type, not just libraries: ephemeral per-pod paths (e.g. DatadogInjectorPreload)
// would otherwise be under-counted, and the socket types predate the library
// feature. The summed value is submitted to MonotonicCount, which is correct
// because the driver never removes its Prometheus series (the per-context sum
// stays monotonic).
func TestClientPublishAggregatesAcrossPathsDroppingPath(t *testing.T) {
	fixture := `# TYPE datadog_csi_driver_node_publish_volume_attempts counter
datadog_csi_driver_node_publish_volume_attempts{path="/var/lib/kubelet/pods/aaaa/volumes/kubernetes.io~csi/x/mount",status="success",type="DatadogInjectorPreload"} 1
datadog_csi_driver_node_publish_volume_attempts{path="/var/lib/kubelet/pods/bbbb/volumes/kubernetes.io~csi/x/mount",status="success",type="DatadogInjectorPreload"} 1
datadog_csi_driver_node_publish_volume_attempts{path="registry.datadoghq.com/dd-lib-java-init@sha256:aaaa",status="success",type="DatadogLibrary"} 2
datadog_csi_driver_node_publish_volume_attempts{path="registry.datadoghq.com/dd-lib-js-init@sha256:bbbb",status="success",type="DatadogLibrary"} 3
datadog_csi_driver_node_publish_volume_attempts{path="/var/run/datadog",status="success",type="DSDSocketDirectory"} 6
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer ts.Close()

	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run())

	tagsEqual := func(want ...string) func([]string) bool {
		return func(tags []string) bool {
			for _, tag := range tags {
				if tag == "path" || len(tag) >= 5 && tag[:5] == "path:" {
					return false
				}
			}
			sorted := slices.Clone(tags)
			slices.Sort(sorted)
			expected := slices.Clone(want)
			slices.Sort(expected)
			return slices.Equal(sorted, expected)
		}
	}

	// Ephemeral per-pod paths are summed (1+1), not lost.
	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_publish_volume_attempts.count",
		2.0, "", mock.MatchedBy(tagsEqual("status:success", "type:DatadogInjectorPreload")))

	// Distinct library image paths are summed (2+3) under one context.
	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_publish_volume_attempts.count",
		5.0, "", mock.MatchedBy(tagsEqual("status:success", "type:DatadogLibrary")))

	// Pre-existing socket type is unaffected.
	mockSender.AssertCalled(t, "MonotonicCount",
		"datadog.csi_driver.node_publish_volume_attempts.count",
		6.0, "", mock.MatchedBy(tagsEqual("status:success", "type:DSDSocketDirectory")))

	mockSender.AssertCalled(t, "Commit")
}

func TestRunEndpointDown(t *testing.T) {
	chk := newTestCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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

// TestCOATHistogramReportsCountAndSum verifies that the download-duration
// histogram is onboarded to COAT as two delta counters (_count and _sum) built
// from the histogram's cumulative count/sum series, while the per-bucket series
// are ignored. Together they let the backend derive volume and average latency.
func TestCOATHistogramReportsCountAndSum(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(scrapeCount.Add(1))
		// Cumulative count grows by 2 and sum by 5.0 each scrape.
		body := fmt.Sprintf(`# TYPE datadog_csi_driver_library_download_duration_seconds histogram
datadog_csi_driver_library_download_duration_seconds_bucket{library="dd-lib-java-init",registry="gcr.io",le="1"} %d
datadog_csi_driver_library_download_duration_seconds_bucket{library="dd-lib-java-init",registry="gcr.io",le="+Inf"} %d
datadog_csi_driver_library_download_duration_seconds_sum{library="dd-lib-java-init",registry="gcr.io"} %d
datadog_csi_driver_library_download_duration_seconds_count{library="dd-lib-java-init",registry="gcr.io"} %d
`, n, 2*n, 5*n, 2*n)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run()) // count=2, sum=5
	require.NoError(t, chk.Run()) // count=4, sum=10

	countMetrics, err := tm.GetCountMetric(CheckName, "library_download_duration_seconds_count")
	require.NoError(t, err)
	require.Len(t, countMetrics, 1)
	assert.Equal(t, 4.0, countMetrics[0].Value())
	assert.Equal(t, map[string]string{"library": "dd-lib-java-init", "registry": "gcr.io"}, countMetrics[0].Tags())

	sumMetrics, err := tm.GetCountMetric(CheckName, "library_download_duration_seconds_sum")
	require.NoError(t, err)
	require.Len(t, sumMetrics, 1)
	assert.Equal(t, 10.0, sumMetrics[0].Value())

	// The per-bucket series must not be onboarded to COAT.
	_, err = tm.GetGaugeMetric(CheckName, "library_download_duration_seconds_bucket")
	require.ErrorContains(t, err, "not found")
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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

// TestCOATCountersAggregateAcrossPathsBoundedCache verifies that the COAT
// publish counter sums all ephemeral per-path source series into a single
// series per preserved-tag context (status,type), and — critically — that the
// delta cache (prevValues) is keyed by that bounded context rather than by the
// unbounded path label. Without this, each new pod path would add a permanent
// prevValues entry, leaking memory for the lifetime of the Agent process.
func TestCOATCountersAggregateAcrossPathsBoundedCache(t *testing.T) {
	tm := telemetryimpl.NewMock(t)

	var scrapeCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(scrapeCount.Add(1))
		// Simulate churn: the number of ephemeral per-pod paths grows every
		// scrape (the driver never removes its series), but they all collapse
		// to the same (status=success, type=DatadogInjectorPreload) context.
		var b strings.Builder
		b.WriteString("# TYPE datadog_csi_driver_node_publish_volume_attempts counter\n")
		nPaths := 3 * n // 3 paths on run 1, 6 on run 2, ...
		for i := 0; i < nPaths; i++ {
			fmt.Fprintf(&b, "datadog_csi_driver_node_publish_volume_attempts{path=\"/var/lib/kubelet/pods/uid-%d/mount\",status=\"success\",type=\"DatadogInjectorPreload\"} 1\n", i)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(b.String()))
	}))
	defer ts.Close()

	chk := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		metrics:   buildMetricDefs(tm),
		state:     newTestState(),
	}
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, chk.Configure(senderManager, integration.FakeConfigHash, []byte(`openmetrics_endpoint: `+ts.URL), []byte(``), "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, chk.Run()) // 3 paths x 1 = 3
	require.NoError(t, chk.Run()) // 6 paths x 1 = 6 (delta 3)

	publishMetrics, err := tm.GetCountMetric(CheckName, "node_publish_volume_attempts")
	require.NoError(t, err)
	require.Len(t, publishMetrics, 1, "all per-path series must collapse into a single COAT context series")
	assert.Equal(t, 6.0, publishMetrics[0].Value(),
		"COAT counter should equal the latest summed cumulative value across all paths")

	// The delta cache must stay bounded by the number of contexts (1 here),
	// not grow with the number of ephemeral paths (9 seen across both scrapes).
	chk.state.mu.Lock()
	prevLen := len(chk.state.prevValues)
	chk.state.mu.Unlock()
	assert.Equal(t, 1, prevLen,
		"prevValues must be keyed by the bounded preserved-tag context, not per ephemeral path")
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
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
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

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
