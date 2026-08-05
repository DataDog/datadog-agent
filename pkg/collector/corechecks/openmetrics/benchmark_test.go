// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
	"github.com/stretchr/testify/require"
)

func BenchmarkOpenMetricsUpstreamFixtures(b *testing.B) {
	for _, benchmark := range benchmarkOpenMetricsFixtureCases(b) {
		b.Run(benchmark.name, func(b *testing.B) {
			benchmarkOpenMetricsRun(b, benchmark.instance, benchmark.payload)
		})
	}
}

func BenchmarkOpenMetricsStressPayload(b *testing.B) {
	payload := benchmarkStressPayload(50_000, 0)
	widePayload := benchmarkStressPayload(50_000, 50_000)

	b.Run("default_cap", func(b *testing.B) {
		benchmarkOpenMetricsRun(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, payload)
	})

	b.Run("default_cap_openmetrics_text", func(b *testing.B) {
		benchmarkOpenMetricsRunWithContentType(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, payload+"# EOF\n", "application/openmetrics-text; version=1.0.0")
	})

	b.Run("default_cap_openmetrics_text_regex_fallback", func(b *testing.B) {
		benchmarkOpenMetricsRunWithSetupAndContentType(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, payload+"# EOF\n", "application/openmetrics-text; version=1.0.0", func(scraper *openmetricsScraper) {
			scraper.transformer.patterns = append(scraper.transformer.patterns, metricPattern{pattern: regexp.MustCompile("a^")})
		})
	})

	b.Run("default_cap_buffered", func(b *testing.B) {
		benchmarkOpenMetricsRunWithSetup(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, payload, func(scraper *openmetricsScraper) {
			scraper.transformer.patterns = append(scraper.transformer.patterns, metricPattern{pattern: regexp.MustCompile("a^")})
		})
	})

	b.Run("uncapped", func(b *testing.B) {
		benchmarkOpenMetricsRun(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
max_returned_metrics: -1
`, payload)
	})

	b.Run("wide_endpoint_subset", func(b *testing.B) {
		benchmarkOpenMetricsRun(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, widePayload)
	})

	b.Run("wide_endpoint_subset_openmetrics_text", func(b *testing.B) {
		benchmarkOpenMetricsRunWithContentType(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, widePayload+"# EOF\n", "application/openmetrics-text; version=1.0.0")
	})

	b.Run("wide_endpoint_subset_openmetrics_text_regex_fallback", func(b *testing.B) {
		benchmarkOpenMetricsRunWithSetupAndContentType(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, widePayload+"# EOF\n", "application/openmetrics-text; version=1.0.0", func(scraper *openmetricsScraper) {
			scraper.transformer.patterns = append(scraper.transformer.patterns, metricPattern{pattern: regexp.MustCompile("a^")})
		})
	})

	b.Run("wide_endpoint_subset_buffered", func(b *testing.B) {
		benchmarkOpenMetricsRunWithSetup(b, `
openmetrics_endpoint: %%endpoint%%
namespace: om_eval
metrics:
  - stress_gauge:
      type: gauge
  - stress_counter:
      type: counter
  - stress_histogram_seconds:
      type: histogram
`, widePayload, func(scraper *openmetricsScraper) {
			scraper.transformer.patterns = append(scraper.transformer.patterns, metricPattern{pattern: regexp.MustCompile("a^")})
		})
	})
}

func TestOpenMetricsUpstreamBenchmarkFixtureSmoke(t *testing.T) {
	for _, fixture := range benchmarkOpenMetricsFixtureCases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			mockSender := runOpenMetricsFixtureOnce(t, fixture.instance, fixture.payload)
			require.True(t, mockSenderHasMetricCall(mockSender), "expected %s to emit at least one metric", fixture.name)
		})
	}
}

type benchmarkOpenMetricsFixture struct {
	name     string
	instance string
	payload  string
}

func benchmarkOpenMetricsFixtureCases(tb testing.TB) []benchmarkOpenMetricsFixture {
	tb.Helper()

	fixtures := benchmarkOpenMetricsFixtures(tb)
	amazonMSK := fixtures.amazonMSK

	return []benchmarkOpenMetricsFixture{
		{
			name: "legacy_ksm_wildcard",
			instance: `
prometheus_url: %%endpoint%%
namespace: bar
metrics:
  - "*"
`,
			payload: fixtures.ksm,
		},
		{
			name:     "legacy_amazon_msk_jmx",
			instance: benchmarkLegacyAmazonMSKInstance(amazonMSK),
			payload:  fixtures.amazonMSKJMX,
		},
		{
			name: "legacy_ksm_label_joins",
			instance: `
prometheus_url: %%endpoint%%
namespace: bar
label_to_hostname: node
metrics:
  - "*"
label_joins:
  kube_pod_info:
    labels_to_match:
      - pod
      - namespace
    labels_to_get:
      - node
  "1":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "2":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "3":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "4":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "5":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "6":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "7":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "8":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
  "9":
    labels_to_match: [pod, namespace]
    labels_to_get: [node]
`,
			payload: fixtures.ksm,
		},
		{
			name: "v2_ksm_wildcard",
			instance: `
openmetrics_endpoint: %%endpoint%%
namespace: bar
metrics:
  - ".+"
`,
			payload: fixtures.ksm,
		},
		{
			name:     "v2_amazon_msk_jmx",
			instance: benchmarkV2AmazonMSKInstance(amazonMSK),
			payload:  fixtures.amazonMSKJMX,
		},
		{
			name: "v2_ksm_label_joins",
			instance: `
openmetrics_endpoint: %%endpoint%%
namespace: bar
hostname_label: node
metrics:
  - ".+"
share_labels:
  kube_pod_info:
    match:
      - pod
      - namespace
    labels:
      - node
    values:
      - 1
  "1":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "2":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "3":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "4":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "5":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "6":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "7":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "8":
    match: [pod, namespace]
    labels: [node]
    values: [1]
  "9":
    match: [pod, namespace]
    labels: [node]
    values: [1]
`,
			payload: fixtures.ksm,
		},
	}
}

type benchmarkFixtures struct {
	ksm          string
	amazonMSKJMX string
	amazonMSK    benchmarkAmazonMSKData
}

func benchmarkOpenMetricsFixtures(tb testing.TB) benchmarkFixtures {
	tb.Helper()

	if root, ok := benchmarkIntegrationsCoreRoot(tb); ok {
		return benchmarkFixtures{
			ksm:          benchmarkReadFixture(tb, root, "datadog_checks_base/tests/fixtures/prometheus/ksm.txt"),
			amazonMSKJMX: benchmarkReadFixture(tb, root, "datadog_checks_base/tests/fixtures/prometheus/amazon_msk_jmx_metrics.txt"),
			amazonMSK:    benchmarkAmazonMSKConfig(tb, filepath.Join(root, "datadog_checks_base/tests/base/checks/openmetrics/bench_utils.py")),
		}
	}

	return benchmarkFixtures{
		ksm:          benchmarkReadCompressedFixture(tb, "ksm.txt.gz"),
		amazonMSKJMX: benchmarkReadCompressedFixture(tb, "amazon_msk_jmx_metrics.txt.gz"),
		amazonMSK:    benchmarkReadCompressedAmazonMSKConfig(tb),
	}
}

func benchmarkIntegrationsCoreRoot(tb testing.TB) (string, bool) {
	tb.Helper()

	if root := os.Getenv("OPENMETRICS_BENCH_INTEGRATIONS_CORE"); root != "" {
		if _, err := os.Stat(root); err != nil {
			tb.Fatalf("OPENMETRICS_BENCH_INTEGRATIONS_CORE=%s is not readable: %v", root, err)
		}
		return root, true
	}

	dir, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			candidate := filepath.Join(filepath.Dir(dir), "integrations-core")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func benchmarkReadFixture(tb testing.TB, root string, relPath string) string {
	tb.Helper()

	payload, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		tb.Fatalf("read %s: %v", relPath, err)
	}
	return string(payload)
}

func benchmarkReadCompressedFixture(tb testing.TB, name string) string {
	tb.Helper()

	payload, err := benchmarkReadCompressedTestdata(name)
	if err != nil {
		tb.Fatalf("read compressed OpenMetrics benchmark fixture %s: %v", name, err)
	}
	return string(payload)
}

func benchmarkReadCompressedAmazonMSKConfig(tb testing.TB) benchmarkAmazonMSKData {
	tb.Helper()

	payload, err := benchmarkReadCompressedTestdata("amazon_msk_jmx_config.json.gz")
	if err != nil {
		tb.Fatalf("read compressed Amazon MSK benchmark config: %v", err)
	}

	var data benchmarkAmazonMSKData
	if err := json.Unmarshal(payload, &data); err != nil {
		tb.Fatalf("parse compressed Amazon MSK benchmark config: %v", err)
	}
	return data
}

func benchmarkReadCompressedTestdata(name string) ([]byte, error) {
	file, err := os.Open(filepath.Join("testdata", "upstream_benchmarks", name))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func benchmarkStressPayload(series int, ignoredSeries int) string {
	var builder strings.Builder
	builder.WriteString("# HELP stress_target_uptime_seconds Synthetic target uptime.\n")
	builder.WriteString("# TYPE stress_target_uptime_seconds gauge\n")
	builder.WriteString("stress_target_uptime_seconds 1\n")
	builder.WriteString("# HELP stress_gauge Synthetic gauge.\n")
	builder.WriteString("# TYPE stress_gauge gauge\n")
	builder.WriteString("# HELP stress_counter_total Synthetic counter.\n")
	builder.WriteString("# TYPE stress_counter_total counter\n")
	builder.WriteString("# HELP stress_histogram_seconds Synthetic histogram.\n")
	builder.WriteString("# TYPE stress_histogram_seconds histogram\n")
	if ignoredSeries > 0 {
		builder.WriteString("# HELP noise_gauge Synthetic unselected gauge.\n")
		builder.WriteString("# TYPE noise_gauge gauge\n")
		builder.WriteString("# HELP noise_counter_total Synthetic unselected counter.\n")
		builder.WriteString("# TYPE noise_counter_total counter\n")
	}
	for i := 0; i < series; i++ {
		shard := i % 64
		fmt.Fprintf(&builder, "stress_gauge{series=\"%d\",shard=\"%d\"} %d\n", i, shard, i%1000)
		fmt.Fprintf(&builder, "stress_counter_total{series=\"%d\",shard=\"%d\"} %d\n", i, shard, i)
		if i%10 == 0 {
			fmt.Fprintf(&builder, "stress_histogram_seconds_bucket{series=\"%d\",le=\"0.1\"} 1\n", i)
			fmt.Fprintf(&builder, "stress_histogram_seconds_bucket{series=\"%d\",le=\"1\"} 2\n", i)
			fmt.Fprintf(&builder, "stress_histogram_seconds_bucket{series=\"%d\",le=\"+Inf\"} 3\n", i)
			fmt.Fprintf(&builder, "stress_histogram_seconds_sum{series=\"%d\"} 0.7\n", i)
			fmt.Fprintf(&builder, "stress_histogram_seconds_count{series=\"%d\"} 3\n", i)
		}
	}
	for i := 0; i < ignoredSeries; i++ {
		shard := i % 128
		fmt.Fprintf(&builder, "noise_gauge{series=\"%d\",shard=\"%d\",region=\"local\",mode=\"ignored\"} %d\n", i, shard, i%1000)
		fmt.Fprintf(&builder, "noise_counter_total{series=\"%d\",shard=\"%d\",region=\"local\",mode=\"ignored\"} %d\n", i, shard, i)
	}
	return builder.String()
}

type benchmarkAmazonMSKData struct {
	Metrics   map[string]string `json:"metrics"`
	Overrides map[string]string `json:"overrides"`
}

func benchmarkAmazonMSKConfig(tb testing.TB, benchUtils string) benchmarkAmazonMSKData {
	tb.Helper()

	script := `import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("bench_utils", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(json.dumps({
    "metrics": module.AMAZON_MSK_JMX_METRICS_MAP,
    "overrides": module.AMAZON_MSK_JMX_METRICS_OVERRIDES,
}, sort_keys=True))
`
	output, err := exec.Command("python3", "-c", script, benchUtils).Output()
	if err != nil {
		tb.Skipf("python3 could not load %s: %v", benchUtils, err)
	}

	var data benchmarkAmazonMSKData
	if err := json.Unmarshal(output, &data); err != nil {
		tb.Fatalf("parse Amazon MSK benchmark config: %v", err)
	}
	return data
}

func benchmarkLegacyAmazonMSKInstance(data benchmarkAmazonMSKData) string {
	var builder strings.Builder
	builder.WriteString("prometheus_url: %%endpoint%%\nnamespace: bar\nmetrics:\n")
	for _, rawName := range sortedMapKeys(data.Metrics) {
		fmt.Fprintf(&builder, "  - %s: %s\n", rawName, data.Metrics[rawName])
	}
	builder.WriteString("type_overrides:\n")
	for _, rawName := range sortedMapKeys(data.Overrides) {
		fmt.Fprintf(&builder, "  %s: %s\n", rawName, data.Overrides[rawName])
	}
	return builder.String()
}

func benchmarkV2AmazonMSKInstance(data benchmarkAmazonMSKData) string {
	var builder strings.Builder
	builder.WriteString("openmetrics_endpoint: %%endpoint%%\nnamespace: bar\nmetrics:\n")
	for _, rawName := range sortedMapKeys(data.Metrics) {
		fmt.Fprintf(&builder, "  - %s:\n      name: %s\n", rawName, data.Metrics[rawName])
		if override, ok := data.Overrides[rawName]; ok {
			fmt.Fprintf(&builder, "      type: %s\n", override)
		}
	}
	return builder.String()
}

func sortedMapKeys[V any](data map[string]V) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func benchmarkOpenMetricsRun(b *testing.B, instance string, payload string) {
	b.Helper()
	benchmarkOpenMetricsRunWithSetup(b, instance, payload, nil)
}

func benchmarkOpenMetricsRunWithContentType(b *testing.B, instance string, payload string, contentType string) {
	b.Helper()
	benchmarkOpenMetricsRunWithSetupAndContentType(b, instance, payload, contentType, nil)
}

func benchmarkOpenMetricsRunWithSetup(b *testing.B, instance string, payload string, setup func(*openmetricsScraper)) {
	b.Helper()
	benchmarkOpenMetricsRunWithSetupAndContentType(b, instance, payload, "text/plain; version=0.0.4", setup)
}

func benchmarkOpenMetricsRunWithSetupAndContentType(b *testing.B, instance string, payload string, contentType string, setup func(*openmetricsScraper)) {
	b.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(payload))
	}))
	b.Cleanup(server.Close)

	cfg := configmock.New(b)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	instance = strings.ReplaceAll(instance, "%%endpoint%%", server.URL)
	senderManager := &benchmarkSenderManager{sender: benchmarkSender{}}
	if err := omCheck.Configure(senderManager, integration.FakeConfigHash, integration.Data([]byte(instance)), nil, "benchmark", "provider"); err != nil {
		b.Fatal(err)
	}
	if setup != nil {
		setup(omCheck.scraper.inner)
	}
	if err := omCheck.Run(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := omCheck.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

func runOpenMetricsFixtureOnce(t *testing.T, instance string, payload string) *mocksender.MockSender {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)

	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	instance = strings.ReplaceAll(instance, "%%endpoint%%", server.URL)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, omCheck.Configure(senderManager, integration.FakeConfigHash, integration.Data([]byte(instance)), nil, "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(omCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()
	require.NoError(t, omCheck.Run())
	return mockSender
}

func mockSenderHasMetricCall(mockSender *mocksender.MockSender) bool {
	metricMethods := map[string]struct{}{
		"Gauge":                             {},
		"GaugeNoIndex":                      {},
		"Rate":                              {},
		"Count":                             {},
		"MonotonicCount":                    {},
		"MonotonicCountWithFlushFirstValue": {},
		"Counter":                           {},
		"Histogram":                         {},
		"Historate":                         {},
		"Distribution":                      {},
		"HistogramBucket":                   {},
		"OpenmetricsBucket":                 {},
	}
	for _, call := range mockSender.Mock.Calls {
		if _, ok := metricMethods[call.Method]; ok {
			return true
		}
	}
	return false
}

type benchmarkSenderManager struct {
	sender benchmarkSender
}

func (m *benchmarkSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	return m.sender, nil
}

func (m *benchmarkSenderManager) SetSender(sender.Sender, checkid.ID) error {
	return nil
}

func (m *benchmarkSenderManager) DestroySender(checkid.ID) {}

func (m *benchmarkSenderManager) GetDefaultSender() (sender.Sender, error) {
	return m.sender, nil
}

type benchmarkSender struct{}

func (benchmarkSender) Commit()                                                                   {}
func (benchmarkSender) Gauge(string, float64, string, []string)                                   {}
func (benchmarkSender) GaugeNoIndex(string, float64, string, []string)                            {}
func (benchmarkSender) Rate(string, float64, string, []string)                                    {}
func (benchmarkSender) Count(string, float64, string, []string)                                   {}
func (benchmarkSender) MonotonicCount(string, float64, string, []string)                          {}
func (benchmarkSender) MonotonicCountWithFlushFirstValue(string, float64, string, []string, bool) {}
func (benchmarkSender) Counter(string, float64, string, []string)                                 {}
func (benchmarkSender) Histogram(string, float64, string, []string)                               {}
func (benchmarkSender) Historate(string, float64, string, []string)                               {}
func (benchmarkSender) Distribution(string, float64, string, []string)                            {}
func (benchmarkSender) ServiceCheck(string, servicecheck.ServiceCheckStatus, string, []string, string) {
}
func (benchmarkSender) HistogramBucket(string, int64, float64, float64, bool, string, []string, bool) {
}
func (benchmarkSender) OpenmetricsBucket(string, int64, float64, float64, bool, string, []string, bool) {
}
func (benchmarkSender) GaugeWithTimestamp(string, float64, string, []string, float64) error {
	return nil
}
func (benchmarkSender) CountWithTimestamp(string, float64, string, []string, float64) error {
	return nil
}
func (benchmarkSender) Event(event.Event)                                            {}
func (benchmarkSender) EventPlatformEvent([]byte, string)                            {}
func (benchmarkSender) GetSenderStats() stats.SenderStats                            { return stats.SenderStats{} }
func (benchmarkSender) DisableDefaultHostname(bool)                                  {}
func (benchmarkSender) SetCheckCustomTags([]string)                                  {}
func (benchmarkSender) SetInfraTagger(*infratags.Tagger)                             {}
func (benchmarkSender) SetCheckService(string)                                       {}
func (benchmarkSender) SetNoIndex(bool)                                              {}
func (benchmarkSender) FinalizeCheckServiceTag()                                     {}
func (benchmarkSender) OrchestratorMetadata([]types.ProcessMessageBody, string, int) {}
func (benchmarkSender) OrchestratorManifest([]types.ProcessMessageBody, string)      {}
