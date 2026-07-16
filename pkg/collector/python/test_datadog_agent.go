// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/version"
)

import "C"

func testGetVersion(t *testing.T) {
	var v *C.char
	GetVersion(&v)
	require.NotNil(t, v)

	av, _ := version.Agent()
	assert.Equal(t, av.GetNumber(), C.GoString(v))
}

func testGetHostname(t *testing.T) {
	var h *C.char
	GetHostname(&h)
	require.NotNil(t, h)

	hname, _ := hostname.Get(context.Background())
	assert.Equal(t, hname, C.GoString(h))
}

func testGetClusterName(t *testing.T) {
	var ch *C.char
	GetClusterName(&ch)
	require.NotNil(t, ch)

	assert.Equal(t, clustername.GetClusterName(context.Background(), ""), C.GoString(ch))
}

func testHeaders(t *testing.T) {
	var headers *C.char
	Headers(&headers)
	require.NotNil(t, headers)

	h := httpHeaders()
	jsonPayload, _ := json.Marshal(h)
	assert.Equal(t, string(jsonPayload), C.GoString(headers))
}

func testGetConfig(t *testing.T) {
	var config *C.char

	GetConfig(C.CString("does not exist"), &config)
	require.Nil(t, config)

	GetConfig(C.CString("cmd_port"), &config)
	require.NotNil(t, config)
	assert.Equal(t, "5001", C.GoString(config))
}

func testSetExternalTags(t *testing.T) {
	ctags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}

	SetExternalTags(C.CString("test_hostname"), C.CString("test_source_type"), &ctags[0])

	payload := externalhost.GetPayload()
	require.NotNil(t, payload)

	yamlPayload, _ := yaml.Marshal(payload)
	assert.Equal(t,
		"- - test_hostname\n  - test_source_type:\n    - tag1\n    - tag2\n",
		string(yamlPayload))
}

func testEmitAgentTelemetry(t *testing.T) {
	resetAgentTelemetryForTest()

	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))

	// Test second time for laziness check
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))

	// Test for lock problems
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			time.Sleep(time.Millisecond * time.Duration(rand.IntN(10)))
			EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))
			wg.Done()
		}()
	}
	wg.Wait()

	// Test that changing the metric type doesn't crash the agent for all the permutations
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("histogram"))

	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("gauge"))

	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("gauge"))

	mf := requireTelemetryFamily(t, "test_check__test_metric")
	require.Len(t, mf.Metric, 1)
	assert.Empty(t, mf.Metric[0].GetLabel())
	assert.Equal(t, 1.0, mf.Metric[0].GetGauge().GetValue())
}

func resetAgentTelemetryForTest() {
	telemetryLock.Lock()
	telemetryMap = map[agentTelemetryMetricKey]*agentTelemetryMetric{}
	telemetryLock.Unlock()
	telemetryimpl.GetCompatComponent().Reset()
}

func callEmitAgentTelemetryWithLabels(checkName string, metricName string, metricValue float64, metricType string, labelsJSON string) string {
	var errOut *C.char
	EmitAgentTelemetryWithLabels(C.CString(checkName), C.CString(metricName), C.double(metricValue), C.CString(metricType), C.CString(labelsJSON), &errOut)
	if errOut == nil {
		return ""
	}
	return C.GoString(errOut)
}

func requireTelemetryFamily(t *testing.T, name string) *dto.MetricFamily {
	t.Helper()
	families, err := telemetryimpl.GetCompatComponent().Gather(false)
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family %s not found", name)
	return nil
}

func metricLabels(metricIndex int, family *dto.MetricFamily) map[string]string {
	labels := map[string]string{}
	for _, label := range family.Metric[metricIndex].GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func testAgentTelemetryMetricKeyAvoidsDelimitedCollisions(t *testing.T) {
	resetAgentTelemetryForTest()

	leftKey := newAgentTelemetryMetricKey("a.b", "c")
	rightKey := newAgentTelemetryMetricKey("a", "b.c")
	telemetryMap[leftKey] = &agentTelemetryMetric{
		metric:     "left-value",
		metricType: "counter",
		labelNames: []string{"check_name"},
	}
	telemetryMap[rightKey] = &agentTelemetryMetric{
		metric:     "right-value",
		metricType: "gauge",
		labelNames: []string{"state"},
	}

	require.Len(t, telemetryMap, 2)
	assert.Equal(t, "counter", telemetryMap[leftKey].metricType)
	assert.Equal(t, []string{"check_name"}, telemetryMap[leftKey].labelNames)
	assert.Equal(t, "left-value", telemetryMap[leftKey].metric)
	assert.Equal(t, "gauge", telemetryMap[rightKey].metricType)
	assert.Equal(t, []string{"state"}, telemetryMap[rightKey].labelNames)
	assert.Equal(t, "right-value", telemetryMap[rightKey].metric)
}

func testEmitAgentTelemetryWithLabels(t *testing.T) {
	resetAgentTelemetryForTest()

	assert.Empty(t, callEmitAgentTelemetryWithLabels("test_check", "test_counter", 1, "counter", `{"check_name":"openmetrics","state":"limited"}`))
	assert.Empty(t, callEmitAgentTelemetryWithLabels("test_check", "test_counter", 2, "counter", `{"state":"limited","check_name":"openmetrics"}`))
	counterFamily := requireTelemetryFamily(t, "test_check__test_counter")
	require.Len(t, counterFamily.Metric, 1)
	assert.Equal(t, map[string]string{"check_name": "openmetrics", "state": "limited"}, metricLabels(0, counterFamily))
	assert.Equal(t, 3.0, counterFamily.Metric[0].GetCounter().GetValue())

	assert.Empty(t, callEmitAgentTelemetryWithLabels("test_check", "test_gauge", 7, "gauge", `{"check_name":"openmetrics"}`))
	gaugeFamily := requireTelemetryFamily(t, "test_check__test_gauge")
	require.Len(t, gaugeFamily.Metric, 1)
	assert.Equal(t, map[string]string{"check_name": "openmetrics"}, metricLabels(0, gaugeFamily))
	assert.Equal(t, 7.0, gaugeFamily.Metric[0].GetGauge().GetValue())

	assert.Empty(t, callEmitAgentTelemetryWithLabels("test_check", "test_histogram", 25, "histogram", `{"check_name":"openmetrics"}`))
	histogramFamily := requireTelemetryFamily(t, "test_check__test_histogram")
	require.Len(t, histogramFamily.Metric, 1)
	assert.Equal(t, map[string]string{"check_name": "openmetrics"}, metricLabels(0, histogramFamily))
	assert.Equal(t, uint64(1), histogramFamily.Metric[0].GetHistogram().GetSampleCount())

	assert.Contains(t, callEmitAgentTelemetryWithLabels("test_check", "bad_json", 1, "counter", `{bad`), "invalid labels JSON")
	assert.Contains(t, callEmitAgentTelemetryWithLabels("test_check", "bad_labels", 1, "counter", `{"check_name":1}`), "invalid labels JSON")
	assert.Contains(t, callEmitAgentTelemetryWithLabels("test_check", "test_counter", 1, "counter", `{"check_name":"openmetrics"}`), "already emitted with labels")
	assert.Contains(t, callEmitAgentTelemetryWithLabels("test_check", "test_counter", 1, "gauge", `{"state":"limited","check_name":"openmetrics"}`), "already emitted as counter")
	assert.Contains(t, callEmitAgentTelemetryWithLabels("test_check", "invalid_type", 1, "rate", `{"check_name":"openmetrics"}`), "unsupported metric type")
}

func testObfuscaterConfig(t *testing.T) {
	pkgconfigmodel.CleanOverride(t)
	_ = pkgconfigmock.New(t)
	o := lazyInitObfuscator()
	o.Stop()
	expected := obfuscate.Config{
		ES: obfuscate.JSONConfig{
			Enabled:            true,
			KeepValues:         []string{},
			ObfuscateSQLValues: []string{},
		},
		OpenSearch: obfuscate.JSONConfig{
			Enabled:            true,
			KeepValues:         []string{},
			ObfuscateSQLValues: []string{},
		},
		Mongo:                defaultMongoObfuscateSettings,
		SQLExecPlan:          defaultSQLPlanObfuscateSettings,
		SQLExecPlanNormalize: defaultSQLPlanNormalizeSettings,
		HTTP: obfuscate.HTTPConfig{
			RemoveQueryString: false,
			RemovePathDigits:  false,
		},
		Redis: obfuscate.RedisConfig{
			Enabled:       true,
			RemoveAllArgs: false,
		},
		Valkey: obfuscate.ValkeyConfig{
			Enabled:       true,
			RemoveAllArgs: false,
		},
		Memcached: obfuscate.MemcachedConfig{
			Enabled:     true,
			KeepCommand: false,
		},
		CreditCard: obfuscate.CreditCardsConfig{
			Enabled:    true,
			Luhn:       false,
			KeepValues: []string{},
		},
		Cache: obfuscate.CacheConfig{
			Enabled: true,
			MaxSize: 5000000,
		},
	}
	assert.Equal(t, expected, obfuscaterConfig)
}

func testReportIssue(t *testing.T) {
	hp := healthplatformmock.New(t)
	SetHealthPlatform(hp)
	t.Cleanup(func() { SetHealthPlatform(nil) })

	t.Run("partial_json_id_only", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("integration:mysql"), C.CString(`{"id":"partial-minimal"}`), &errOut)
		require.Nil(t, errOut, reportIssueErrMsg(errOut))
		got := hp.GetIssue("partial-minimal")
		require.NotNil(t, got)
		assert.Equal(t, "partial-minimal", got.Id)
		assert.Empty(t, got.IssueName, "optional proto fields omitted in JSON should unmarshal as empty")
		assert.Equal(t, "integration:mysql", got.Source, "ReportIssue sets Source from check name")
	})

	t.Run("partial_json_subset_of_fields", func(t *testing.T) {
		var errOut *C.char
		payload := `{"id":"partial-rich","issueName":"conn-timeout","title":"DB timeout","severity":"ISSUE_SEVERITY_MEDIUM"}`
		ReportIssue(C.CString("py:check"), C.CString(payload), &errOut)
		require.Nil(t, errOut, reportIssueErrMsg(errOut))
		got := hp.GetIssue("partial-rich")
		require.NotNil(t, got)
		assert.Equal(t, "partial-rich", got.Id)
		assert.Equal(t, "conn-timeout", got.IssueName)
		assert.Equal(t, "DB timeout", got.Title)
		assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM, got.Severity)
		assert.Equal(t, "py:check", got.Source)
		assert.Empty(t, got.Description)
	})

	t.Run("missing_id_rejected", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("c"), C.CString(`{"issueName":"orphan-name"}`), &errOut)
		require.NotNil(t, errOut)
		assert.Contains(t, C.GoString(errOut), "empty or null id")
	})

	t.Run("invalid_json_rejected", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("c"), C.CString(`{`), &errOut)
		require.NotNil(t, errOut)
		assert.NotEmpty(t, C.GoString(errOut))
	})
}

func reportIssueErrMsg(errOut *C.char) string {
	if errOut == nil {
		return ""
	}
	return C.GoString(errOut)
}
