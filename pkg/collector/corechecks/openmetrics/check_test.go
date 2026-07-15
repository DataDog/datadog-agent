// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

type checkRun struct {
	sender   *mocksender.MockSender
	endpoint string
	check    *Check
	checkID  string
}

func runOpenMetricsCheck(t *testing.T, instance string, payload string) checkRun {
	t.Helper()

	run := configureOpenMetricsCheck(t, instance, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	run.run(t)
	return run
}

func runOpenMetricsCheckWithResponse(t *testing.T, instance string, payload string, statusCode int, contentType string) checkRun {
	t.Helper()

	run := configureOpenMetricsCheck(t, instance, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		}
		w.WriteHeader(statusCode)
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	run.run(t)
	return run
}

func configureOpenMetricsCheck(t *testing.T, instance string, handler http.HandlerFunc) checkRun {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return configureOpenMetricsCheckForEndpoint(t, instance, server.URL)
}

func configureOpenMetricsCheckForEndpoint(t *testing.T, instance string, endpoint string) checkRun {
	t.Helper()

	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	instance = strings.ReplaceAll(instance, "%%endpoint%%", endpoint)
	err := omCheck.Configure(senderManager, integration.FakeConfigHash, integration.Data([]byte(instance)), nil, "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(omCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	return checkRun{sender: mockSender, endpoint: endpoint, check: omCheck, checkID: string(omCheck.ID())}
}

func configureOpenMetricsCheckWithoutServer(t *testing.T, instance string) checkRun {
	t.Helper()

	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := omCheck.Configure(senderManager, integration.FakeConfigHash, integration.Data([]byte(instance)), nil, "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(omCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	return checkRun{sender: mockSender, check: omCheck, checkID: string(omCheck.ID())}
}

func (r checkRun) run(t *testing.T) {
	t.Helper()
	require.NoError(t, r.check.Run())
}

func TestConfigureSkipsWhenCoreLoaderFlagDisabled(t *testing.T) {
	for _, fl := range []string{flavor.DefaultAgent, flavor.IotAgent, flavor.ClusterAgent} {
		t.Run(fl, func(t *testing.T) {
			flavor.SetTestFlavor(t, fl)
			cfg := configmock.New(t)
			cfg.Set("openmetrics.use_core_loader", false, configmodel.SourceAgentRuntime)

			omCheck := newCheck().(*Check)
			err := omCheck.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte(`openmetrics_endpoint: http://127.0.0.1/metrics`), nil, "test", "provider")

			require.True(t, errors.Is(err, check.ErrSkipCheckInstance))
		})
	}
}

func TestConfigureSkipsUnsupportedCoreHTTPConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	err := omCheck.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte(`
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
auth_type: digest
`), nil, "test", "provider")

	require.True(t, errors.Is(err, check.ErrSkipCheckInstance))
	require.ErrorContains(t, err, "auth_type `digest`")
}

func TestConfigureFinalizesServiceTag(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	instance := integration.Data([]byte(`
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
service: instance-service
`))
	initConfig := integration.Data([]byte("service: init-service\n"))
	omCheck := newCheck().(*Check)
	omCheck.BuildID(integration.FakeConfigHash, instance, initConfig)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager(omCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, omCheck.Configure(senderManager, integration.FakeConfigHash, instance, initConfig, "test", "provider"))
	mockSender.Mock.AssertCalled(t, "SetCheckService", "init-service")
	mockSender.Mock.AssertCalled(t, "SetCheckService", "instance-service")
	mockSender.Mock.AssertNumberOfCalls(t, "FinalizeCheckServiceTag", 1)
}

func TestBasicAuthorizationEncodingParity(t *testing.T) {
	legacy, err := basicAuthorization("caf\u00e9", "cr\u00e8me", true)
	require.NoError(t, err)
	require.Equal(t, "Basic Y2Fm6TpjcuhtZQ==", legacy)

	utf8, err := basicAuthorization("caf\u00e9", "cr\u00e8me", false)
	require.NoError(t, err)
	require.Equal(t, "Basic Y2Fmw6k6Y3LDqG1l", utf8)

	_, err = basicAuthorization("snowman \u2603", "password", true)
	require.ErrorContains(t, err, "cannot be encoded as latin-1")
}

func TestLatestConfigValidationParity(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	tests := []struct {
		name     string
		instance string
		error    string
	}{
		{
			name: "endpoint type",
			instance: `
openmetrics_endpoint: 9000
metrics: []
`,
			error: "the setting `openmetrics_endpoint` must be a string",
		},
		{
			name: "endpoint required",
			instance: `
openmetrics_endpoint: ""
metrics: []
`,
			error: "the setting `openmetrics_endpoint` is required",
		},
		{
			name: "metrics array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: 9000
`,
			error: "setting `metrics` must be an array",
		},
		{
			name: "namespace type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
namespace: 9000
metrics: []
`,
			error: "setting `namespace` must be a string",
		},
		{
			name: "raw metric prefix type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
raw_metric_prefix: 9000
metrics: []
`,
			error: "setting `raw_metric_prefix` must be a string",
		},
		{
			name: "hostname label type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
hostname_label: 9000
metrics: []
`,
			error: "setting `hostname_label` must be a string",
		},
		{
			name: "hostname format type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
hostname_format: 9000
metrics: []
`,
			error: "setting `hostname_format` must be a string",
		},
		{
			name: "hostname format placeholder",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
hostname_label: node
hostname_format: node
metrics: []
`,
			error: "setting `hostname_format` does not contain the placeholder `<HOSTNAME>`",
		},
		{
			name: "exclude labels array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_labels: 9000
metrics: []
`,
			error: "setting `exclude_labels` must be an array",
		},
		{
			name: "exclude labels entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_labels:
  - 9000
metrics: []
`,
			error: "entry #1 of setting `exclude_labels` must be a string",
		},
		{
			name: "include labels array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
include_labels: 9000
metrics: []
`,
			error: "setting `include_labels` must be an array",
		},
		{
			name: "include labels entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
include_labels:
  - 9000
metrics: []
`,
			error: "entry #1 of setting `include_labels` must be a string",
		},
		{
			name: "rename labels mapping",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
rename_labels: 9000
metrics: []
`,
			error: "setting `rename_labels` must be a mapping",
		},
		{
			name: "rename labels value",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
rename_labels:
  foo: 9000
metrics: []
`,
			error: "value for label `foo` of setting `rename_labels` must be a string",
		},
		{
			name: "exclude metrics array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_metrics: 9000
metrics: []
`,
			error: "setting `exclude_metrics` must be an array",
		},
		{
			name: "exclude metrics entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_metrics:
  - 9000
metrics: []
`,
			error: "entry #1 of setting `exclude_metrics` must be a string",
		},
		{
			name: "exclude metrics by labels mapping",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_metrics_by_labels: 9000
metrics: []
`,
			error: "setting `exclude_metrics_by_labels` must be a mapping",
		},
		{
			name: "exclude metrics by labels value",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_metrics_by_labels:
  foo:
    - 9000
metrics: []
`,
			error: "value #1 for label `foo` of setting `exclude_metrics_by_labels` must be a string",
		},
		{
			name: "exclude metrics by labels invalid type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
exclude_metrics_by_labels:
  foo: 9000
metrics: []
`,
			error: "label `foo` of setting `exclude_metrics_by_labels` must be an array or set to `true`",
		},
		{
			name: "tags array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
tags: 9000
metrics: []
`,
			error: "setting `tags` must be an array",
		},
		{
			name: "tags entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
tags:
  - 9000
metrics: []
`,
			error: "entry #1 of setting `tags` must be a string",
		},
		{
			name: "raw line filters array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
raw_line_filters: 9000
metrics: []
`,
			error: "setting `raw_line_filters` must be an array",
		},
		{
			name: "raw line filters entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
raw_line_filters:
  - 9000
metrics: []
`,
			error: "entry #1 of setting `raw_line_filters` must be a string",
		},
		{
			name: "raw line filters invalid pattern",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
raw_line_filters:
  - "["
metrics: []
`,
			error: "missing closing ]",
		},
		{
			name: "metrics entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - 9000
`,
			error: "entry #1 of setting `metrics` must be a string or a mapping",
		},
		{
			name: "metrics mapped value",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo: 9000
`,
			error: "value of entry `foo` of setting `metrics` must be a string or a mapping",
		},
		{
			name: "metrics config name",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      name: 9000
`,
			error: "error compiling transformer for metric `foo`: field `name` must be a string",
		},
		{
			name: "metrics config type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: 9000
`,
			error: "error compiling transformer for metric `foo`: field `type` must be a string",
		},
		{
			name: "metrics config unknown type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: bar
`,
			error: "error compiling transformer for metric `foo`: unknown type `bar`",
		},
		{
			name: "extra metrics array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
extra_metrics: 9000
`,
			error: "setting `extra_metrics` must be an array",
		},
		{
			name: "extra metrics entry type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
extra_metrics:
  - 9000
`,
			error: "entry #1 of setting `extra_metrics` must be a string or a mapping",
		},
		{
			name: "extra metrics mapped value",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
extra_metrics:
  - foo: 9000
`,
			error: "value of entry `foo` of setting `extra_metrics` must be a string or a mapping",
		},
		{
			name: "temporal percent no scale",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: temporal_percent
`,
			error: "the `scale` parameter is required",
		},
		{
			name: "temporal percent unknown scale",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: temporal_percent
      scale: bar
`,
			error: "the `scale` parameter must be one of:",
		},
		{
			name: "temporal percent scale type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: temporal_percent
      scale: 1.23
`,
			error: "the `scale` parameter must be an integer representing parts of a second",
		},
		{
			name: "service check no status map",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
`,
			error: "the `status_map` parameter is required",
		},
		{
			name: "service check status map type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
      status_map: 5
`,
			error: "the `status_map` parameter must be a mapping",
		},
		{
			name: "service check status map empty",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
      status_map: {}
`,
			error: "the `status_map` parameter must not be empty",
		},
		{
			name: "service check status map value",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
      status_map:
        true: OK
`,
			error: "does not represent an integer",
		},
		{
			name: "service check status map status type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
      status_map:
        "9000": 0
`,
			error: "is not a string",
		},
		{
			name: "service check status map invalid status",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: service_check
      status_map:
        "9000": 0k
`,
			error: "invalid status `0k`",
		},
		{
			name: "metadata label type",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: metadata
      label: 9000
`,
			error: "the `label` parameter must be a string",
		},
		{
			name: "metadata no label",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics:
  - foo:
      type: metadata
`,
			error: "the `label` parameter is required",
		},
		{
			name: "share labels entry",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info: 9000
`,
			error: "metric `build_info` of setting `share_labels` must be a mapping or set to `true`",
		},
		{
			name: "share labels mapping",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels: 9000
`,
			error: "setting `share_labels` must be a mapping",
		},
		{
			name: "share labels values array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    values: 9000
`,
			error: "option `values` for metric `build_info` of setting `share_labels` must be an array",
		},
		{
			name: "share labels values entry",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    values:
      - 1.0
`,
			error: "entry #1 of option `values` for metric `build_info` of setting `share_labels` must represent an integer",
		},
		{
			name: "share labels labels array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    labels: 9000
`,
			error: "option `labels` for metric `build_info` of setting `share_labels` must be an array",
		},
		{
			name: "share labels labels entry",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    labels:
      - 9000
`,
			error: "entry #1 of option `labels` for metric `build_info` of setting `share_labels` must be a string",
		},
		{
			name: "share labels match array",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    match: 9000
`,
			error: "option `match` for metric `build_info` of setting `share_labels` must be an array",
		},
		{
			name: "share labels match entry",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
share_labels:
  build_info:
    match:
      - 9000
`,
			error: "entry #1 of option `match` for metric `build_info` of setting `share_labels` must be a string",
		},
		{
			name: "target info boolean",
			instance: `
openmetrics_endpoint: http://127.0.0.1/metrics
metrics: []
target_info: "true"
`,
			error: "setting `target_info` must be a boolean",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			omCheck := newCheck().(*Check)
			err := omCheck.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte(test.instance), nil, "test", "provider")
			require.ErrorContains(t, err, test.error)
		})
	}
}

func TestLatestGaugeCounterTagsAndHealth(t *testing.T) {
	payload := `
# HELP demo_temperature Demo gauge.
# TYPE demo_temperature gauge
demo_temperature{pod="api",instance="i-1"} 42
# HELP http_requests_total Requests.
# TYPE http_requests_total counter
http_requests_total{method="GET"} 7
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - demo_temperature
  - http_requests
tags:
  - env:test
  - drop:me
ignore_tags:
  - "^drop:"
rename_labels:
  pod: kube_pod
exclude_labels:
  - instance
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.demo_temperature", 42, "", []string{"kube_pod:api", "env:test", "endpoint:" + run.endpoint})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "test.demo_temperature", []string{"instance:i-1"})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "test.demo_temperature", []string{nameLabel + ":demo_temperature"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.http_requests.count", 7, "", []string{"method:GET", "env:test", "endpoint:" + run.endpoint}, false)
	run.sender.AssertServiceCheck(t, "test.openmetrics.health", servicecheck.ServiceCheckOK, "", []string{"env:test", "endpoint:" + run.endpoint}, "")
}

func TestLatestOpenMetricsStrictSpec(t *testing.T) {
	payload := `# TYPE metric1 gauge
metric1{node="host1",flavor="test",matched_label="foobar"} 99.9
# TYPE metric2 gauge
metric2{node="host2",timestamp="123",matched_label="foobar"} 12.2
# TYPE counter1 counter
counter1{node="host2"} 42
# EOF
`
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1: renamed.metric1
  - metric2
  - counter1
collect_histogram_buckets: true
use_latest_spec: true
`, payload, http.StatusOK, "text/plain")

	run.sender.AssertMetric(t, "Gauge", "openmetrics.renamed.metric1", 99.9, "", []string{"endpoint:" + run.endpoint, "node:host1", "flavor:test", "matched_label:foobar"})
	run.sender.AssertMetric(t, "Gauge", "openmetrics.metric2", 12.2, "", []string{"endpoint:" + run.endpoint, "node:host2", "timestamp:123", "matched_label:foobar"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "openmetrics.counter1.count", 42, "", []string{"endpoint:" + run.endpoint, "node:host2"}, false)
}

func TestLatestEmptyResponseAndIgnoredConnection(t *testing.T) {
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - ".+"
`, "", http.StatusOK, "text/plain")
	run.sender.AssertMetricMissing(t, "Gauge", "test.anything")

	ignored := configureOpenMetricsCheckWithoutServer(t, `
openmetrics_endpoint: http://127.0.0.1:1/metrics
namespace: test
metrics:
 - ".+"
ignore_connection_errors: true
`)
	require.NoError(t, ignored.check.Run())
	ignored.sender.Mock.AssertCalled(t, "ServiceCheck", "test.openmetrics.health", servicecheck.ServiceCheckCritical, "", mocksender.MatchTagsContains([]string{"endpoint:http://127.0.0.1:1/metrics"}), mock.AnythingOfType("string"))
}

func TestLatestHealthServiceCheckOptions(t *testing.T) {
	okRun := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
tags:
  - foo:bar
`, `
# TYPE app_up gauge
app_up 1
`)
	okRun.sender.AssertServiceCheck(t, "test.openmetrics.health", servicecheck.ServiceCheckOK, "", []string{"endpoint:" + okRun.endpoint, "foo:bar"}, "")

	criticalRun := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
tags:
  - foo:bar
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	require.Error(t, criticalRun.check.Run())
	criticalRun.sender.AssertServiceCheck(t, "test.openmetrics.health", servicecheck.ServiceCheckCritical, "", []string{"endpoint:" + criticalRun.endpoint, "foo:bar"}, "unexpected status code 401 scraping "+criticalRun.endpoint)

	disabledRun := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
enable_health_service_check: false
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	require.Error(t, disabledRun.check.Run())
	disabledRun.sender.Mock.AssertNotCalled(t, "ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestLatestOptionsParity(t *testing.T) {
	payload := `
# TYPE foo_go_memstats_alloc_bytes gauge
foo_go_memstats_alloc_bytes{foo="bar"} 6396288
# TYPE go_memstats_gc_sys_bytes gauge
go_memstats_gc_sys_bytes{bar="foo"} 901120
# TYPE go_memstats_free_bytes gauge
go_memstats_free_bytes{foo="bar",zip="zap"} 6396288
# TYPE go_memstats_other_bytes untyped
go_memstats_other_bytes{foo="baz"} 5
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - go_memstats_alloc_bytes
  - go_memstats_gc_sys_bytes
  - go_memstats_free_bytes
  - go_memstats_other_bytes:
      type: gauge
raw_metric_prefix: foo_
hostname_label: foo
hostname_format: region_<HOSTNAME>
include_labels:
  - foo
  - zip
exclude_labels:
  - zip
rename_labels:
  foo: renamed
exclude_metrics:
  - "^go_memstats_free_bytes$"
exclude_metrics_by_labels:
  foo:
    - baz
tags:
  - keep:tag
  - drop:tag
ignore_tags:
  - "^drop:"
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.go_memstats_alloc_bytes", 6396288, "region_bar", []string{"endpoint:" + run.endpoint, "renamed:bar", "keep:tag"})
	run.sender.AssertMetric(t, "Gauge", "test.go_memstats_gc_sys_bytes", 901120, "", []string{"endpoint:" + run.endpoint, "keep:tag"})
	run.sender.AssertMetricMissing(t, "Gauge", "test.go_memstats_free_bytes")
	run.sender.AssertMetricMissing(t, "Gauge", "test.go_memstats_other_bytes")
}

func TestEmptyHostnameLabelDoesNotOverrideDefaultHostname(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
hostname_label: node
hostname_format: <HOSTNAME>.cluster
`, `
# TYPE app_up gauge
app_up{node=""} 1
`)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"node:", "endpoint:" + run.endpoint})
}

func TestLatestExcludeAllByLabelAndRawLineFilters(t *testing.T) {
	payload := `
# TYPE kept gauge
kept{foo="bar"} 1
# TYPE dropped_by_label gauge
dropped_by_label{foo="baz"} 2
# TYPE dropped_by_line gauge
dropped_by_line{foo=""} 3
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - ".+"
exclude_metrics_by_labels:
  foo: true
raw_line_filters:
  - '=""'
`, payload)

	run.sender.AssertMetricMissing(t, "Gauge", "test.kept")
	run.sender.AssertMetricMissing(t, "Gauge", "test.dropped_by_label")
	run.sender.AssertMetricMissing(t, "Gauge", "test.dropped_by_line")
}

func TestLatestTransformerParity(t *testing.T) {
	startedAt := float64(time.Now().Unix() - 60)
	payload := `
# TYPE foo_total counter
foo_total 9339544592
# TYPE bar_count counter
bar_count{bar="foo"} 128219257
# TYPE state gauge
state{foo="bar"} 3
# TYPE unknown_state gauge
unknown_state 3
# TYPE rate_metric gauge
rate_metric 12
# TYPE cpu_time gauge
cpu_time 25
# TYPE started_at gauge
started_at ` + strconv.FormatFloat(startedAt, 'f', -1, 64) + `
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - ".+"
  - state:
      type: service_check
      status_map:
        "3": ok
  - unknown_state:
      type: service_check
      status_map:
        "7": ok
  - rate_metric:
      type: rate
  - cpu_time:
      type: temporal_percent
      scale: second
  - started_at:
      type: time_elapsed
`, payload)

	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.foo.count", 9339544592, "", []string{"endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.bar_count.count", 128219257, "", []string{"endpoint:" + run.endpoint, "bar:foo"}, false)
	run.sender.AssertServiceCheck(t, "test.state", servicecheck.ServiceCheckOK, "", []string{"endpoint:" + run.endpoint}, "")
	run.sender.AssertServiceCheck(t, "test.unknown_state", servicecheck.ServiceCheckUnknown, "", []string{"endpoint:" + run.endpoint}, "")
	run.sender.AssertMetric(t, "Rate", "test.rate_metric", 12, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetric(t, "Rate", "test.cpu_time", 2500, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetricInRange(t, "Gauge", "test.started_at", 59, 90, "", []string{"endpoint:" + run.endpoint})
}

func TestLatestSummaryAndNativeDynamicParity(t *testing.T) {
	payload := `
# TYPE http_request_duration_microseconds summary
http_request_duration_microseconds{handler="prometheus",quantile="0.5"} 1599.011
http_request_duration_microseconds{handler="prometheus",quantile="0.9"} 1599.011
http_request_duration_microseconds_sum{handler="prometheus"} 65093.229
http_request_duration_microseconds_count{handler="prometheus"} 25
# TYPE dynamic_metric gauge
dynamic_metric{foo="bar"} 7
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - http_request_duration_microseconds:
      name: http_request_duration_microseconds
      type: summary
  - dynamic_metric:
      type: native_dynamic
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.http_request_duration_microseconds.quantile", 1599.011, "", []string{"endpoint:" + run.endpoint, "handler:prometheus", "quantile:0.5"})
	run.sender.AssertMetric(t, "Gauge", "test.http_request_duration_microseconds.quantile", 1599.011, "", []string{"endpoint:" + run.endpoint, "handler:prometheus", "quantile:0.9"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.http_request_duration_microseconds.sum", 65093.229, "", []string{"endpoint:" + run.endpoint, "handler:prometheus"}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.http_request_duration_microseconds.count", 25, "", []string{"endpoint:" + run.endpoint, "handler:prometheus"}, false)
	run.sender.AssertMetric(t, "Gauge", "test.dynamic_metric", 7, "", []string{"endpoint:" + run.endpoint, "foo:bar"})
}

func TestLatestCounterGaugeNativeDynamicAndTypeOverrideParity(t *testing.T) {
	counterGauge := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - go_memstats_alloc_bytes:
      type: counter_gauge
  - go_memstats_frees:
      type: counter_gauge
`, `
# TYPE go_memstats_alloc_bytes_total counter
go_memstats_alloc_bytes_total{foo="bar"} 9339544592
# TYPE go_memstats_frees_total counter
go_memstats_frees_total{bar="foo"} 128219257
`)
	counterGauge.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.go_memstats_alloc_bytes.count", 9339544592, "", []string{"endpoint:" + counterGauge.endpoint, "foo:bar"}, false)
	counterGauge.sender.AssertMetric(t, "Gauge", "test.go_memstats_alloc_bytes.total", 9339544592, "", []string{"endpoint:" + counterGauge.endpoint, "foo:bar"})
	counterGauge.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.go_memstats_frees.count", 128219257, "", []string{"endpoint:" + counterGauge.endpoint, "bar:foo"}, false)
	counterGauge.sender.AssertMetric(t, "Gauge", "test.go_memstats_frees.total", 128219257, "", []string{"endpoint:" + counterGauge.endpoint, "bar:foo"})

	nativeDynamic := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - go_memstats_alloc_bytes:
      type: native_dynamic
`, `
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 1900688
# TYPE go_memstats_alloc_bytes_total counter
go_memstats_alloc_bytes_total 258684656
`)
	nativeDynamic.sender.AssertMetric(t, "Gauge", "test.go_memstats_alloc_bytes", 1900688, "", []string{"endpoint:" + nativeDynamic.endpoint})
	nativeDynamic.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.go_memstats_alloc_bytes.count", 258684656, "", []string{"endpoint:" + nativeDynamic.endpoint}, false)

	for _, metricType := range []string{"counter", "gauge"} {
		t.Run("untyped override "+metricType, func(t *testing.T) {
			run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - foo:
      name: foo
      type: `+metricType+`
  - bar_total:
      name: bar
      type: `+metricType+`
  - baz_total:
      name: baz
      type: `+metricType+`
  - fiz:
      name: fiz
      type: `+metricType+`
  - bux:
      name: bux
      type: `+metricType+`
`, `
# TYPE foo untyped
foo 0
# TYPE bar_total untyped
bar_total 1
# TYPE baz untyped
baz_total 2
# TYPE qux untyped
fiz 3
# TYPE bux histogram
bux 4
`)
			expected := map[string]float64{
				"foo": 0,
				"bar": 1,
				"baz": 2,
				"fiz": 3,
				"bux": 4,
			}
			for metric, value := range expected {
				if metricType == "counter" {
					run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test."+metric+".count", value, "", []string{"endpoint:" + run.endpoint}, false)
				} else {
					run.sender.AssertMetric(t, "Gauge", "test."+metric, value, "", []string{"endpoint:" + run.endpoint})
				}
			}
		})
	}
}

func TestLatestHistogramDisableBuckets(t *testing.T) {
	payload := `
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{route="/",le="0.1"} 2
request_duration_seconds_bucket{route="/",le="0.5"} 5
request_duration_seconds_bucket{route="/",le="+Inf"} 5
request_duration_seconds_sum{route="/"} 1.2
request_duration_seconds_count{route="/"} 5
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - request_duration_seconds
collect_histogram_buckets: false
`, payload)

	run.sender.Mock.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 1.2, "", []string{"route:/", "endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 5, "", []string{"route:/", "endpoint:" + run.endpoint}, false)
}

func TestTLSHostAndCipherOptions(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("openmetrics.use_core_loader", true, configmodel.SourceAgentRuntime)

	omCheck := newCheck().(*Check)
	err := omCheck.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte(`
openmetrics_endpoint: https://127.0.0.1/metrics
namespace: test
metrics: []
headers:
  Host: metrics.example.com:8443
tls_use_host_header: true
tls_ciphers:
  - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
`), nil, "test", "provider")
	require.NoError(t, err)

	transport, ok := omCheck.scraper.inner.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, "metrics.example.com", transport.TLSClientConfig.ServerName)
	require.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, transport.TLSClientConfig.CipherSuites)
}

func TestAllowRedirectsFalseAcceptsRedirectResponse(t *testing.T) {
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
allow_redirects: false
`, "# TYPE app_up gauge\napp_up 1\n", http.StatusFound, "text/plain; version=0.0.4")

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestPersistConnectionsPersistsCookies(t *testing.T) {
	var requests atomic.Int32
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
persist_connections: true
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "openmetrics"})
		} else {
			cookie, err := request.Cookie("session")
			require.NoError(t, err)
			require.Equal(t, "openmetrics", cookie.Value)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte("# TYPE app_up gauge\napp_up 1\n"))
		require.NoError(t, err)
	}))

	run.run(t)
	run.run(t)
}

func TestAuthTokenRefreshesAndRetriesAfterRejection(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "auth-token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("first-token\n"), 0o600))

	var requests atomic.Int32
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
auth_token:
  reader:
    type: file
    path: `+tokenPath+`
  writer:
    type: header
    name: Authorization
    value: Bearer <TOKEN>
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			require.Equal(t, "Bearer first-token", request.Header.Get("Authorization"))
			require.NoError(t, os.WriteFile(tokenPath, []byte("second-token\n"), 0o600))
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		require.Equal(t, "Bearer second-token", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte("# TYPE app_up gauge\napp_up 1\n"))
		require.NoError(t, err)
	}))

	run.run(t)
	require.Equal(t, int32(2), requests.Load())
	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestProxyNoProxyBypassesProxy(t *testing.T) {
	proxyCalled := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalled = true
		http.Error(w, "proxy should have been bypassed", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	payload := `
# TYPE app_up gauge
app_up 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
proxy:
  http: `+proxy.URL+`
  no_proxy:
    - 127.0.0.1
`, payload)

	require.False(t, proxyCalled)
	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestProxyUsedWhenNoProxyDoesNotMatch(t *testing.T) {
	payload := `
# TYPE app_up gauge
app_up 1
`
	proxyCalled := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		proxyCalled = true
		require.Equal(t, "http://example.com/metrics", request.URL.String())
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	t.Cleanup(proxy.Close)

	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: http://example.com/metrics
namespace: test
metrics:
  - app_up
proxy:
  http: `+proxy.URL+`
  no_proxy: .internal.example
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "direct endpoint should not be used", http.StatusBadGateway)
	}))
	run.run(t)

	require.True(t, proxyCalled)
	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:http://example.com/metrics"})
}

func TestLatestHistogramNonCumulativeBuckets(t *testing.T) {
	payload := `
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{route="/",le="0.1"} 2
request_duration_seconds_bucket{route="/",le="0.5"} 5
request_duration_seconds_bucket{route="/",le="+Inf"} 5
request_duration_seconds_sum{route="/"} 1.2
request_duration_seconds_count{route="/"} 5
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - request_duration_seconds
non_cumulative_histogram_buckets: true
`, payload)

	baseTags := []string{"route:/", "endpoint:" + run.endpoint}
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", 2, "", append(baseTags, "upper_bound:0.1", "lower_bound:0"), false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", 3, "", append(baseTags, "upper_bound:0.5", "lower_bound:0.1"), false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 1.2, "", baseTags, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 5, "", baseTags, false)
}

func TestLatestHistogramBucketsAsDistributions(t *testing.T) {
	payload := `
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{route="/",le="0.1"} 2
request_duration_seconds_bucket{route="/",le="0.5"} 5
request_duration_seconds_bucket{route="/",le="+Inf"} 5
request_duration_seconds_sum{route="/"} 1.2
request_duration_seconds_count{route="/"} 5
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - request_duration_seconds
histogram_buckets_as_distributions: true
collect_counters_with_distributions: true
`, payload)

	run.sender.Mock.AssertCalled(t, "HistogramBucket", "test.request_duration_seconds", int64(2), 0.0, 0.1, true, "", mocksender.MatchTagsContains([]string{"route:/", "upper_bound:0.1", "lower_bound:0", "endpoint:" + run.endpoint}), false)
	run.sender.Mock.AssertCalled(t, "HistogramBucket", "test.request_duration_seconds", int64(3), 0.1, 0.5, true, "", mocksender.MatchTagsContains([]string{"route:/", "upper_bound:0.5", "lower_bound:0.1", "endpoint:" + run.endpoint}), false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 1.2, "", []string{"route:/", "endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 5, "", []string{"route:/", "endpoint:" + run.endpoint}, false)
}

func TestLatestShareLabelsHostnameAndSampleExclusion(t *testing.T) {
	payload := `
# TYPE build_info gauge
build_info{pod="api",version="1.0.0"} 1
# TYPE app_up gauge
app_up{pod="api",node="node-a"} 1
app_up{pod="skip",node="node-b"} 1
# TYPE ignored_metric gauge
ignored_metric{pod="api"} 2
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
  - ignored_metric
share_labels:
  build_info:
    match:
      - pod
    labels:
      - version
    values:
      - "1"
exclude_metrics:
  - ignored_metric
exclude_metrics_by_labels:
  pod:
    - skip
hostname_label: node
hostname_format: <HOSTNAME>.cluster
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "node-a.cluster", []string{"pod:api", "version:1.0.0", "node:node-a", "endpoint:" + run.endpoint})
	run.sender.Mock.AssertNotCalled(t, "Gauge", "test.app_up", 1.0, "node-b.cluster", mock.AnythingOfType("[]string"))
	run.sender.AssertMetricMissing(t, "Gauge", "test.ignored_metric")
}

func TestLatestShareLabelsTrueSharesAllLabels(t *testing.T) {
	payload := `
# TYPE build_info gauge
build_info{pod="api",version="1.0.0",runtime="go"} 1
# TYPE app_up gauge
app_up{pod="api"} 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
share_labels:
  build_info: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"pod:api", "version:1.0.0", "runtime:go", "endpoint:" + run.endpoint})
}

func TestLatestTargetInfoLabels(t *testing.T) {
	payload := `
# TYPE target_info gauge
target_info{cluster="prod",region="us-east-1"} 1
# TYPE app_up gauge
app_up 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
target_info: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"cluster:prod", "region:us-east-1", "endpoint:" + run.endpoint})
}

func TestLatestRawLineFiltersAndTelemetry(t *testing.T) {
	payload := `
# TYPE kept gauge
kept 1
# TYPE ignored gauge
ignored 2
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - kept
  - ignored
raw_line_filters:
  - ignored
telemetry: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.kept", 1, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetricMissing(t, "Gauge", "test.ignored")
	run.sender.AssertMetric(t, "Count", "test.telemetry.metrics.blacklist.count", 2, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetric(t, "Count", "test.telemetry.metrics.input.count", 1, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetric(t, "Count", "test.telemetry.metrics.processed.count", 1, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetric(t, "Gauge", "test.telemetry.payload.size", float64(len(payload)), "", []string{"endpoint:" + run.endpoint})
}

func TestLegacyOpenMetricsParity(t *testing.T) {
	payload := `
# TYPE metric1 gauge
metric1{node="host1",flavor="test"} 1
# TYPE metric2 gauge
metric2{node="host2",timestamp="123"} 2
# TYPE counter1_total counter
counter1_total{node="host2"} 10
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1: renamed.metric1
  - metric2
  - counter1_total
send_histograms_buckets: true
send_monotonic_counter: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "openmetrics.renamed.metric1", 1, "", []string{"node:host1", "flavor:test"})
	run.sender.AssertMetric(t, "Gauge", "openmetrics.metric2", 2, "", []string{"node:host2", "timestamp:123"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "openmetrics.counter1_total", 10, "", []string{"node:host2"}, false)
	run.sender.AssertServiceCheck(t, "openmetrics.prometheus.health", servicecheck.ServiceCheckOK, "", nil, "")
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "openmetrics.metric2", []string{"endpoint:" + run.endpoint})
}

func TestLegacyCounterGaugeCompatibility(t *testing.T) {
	payload := `
# TYPE counter1_total counter
counter1_total{node="host2"} 10
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - counter1_total
send_monotonic_counter: false
send_monotonic_with_gauge: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "openmetrics.counter1_total", 10, "", []string{"node:host2"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "openmetrics.counter1_total.total", 10, "", []string{"node:host2"}, false)
}

func TestLegacyInvalidMetricAndWildcardCompatibility(t *testing.T) {
	payload := `
# TYPE metric1 gauge
metric1{node="host1",flavor="test",matched_label="foobar"} 1
# TYPE metric2 gauge
metric2{node="host2",timestamp="123",matched_label="foobar"} 2
`
	invalid := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1: renamed.metric1
  - metric2
  - metric3
`, payload)
	invalid.sender.AssertMetricMissing(t, "Gauge", "openmetrics.metric3")

	wildcard := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - metric*
`, payload)
	wildcard.sender.AssertMetric(t, "Gauge", "openmetrics.metric1", 1, "", []string{"node:host1", "flavor:test", "matched_label:foobar"})
	wildcard.sender.AssertMetric(t, "Gauge", "openmetrics.metric2", 2, "", []string{"node:host2", "timestamp:123", "matched_label:foobar"})
}

func TestMaxReturnedMetricsStillValidatesResponseTail(t *testing.T) {
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: openmetrics
metrics:
  - metric*
max_returned_metrics: 1
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(`
# TYPE metric1 gauge
metric1 1
# TYPE metric2 gauge
metric2{bad} 2
`))
		require.NoError(t, err)
	}))

	require.Error(t, run.check.Run())
}

func TestLatestDirectStreamFastPathSubmitsExactMetrics(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests
  - queue_depth
  - request_duration_seconds
`, `
# TYPE requests_total counter
requests_total{method="GET"} 10
# TYPE queue_depth gauge
queue_depth{queue="default"} 3
# TYPE ignored_metric gauge
ignored_metric{expensive_label="skipped"} 99
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 2
request_duration_seconds_bucket{le="+Inf"} 2
request_duration_seconds_sum 0.2
request_duration_seconds_count 2
`)

	require.True(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 10, "", []string{"endpoint:" + run.endpoint, "method:GET"}, false)
	run.sender.AssertMetric(t, "Gauge", "test.queue_depth", 3, "", []string{"endpoint:" + run.endpoint, "queue:default"})
	run.sender.AssertMetricMissing(t, "Gauge", "test.ignored_metric")
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", 2, "", []string{"endpoint:" + run.endpoint, "upper_bound:0.1"}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 0.2, "", []string{"endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 2, "", []string{"endpoint:" + run.endpoint}, false)
}

func TestLatestDirectStreamFastPathSubmitsSummary(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - request_latency_seconds
`, `
# TYPE request_latency_seconds summary
request_latency_seconds{route="/",quantile="0.5"} 0.25
request_latency_seconds{route="/",quantile="0.9"} 0.9
request_latency_seconds_sum{route="/"} 12.5
request_latency_seconds_count{route="/"} 50
# TYPE ignored_summary summary
ignored_summary{quantile="0.5"} 1
`)

	require.True(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMetric(t, "Gauge", "test.request_latency_seconds.quantile", 0.25, "", []string{"endpoint:" + run.endpoint, "route:/", "quantile:0.5"})
	run.sender.AssertMetric(t, "Gauge", "test.request_latency_seconds.quantile", 0.9, "", []string{"endpoint:" + run.endpoint, "route:/", "quantile:0.9"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_latency_seconds.sum", 12.5, "", []string{"endpoint:" + run.endpoint, "route:/"}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_latency_seconds.count", 50, "", []string{"endpoint:" + run.endpoint, "route:/"}, false)
	run.sender.AssertMetricMissing(t, "Gauge", "test.ignored_summary.quantile")
}

func TestLatestDirectStreamFastPathSubmitsPrimitiveTransformers(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests:
      type: counter
  - queue_depth:
      type: gauge
  - request_duration_seconds:
      type: histogram
`, `
# TYPE requests_total counter
requests_total{method="POST"} 7
# TYPE queue_depth gauge
queue_depth{queue="critical"} 4
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.5"} 6
request_duration_seconds_bucket{le="+Inf"} 6
request_duration_seconds_sum 1.5
request_duration_seconds_count 6
`)

	require.True(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 7, "", []string{"endpoint:" + run.endpoint, "method:POST"}, false)
	run.sender.AssertMetric(t, "Gauge", "test.queue_depth", 4, "", []string{"endpoint:" + run.endpoint, "queue:critical"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", 6, "", []string{"endpoint:" + run.endpoint, "upper_bound:0.5"}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 1.5, "", []string{"endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 6, "", []string{"endpoint:" + run.endpoint}, false)
}

func TestLatestOpenMetricsStreamSubmitsOpenMetricsText(t *testing.T) {
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests
  - queue_depth
  - request_duration_seconds
`, `# TYPE requests counter
requests_total{method="GET"} 10 123.456 # {trace_id="abc"} 7 456.789
requests_created{method="GET"} 100
# TYPE queue_depth gauge
queue_depth{queue="default"} 3
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 2 # {trace_id="bucket"} 1
request_duration_seconds_bucket{le="+Inf"} 2
request_duration_seconds_sum 0.2
request_duration_seconds_count 2
# EOF
`, http.StatusOK, "application/openmetrics-text; version=1.0.0")

	require.False(t, run.check.scraper.inner.canDirectStreamParse(true))
	require.True(t, run.check.scraper.inner.canOpenMetricsStreamParse(true))
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 10, "", []string{"endpoint:" + run.endpoint, "method:GET"}, false)
	run.sender.AssertMetricMissing(t, "Gauge", "test.requests_created")
	run.sender.AssertMetric(t, "Gauge", "test.queue_depth", 3, "", []string{"endpoint:" + run.endpoint, "queue:default"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.bucket", 2, "", []string{"endpoint:" + run.endpoint, "upper_bound:0.1"}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.sum", 0.2, "", []string{"endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.request_duration_seconds.count", 2, "", []string{"endpoint:" + run.endpoint}, false)
}

func TestLatestOpenMetricsStreamUsesCurrentOpenMetricsFamily(t *testing.T) {
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests
  - other
`, `# TYPE requests counter
# TYPE other gauge
other 5
requests_total{method="GET"} 10
# EOF
`, http.StatusOK, "application/openmetrics-text; version=1.0.0")

	require.False(t, run.check.scraper.inner.canDirectStreamParse(true))
	require.True(t, run.check.scraper.inner.canOpenMetricsStreamParse(true))
	run.sender.AssertMetric(t, "Gauge", "test.other", 5, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetricMissing(t, "MonotonicCountWithFlushFirstValue", "test.requests.count")
}

func TestLatestOpenMetricsStreamHandlesQuotedNamesAndLabels(t *testing.T) {
	run := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - gc_duration_seconds
`, `# TYPE "gc_duration_seconds" summary
{"gc_duration_seconds",quantile="0.5","strange.label"="ok"} 1.5
{"gc_duration_seconds_created"} 100
{"gc_duration_seconds_sum"} 3
{"gc_duration_seconds_count"} 2 # {"id.thing"="summary-count-test"} 1 123.321
# EOF
`, http.StatusOK, "application/openmetrics-text; version=1.0.0")

	require.False(t, run.check.scraper.inner.canDirectStreamParse(true))
	require.True(t, run.check.scraper.inner.canOpenMetricsStreamParse(true))
	run.sender.AssertMetric(t, "Gauge", "test.gc_duration_seconds.quantile", 1.5, "", []string{"endpoint:" + run.endpoint, "quantile:0.5", "strange.label:ok"})
	run.sender.AssertMetricMissing(t, "Gauge", "test.gc_duration_seconds_created")
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.gc_duration_seconds.sum", 3, "", []string{"endpoint:" + run.endpoint}, false)
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.gc_duration_seconds.count", 2, "", []string{"endpoint:" + run.endpoint}, false)
}

func TestLatestOpenMetricsStreamRequiresEOF(t *testing.T) {
	for name, payload := range map[string]string{
		"missing EOF": `# TYPE metric1 gauge
metric1 1
`,
		"blank line before EOF": `# TYPE metric1 gauge
metric1 1

# EOF
`,
		"leading whitespace": `# TYPE metric1 gauge
 metric1 1
# EOF
`,
		"data after EOF": `# TYPE metric1 gauge
metric1 1
# EOF
metric2 2
`,
	} {
		t.Run(name, func(t *testing.T) {
			run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/openmetrics-text; version=1.0.0")
				_, err := w.Write([]byte(payload))
				require.NoError(t, err)
			}))

			require.False(t, run.check.scraper.inner.canDirectStreamParse(true))
			require.True(t, run.check.scraper.inner.canOpenMetricsStreamParse(true))
			require.Error(t, run.check.Run())
		})
	}
}

func TestLatestDirectStreamSkippedMetricStillValidatesResponseTail(t *testing.T) {
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(`
# TYPE metric1 gauge
metric1 1
# TYPE ignored gauge
ignored{bad} 2
`))
		require.NoError(t, err)
	}))

	require.True(t, run.check.scraper.inner.canDirectStreamParse(false))
	require.Error(t, run.check.Run())
}

func TestLatestDirectStreamMaxReturnedMetricsStillValidatesResponseTail(t *testing.T) {
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: openmetrics
metrics:
  - metric1
  - metric2
max_returned_metrics: 1
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(`
# TYPE metric1 gauge
metric1 1
# TYPE metric2 gauge
metric2{bad} 2
`))
		require.NoError(t, err)
	}))

	require.True(t, run.check.scraper.inner.canDirectStreamParse(false))
	require.Error(t, run.check.Run())
}

func TestLatestDirectStreamDisabledForUnboundedScrapes(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - metric1
max_returned_metrics: -1
`, `
# TYPE metric1 gauge
metric1 1
`)

	require.False(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMetric(t, "Gauge", "test.metric1", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestLatestDirectStreamDisabledForTelemetry(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - metric1
telemetry: true
`, `
# TYPE metric1 gauge
metric1 1
`)

	require.False(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMetric(t, "Gauge", "test.metric1", 1, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetric(t, "Count", "test.telemetry.metrics.input.count", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestLatestDirectStreamDisabledForNonNativeTransformer(t *testing.T) {
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests:
      type: counter_gauge
`, `
# TYPE requests_total counter
requests_total 10
`)

	require.False(t, run.check.scraper.inner.canDirectStreamParse(false))
	run.sender.AssertMetric(t, "Gauge", "test.requests.total", 10, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 10, "", []string{"endpoint:" + run.endpoint}, false)
}

func TestIntegrationsCoreOpenMetricsFixtureParity(t *testing.T) {
	payload := `
# TYPE prometheus_target_interval_length_seconds summary
prometheus_target_interval_length_seconds{quantile="0.5"} 10
prometheus_target_interval_length_seconds_sum 20
prometheus_target_interval_length_seconds_count 2
# TYPE prometheus_http_request_duration_seconds histogram
prometheus_http_request_duration_seconds_bucket{le="0.5"} 4
prometheus_http_request_duration_seconds_bucket{le="+Inf"} 4
prometheus_http_request_duration_seconds_sum 1.4
prometheus_http_request_duration_seconds_count 4
# TYPE go_memstats_mallocs_total counter
go_memstats_mallocs_total 99
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 123
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - prometheus_target_interval_length_seconds: target_interval_seconds
  - prometheus_http_request_duration_seconds: http_req_duration_seconds
  - go_memstats_mallocs_total
  - go_memstats_alloc_bytes
`, payload)

	run.sender.AssertMetric(t, "Gauge", "openmetrics.target_interval_seconds.sum", 20, "", nil)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.target_interval_seconds.count", 2, "", nil)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.target_interval_seconds.quantile", 10, "", []string{"quantile:0.5"})
	run.sender.AssertMetric(t, "Gauge", "openmetrics.http_req_duration_seconds.sum", 1.4, "", nil)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.http_req_duration_seconds.count", 4, "", []string{"upper_bound:none"})
	run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "openmetrics.go_memstats_mallocs_total", 99, "", nil, false)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.go_memstats_alloc_bytes", 123, "", nil)
}

func TestLegacyLabelJoinsCompatibility(t *testing.T) {
	payload := `
# TYPE kube_global_labels gauge
kube_global_labels{cluster="prod",leader="true"} 1
# TYPE kube_local_labels gauge
kube_local_labels{foo="bar",ignored="yes"} 1
# TYPE kube_pod_info gauge
kube_pod_info{pod="api",namespace="default",node="node-a",pod_ip="10.0.0.1"} 1
kube_pod_info{pod="worker",namespace="default",node="node-b",pod_ip="10.0.0.2"} 1
# TYPE kube_pod_labels gauge
kube_pod_labels{pod="api",namespace="default",label_app="api"} 1
kube_pod_labels{pod="worker",namespace="default",label_app="worker"} 1
# TYPE kube_deployment_labels gauge
kube_deployment_labels{deployment="api",namespace="default",label_k8s_app="api",label_kubernetes_io_cluster_service="true"} 1
# TYPE kube_pod_status_ready gauge
kube_pod_status_ready{pod="api",namespace="default",condition="true"} 1
kube_pod_status_ready{pod="worker",namespace="default",condition="true"} 1
# TYPE kube_deployment_status_replicas gauge
kube_deployment_status_replicas{deployment="api",namespace="default"} 3
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: ksm
metrics:
  - kube_pod_status_ready: pod.ready
  - kube_deployment_status_replicas: deploy.replicas.available
label_joins:
  kube_global_labels:
    label_to_match: "*"
    labels_to_get:
      - "*"
  kube_local_labels:
    labels_to_match:
      - "*"
    labels_to_get:
      - foo
  kube_pod_info:
    label_to_match: pod
    labels_to_get:
      - node
      - pod_ip
  kube_pod_labels:
    labels_to_match:
      - pod
      - namespace
    labels_to_get:
      - "*"
  kube_deployment_labels:
    label_to_match:
      - deployment
    labels_to_get:
      - label_k8s_app
      - label_kubernetes_io_cluster_service
`, payload)

	run.sender.AssertMetric(t, "Gauge", "ksm.pod.ready", 1, "", []string{
		"pod:api",
		"namespace:default",
		"condition:true",
		"cluster:prod",
		"leader:true",
		"foo:bar",
		"node:node-a",
		"pod_ip:10.0.0.1",
		"label_app:api",
	})
	run.sender.AssertMetric(t, "Gauge", "ksm.pod.ready", 1, "", []string{
		"pod:worker",
		"namespace:default",
		"condition:true",
		"node:node-b",
		"pod_ip:10.0.0.2",
		"label_app:worker",
	})
	run.sender.AssertMetric(t, "Gauge", "ksm.deploy.replicas.available", 3, "", []string{
		"deployment:api",
		"namespace:default",
		"label_k8s_app:api",
		"label_kubernetes_io_cluster_service:true",
	})
}

func TestLegacyLabelJoinsIgnoresMissingLabelsAndMetrics(t *testing.T) {
	payload := `
# TYPE kube_pod_info gauge
kube_pod_info{pod="api",node="node-a",pod_ip="10.0.0.1"} 1
# TYPE kube_pod_status_ready gauge
kube_pod_status_ready{pod="api",namespace="default",condition="true"} 1
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: ksm
metrics:
  - kube_pod_status_ready: pod.ready
label_joins:
  kube_pod_info:
    label_to_match: not_existing
    labels_to_get:
      - node
      - pod_ip
  not_existing:
    label_to_match: pod
    labels_to_get:
      - node
`, payload)

	run.sender.AssertMetric(t, "Gauge", "ksm.pod.ready", 1, "", []string{"pod:api", "namespace:default", "condition:true"})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "ksm.pod.ready", []string{"node:node-a"})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "ksm.pod.ready", []string{"pod_ip:10.0.0.1"})
}

func TestLegacyLabelJoinsUpdateWithoutStaleValues(t *testing.T) {
	var payload atomic.Value
	payload.Store(`
# TYPE kube_pod_info gauge
kube_pod_info{pod="api",node="node-a"} 1
# TYPE kube_pod_status_phase gauge
kube_pod_status_phase{pod="api",phase="Running"} 1
# TYPE kube_pod_status_ready gauge
kube_pod_status_ready{pod="api"} 1
`)
	run := configureOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: ksm
metrics:
  - kube_pod_status_ready: pod.ready
label_joins:
  kube_pod_info:
    label_to_match: pod
    labels_to_get:
      - node
  kube_pod_status_phase:
    label_to_match: pod
    labels_to_get:
      - phase
`, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload.Load().(string)))
		require.NoError(t, err)
	}))
	run.run(t)
	run.sender.AssertMetric(t, "Gauge", "ksm.pod.ready", 1, "", []string{"pod:api", "node:node-a", "phase:Running"})

	payload.Store(`
# TYPE kube_pod_info gauge
kube_pod_info{pod="api",node="node-b"} 1
# TYPE kube_pod_status_phase gauge
kube_pod_status_phase{pod="api",phase="Pending"} 1
# TYPE kube_pod_status_ready gauge
kube_pod_status_ready{pod="api"} 1
`)
	run.sender.ResetCalls()
	run.run(t)
	run.sender.AssertMetric(t, "Gauge", "ksm.pod.ready", 1, "", []string{"pod:api", "node:node-b", "phase:Pending"})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "ksm.pod.ready", []string{"node:node-a"})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "ksm.pod.ready", []string{"phase:Running"})
}

func TestLatestLabelJoinsCompatibility(t *testing.T) {
	payload := `
# TYPE build_info gauge
build_info{pod="api",version="1.0.0"} 1
# TYPE app_up gauge
app_up{pod="api"} 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
label_joins:
  build_info:
    labels_to_match:
      - pod
    labels_to_get:
      - version
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"pod:api", "version:1.0.0", "endpoint:" + run.endpoint})
}

func TestLegacyEmptyNamespace(t *testing.T) {
	payload := `
# TYPE process_virtual_memory_bytes gauge
process_virtual_memory_bytes 12
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: ""
metrics:
  - process_virtual_memory_bytes: process.vm.bytes
`, payload)

	run.sender.AssertMetric(t, "Gauge", "process.vm.bytes", 12, "", nil)
	run.sender.AssertMetricMissing(t, "Gauge", "openmetrics.process.vm.bytes")
}

func TestShareLabelsCacheMatchesPythonOrdering(t *testing.T) {
	payload := `
# TYPE app_up gauge
app_up{pod="api"} 1
# TYPE build_info gauge
build_info{pod="api",version="1.0.0"} 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
share_labels:
  build_info:
    match:
      - pod
    labels:
      - version
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"pod:api", "endpoint:" + run.endpoint})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "test.app_up", []string{"version:1.0.0"})

	run.sender.ResetCalls()
	run.run(t)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"pod:api", "version:1.0.0", "endpoint:" + run.endpoint})
}

func TestTargetInfoCacheMatchesPythonOrdering(t *testing.T) {
	payload := `
# TYPE app_up gauge
app_up 1
# TYPE target info
target_info{cluster="prod",region="us-east-1"} 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
target_info: true
`, payload)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
	run.sender.AssertMetricNotTaggedWith(t, "Gauge", "test.app_up", []string{"cluster:prod"})

	run.sender.ResetCalls()
	run.run(t)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"cluster:prod", "region:us-east-1", "endpoint:" + run.endpoint})
}

func TestLatestMetadataTransformer(t *testing.T) {
	invChecks := inventorychecksimpl.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	t.Cleanup(check.ReleaseContext)

	payload := `
# TYPE kubernetes_build_info gauge
kubernetes_build_info{gitVersion="v1.6.0-alpha.0.680+3872cb93abf948-dirty"} 1
`
	run := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - kubernetes_build_info:
      name: version
      type: metadata
      label: gitVersion
`, payload)

	metadata := invChecks.GetInstanceMetadata(run.checkID)
	require.Equal(t, "1", metadata["version.major"])
	require.Equal(t, "6", metadata["version.minor"])
	require.Equal(t, "0", metadata["version.patch"])
	require.Equal(t, "alpha.0.680", metadata["version.release"])
	require.Equal(t, "3872cb93abf948-dirty", metadata["version.build"])
	require.Equal(t, "v1.6.0-alpha.0.680+3872cb93abf948-dirty", metadata["version.raw"])
	require.Equal(t, "semver", metadata["version.scheme"])
}

func TestLegacyMetadataTransformer(t *testing.T) {
	invChecks := inventorychecksimpl.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	t.Cleanup(check.ReleaseContext)

	payload := `
# TYPE build_info gauge
build_info{version="2.3.4",revision="abc123"} 1
`
	run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - build_info
metadata_metric_name: build_info
metadata_label_map:
  version.raw: version
  revision: revision
`, payload)

	metadata := invChecks.GetInstanceMetadata(run.checkID)
	require.Equal(t, "2.3.4", metadata["version.raw"])
	require.Equal(t, "abc123", metadata["revision"])
}

func TestAuthTokenFileHeader(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("secret-token\n"), 0o600))

	payload := `
# TYPE app_up gauge
app_up 1
`
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
auth_token:
  reader:
    type: file
    path: `+tokenPath+`
  writer:
    type: header
    name: Authorization
    value: Bearer <TOKEN>
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer secret-token", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	run.run(t)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestAuthTokenOAuthHeader(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.NoError(t, request.ParseForm())
		require.Equal(t, "client_credentials", request.Form.Get("grant_type"))
		require.Equal(t, "client-id", request.Form.Get("client_id"))
		require.Equal(t, "client-secret", request.Form.Get("client_secret"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"access_token":"oauth-token","expires_in":300}`))
		require.NoError(t, err)
	}))
	t.Cleanup(tokenServer.Close)

	payload := `
# TYPE app_up gauge
app_up 1
`
	run := configureOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - app_up
auth_token:
  reader:
    type: oauth
    url: `+tokenServer.URL+`
    client_id: client-id
    client_secret: client-secret
  writer:
    type: header
    name: Authorization
    value: Bearer <TOKEN>
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer oauth-token", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	run.run(t)

	run.sender.AssertMetric(t, "Gauge", "test.app_up", 1, "", []string{"endpoint:" + run.endpoint})
}

func TestLegacyBearerTokenAuthCompatibility(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("first-token\n"), 0o600))

	payload := `
# TYPE app_up gauge
app_up 1
`
	defaultTokenPath := filepath.Join(t.TempDir(), "default-token")
	require.NoError(t, os.WriteFile(defaultTokenPath, []byte("default-token\n"), 0o600))
	originalDefaultBearerTokenPath := defaultBearerTokenPath
	defaultBearerTokenPath = defaultTokenPath
	t.Cleanup(func() {
		defaultBearerTokenPath = originalDefaultBearerTokenPath
	})

	defaultTokenRun := configureOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: true
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer default-token", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	defaultTokenRun.run(t)
	defaultTokenRun.sender.AssertMetric(t, "Gauge", "openmetrics.app_up", 1, "", nil)

	var requestCount atomic.Int32
	run := configureOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: true
bearer_token_path: `+tokenPath+`
bearer_token_refresh_interval: 0
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			require.Equal(t, "Bearer first-token", request.Header.Get("Authorization"))
		} else {
			require.Equal(t, "Bearer second-token", request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	run.run(t)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.app_up", 1, "", nil)

	require.NoError(t, os.WriteFile(tokenPath, []byte("second-token\n"), 0o600))
	run.sender.ResetCalls()
	run.run(t)
	run.sender.AssertMetric(t, "Gauge", "openmetrics.app_up", 1, "", nil)

	disabled := configureOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: false
bearer_token_path: `+tokenPath+`
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	disabled.run(t)

	tlsOnlyHTTP := configureOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: tls_only
bearer_token_path: `+tokenPath+`
`, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	tlsOnlyHTTP.run(t)

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer second-token", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(payload))
		require.NoError(t, err)
	}))
	t.Cleanup(tlsServer.Close)

	tlsOnlyHTTPS := configureOpenMetricsCheckForEndpoint(t, `
prometheus_url: %%endpoint%%
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: tls_only
bearer_token_path: `+tokenPath+`
ssl_verify: false
`, tlsServer.URL)
	tlsOnlyHTTPS.run(t)
	tlsOnlyHTTPS.sender.AssertMetric(t, "Gauge", "openmetrics.app_up", 1, "", nil)
}

func TestLegacyBearerTokenMissingFileReportsCritical(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-token")
	run := configureOpenMetricsCheckWithoutServer(t, `
prometheus_url: http://127.0.0.1:1/metrics
namespace: openmetrics
metrics:
  - app_up
bearer_token_auth: true
bearer_token_path: `+missingPath+`
`)

	require.Error(t, run.check.Run())
	run.sender.Mock.AssertCalled(t, "ServiceCheck", "openmetrics.prometheus.health", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
}

func TestLegacyUseProcessStartTimeFlushFirstValue(t *testing.T) {
	originalStartTime := pkgconfigsetup.StartTime
	t.Cleanup(func() {
		pkgconfigsetup.StartTime = originalStartTime
	})

	tests := []struct {
		name                    string
		useProcessStartTime     bool
		sendDistributionBuckets bool
		agentStartTime          int64
		processStartTimes       []float64
		flushFirstValue         bool
	}{
		{name: "disabled", useProcessStartTime: false, agentStartTime: 10, flushFirstValue: false},
		{name: "enabled agent older", useProcessStartTime: true, agentStartTime: 10, processStartTimes: []float64{20}, flushFirstValue: true},
		{name: "enabled agent newer", useProcessStartTime: true, agentStartTime: 20, processStartTimes: []float64{10}, flushFirstValue: false},
		{name: "enabled no metric", useProcessStartTime: true, agentStartTime: 10, flushFirstValue: false},
		{name: "enabled many metrics all newer", useProcessStartTime: true, agentStartTime: 10, processStartTimes: []float64{20, 30, 40}, flushFirstValue: true},
		{name: "enabled many metrics some newer", useProcessStartTime: true, agentStartTime: 20, processStartTimes: []float64{10, 30, 40}, flushFirstValue: false},
		{name: "with buckets disabled", useProcessStartTime: false, sendDistributionBuckets: true, agentStartTime: 10, flushFirstValue: false},
		{name: "with buckets enabled agent older", useProcessStartTime: true, sendDistributionBuckets: true, agentStartTime: 10, processStartTimes: []float64{20}, flushFirstValue: true},
		{name: "with buckets enabled agent newer", useProcessStartTime: true, sendDistributionBuckets: true, agentStartTime: 20, processStartTimes: []float64{10}, flushFirstValue: false},
		{name: "with buckets enabled no metric", useProcessStartTime: true, sendDistributionBuckets: true, agentStartTime: 10, flushFirstValue: false},
		{name: "with buckets enabled many metrics all newer", useProcessStartTime: true, sendDistributionBuckets: true, agentStartTime: 10, processStartTimes: []float64{20, 30, 40}, flushFirstValue: true},
		{name: "with buckets enabled many metrics some newer", useProcessStartTime: true, sendDistributionBuckets: true, agentStartTime: 20, processStartTimes: []float64{10, 30, 40}, flushFirstValue: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pkgconfigsetup.StartTime = time.Unix(test.agentStartTime, 0)
			var processStartPayload strings.Builder
			for i, startedAt := range test.processStartTimes {
				labels := ""
				if len(test.processStartTimes) > 1 {
					labels = `{pid="` + strconv.Itoa(i) + `"}`
				}
				processStartPayload.WriteString("process_start_time_seconds")
				processStartPayload.WriteString(labels)
				processStartPayload.WriteString(" ")
				processStartPayload.WriteString(strconv.FormatFloat(startedAt, 'f', -1, 64))
				processStartPayload.WriteString("\n")
			}
			payload := `
# TYPE go_memstats_alloc_bytes_total counter
go_memstats_alloc_bytes_total 9
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 2
request_duration_seconds_bucket{le="+Inf"} 2
request_duration_seconds_sum 0.2
request_duration_seconds_count 2
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0.5"} 0.01
go_gc_duration_seconds_sum 0.02
go_gc_duration_seconds_count 1
# TYPE process_start_time_seconds gauge
` + processStartPayload.String()

			run := runOpenMetricsCheck(t, `
prometheus_url: %%endpoint%%
namespace: ""
metrics:
  - go_memstats_alloc_bytes_total
  - request_duration_seconds
  - go_gc_duration_seconds
send_distribution_counts_as_monotonic: true
send_distribution_buckets: `+strconv.FormatBool(test.sendDistributionBuckets)+`
use_process_start_time: `+strconv.FormatBool(test.useProcessStartTime)+`
`, payload)

			run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "go_memstats_alloc_bytes_total", 9, "", nil, test.flushFirstValue)
			run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "go_gc_duration_seconds.count", 1, "", nil, test.flushFirstValue)
			if test.sendDistributionBuckets {
				run.sender.Mock.AssertCalled(t, "HistogramBucket", "request_duration_seconds", int64(2), 0.0, 0.1, true, "", mocksender.MatchTagsContains([]string{"lower_bound:0", "upper_bound:0.1"}), test.flushFirstValue)
			} else {
				run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "request_duration_seconds.count", 2, "", []string{"upper_bound:none"}, test.flushFirstValue)
			}

			run.sender.ResetCalls()
			run.run(t)
			run.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "go_memstats_alloc_bytes_total", 9, "", nil, true)
		})
	}
}

func TestLatestPropertyBasedInvariantEquivalents(t *testing.T) {
	gaugePayload := `
# TYPE alpha gauge
alpha 1
# TYPE beta_value gauge
beta_value 2
# TYPE gamma_total gauge
gamma_total 3
`
	allGauges := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - ".+"
`, gaugePayload)
	allGauges.sender.AssertMetric(t, "Gauge", "test.alpha", 1, "", []string{"endpoint:" + allGauges.endpoint})
	allGauges.sender.AssertMetric(t, "Gauge", "test.beta_value", 2, "", []string{"endpoint:" + allGauges.endpoint})
	allGauges.sender.AssertMetric(t, "Gauge", "test.gamma_total", 3, "", []string{"endpoint:" + allGauges.endpoint})

	subsetGauges := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - alpha
  - beta_value
`, gaugePayload)
	subsetGauges.sender.AssertMetric(t, "Gauge", "test.alpha", 1, "", []string{"endpoint:" + subsetGauges.endpoint})
	subsetGauges.sender.AssertMetric(t, "Gauge", "test.beta_value", 2, "", []string{"endpoint:" + subsetGauges.endpoint})
	subsetGauges.sender.AssertMetricMissing(t, "Gauge", "test.gamma_total")

	emptyMapping := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics: []
`, gaugePayload)
	emptyMapping.sender.AssertMetricMissing(t, "Gauge", "test.alpha")
	emptyMapping.sender.AssertMetricMissing(t, "Gauge", "test.beta_value")
	emptyMapping.sender.AssertMetricMissing(t, "Gauge", "test.gamma_total")

	prometheusCounters := runOpenMetricsCheck(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - ".+"
`, `
# TYPE requests counter
requests 10
# TYPE jobs_processed counter
jobs_processed 11
`)
	prometheusCounters.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 10, "", []string{"endpoint:" + prometheusCounters.endpoint}, false)
	prometheusCounters.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.jobs_processed.count", 11, "", []string{"endpoint:" + prometheusCounters.endpoint}, false)

	openMetricsCounters := runOpenMetricsCheckWithResponse(t, `
openmetrics_endpoint: %%endpoint%%
namespace: test
metrics:
  - requests
  - jobs_processed
`, `# TYPE requests counter
requests_total 10
# TYPE jobs_processed counter
jobs_processed_total 11
# EOF
`, http.StatusOK, "application/openmetrics-text")
	openMetricsCounters.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.requests.count", 10, "", []string{"endpoint:" + openMetricsCounters.endpoint}, false)
	openMetricsCounters.sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test.jobs_processed.count", 11, "", []string{"endpoint:" + openMetricsCounters.endpoint}, false)
}
