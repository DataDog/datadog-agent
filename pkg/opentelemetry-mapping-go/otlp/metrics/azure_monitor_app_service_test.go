// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

const azureMonitorTestTimestamp pcommon.Timestamp = 1_700_000_000_000_000_000

func azureMonitorAppServiceTranslators(t *testing.T, options ...TranslatorOption) map[string]Provider {
	t.Helper()
	settings := componenttest.NewNopTelemetrySettings()
	at, err := attributes.NewTranslator(settings)
	require.NoError(t, err)
	options = append(options, WithFallbackSourceProvider(testProvider("collector-host")))
	full, err := NewDefaultTranslator(settings, at, options...)
	require.NoError(t, err)
	minimal, err := NewMinimalTranslator(settings.Logger, at, options...)
	require.NoError(t, err)
	return map[string]Provider{"full": full, "minimal": minimal}
}

// The receiver shares a resource across apps, putting identity on each point.
// Values here are synthetic; the JSON replay test separately covers a capture.
func azureMonitorAppServiceTestMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("azuremonitor.tenant_id", "")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(azureMonitorScope)
	sm.Scope().SetVersion("0.162.0")
	m := sm.Metrics().AppendEmpty()
	m.SetName("azure_cputime_total")
	m.SetUnit("s")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetDoubleValue(0.25)
	dp.SetTimestamp(azureMonitorTestTimestamp)
	setAzureMonitorTestIdentity(dp, "Sub-1", "Example-RG", "Example-App", "WorkerA")
	dp.Attributes().PutStr("location", "eastus2")
	dp.Attributes().PutStr("timegrain", "PT1M")
	return md
}

func setAzureMonitorTestIdentity(dp pmetric.NumberDataPoint, subscription, group, name, instance string) {
	dp.Attributes().PutStr("azuremonitor.resource_id", "/subscriptions/"+subscription+"/resourceGroups/"+group+"/providers/Microsoft.Web/sites/"+name)
	dp.Attributes().PutStr("resource_group", strings.ToLower(group))
	dp.Attributes().PutStr("name", name)
	dp.Attributes().PutStr("type", "Microsoft.Web/sites")
	dp.Attributes().PutStr("metadata_Instance", instance)
}

func azureMonitorProofPoints(consumer testConsumer) []TestTimeSeries {
	var points []TestTimeSeries
	for _, point := range consumer.data.Metrics.TimeSeries {
		if point.Name == azureMonitorAppServiceActiveInstance {
			points = append(points, point)
		}
	}
	return points
}

// TestAzureMonitorAppServiceReceiverReplay uses a sanitized file-exporter capture
// from azuremonitorreceiver v0.162.0: two apps sharing one mixed-case Instance.
// For a local replay, AZURE_MONITOR_REPLAY_PATH may point to a single OTLP JSON
// export request with the same shape. Output is logged, never submitted remotely.
func TestAzureMonitorAppServiceReceiverReplay(t *testing.T) {
	path := os.Getenv("AZURE_MONITOR_REPLAY_PATH")
	if path == "" {
		path = "test/azure_monitor_app_service_receiver.json"
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for name, translator := range azureMonitorAppServiceTranslators(t, WithAzureMonitorAppServiceMetrics()) {
		t.Run(name, func(t *testing.T) {
			var unmarshaler pmetric.JSONUnmarshaler
			md, err := unmarshaler.UnmarshalMetrics(data)
			require.NoError(t, err)
			rm := md.ResourceMetrics().At(0)
			subscription, ok := rm.Resource().Attributes().Get("azuremonitor.subscription_id")
			require.True(t, ok)
			var expected []TestTimeSeries
			for _, m := range rm.ScopeMetrics().At(0).Metrics().All() {
				require.Equal(t, "azure_cputime_total", m.Name())
				for _, dp := range m.Gauge().DataPoints().All() {
					require.Greater(t, dp.DoubleValue(), 0.0)
					tags := []string{"subscription_id:" + strings.ToLower(subscription.Str())}
					for _, pair := range [][2]string{{"resource_group", "resource_group"}, {"name", "name"}, {"instance", "metadata_Instance"}} {
						value, ok := dp.Attributes().Get(pair[1])
						require.True(t, ok)
						tags = append(tags, pair[0]+":"+strings.ToLower(value.Str()))
					}
					expected = append(expected, TestTimeSeries{
						TestDimensions: TestDimensions{
							Name: azureMonitorAppServiceActiveInstance, Tags: tags,
							OriginSubProduct: OriginSubProductOTLP, OriginProductDetail: OriginProductDetailAzureMonitorReceiver,
						},
						Type: Gauge, Timestamp: uint64(dp.Timestamp()), Value: 1,
					})
				}
			}
			require.Len(t, expected, 2)
			consumer := newTestConsumer()
			_, err = translator.MapMetrics(t.Context(), md, &consumer, nil)
			require.NoError(t, err)
			points := azureMonitorProofPoints(consumer)
			require.Equal(t, expected, points)
			assert.NotEqual(t, points[0].Tags, points[1].Tags, "apps sharing a worker remain distinct")
			output, err := json.Marshal(points)
			require.NoError(t, err)
			t.Logf("receiver replay output: %s", output)
		})
	}
}

func TestAzureMonitorAppServiceMapMetrics(t *testing.T) {
	for name, translator := range azureMonitorAppServiceTranslators(t, WithAzureMonitorAppServiceMetrics()) {
		t.Run(name, func(t *testing.T) {
			md := azureMonitorAppServiceTestMetrics()
			// Resource metadata belongs to the Collector, not the remote app.
			md.ResourceMetrics().At(0).Resource().Attributes().PutStr("service.name", "collector")
			md.ResourceMetrics().At(0).Resource().Attributes().PutStr("azuremonitor.subscription_id", "not-the-app-subscription")
			consumer := newTestConsumer()
			_, err := translator.MapMetrics(t.Context(), md, &consumer, nil)
			require.NoError(t, err)
			points := azureMonitorProofPoints(consumer)
			require.Len(t, points, 1)
			assert.Equal(t, TestTimeSeries{
				TestDimensions: TestDimensions{
					Name:                azureMonitorAppServiceActiveInstance,
					Tags:                []string{"subscription_id:sub-1", "resource_group:example-rg", "name:example-app", "instance:workera"},
					OriginSubProduct:    OriginSubProductOTLP,
					OriginProductDetail: OriginProductDetailAzureMonitorReceiver,
				},
				Type: Gauge, Timestamp: uint64(azureMonitorTestTimestamp), Value: 1,
			}, points[0])
			// Raw metric forwarding still has its original value and fallback host.
			require.Len(t, consumer.data.Metrics.TimeSeries, 2)
			raw := consumer.data.Metrics.TimeSeries[0]
			assert.Equal(t, "azure_cputime_total", raw.Name)
			assert.Equal(t, 0.25, raw.Value)
			assert.Equal(t, "collector-host", raw.Host)
			assert.Equal(t, uint64(azureMonitorTestTimestamp), raw.Timestamp)
			assert.Equal(t, map[string]struct{}{"collector-host": {}}, consumer.data.Hosts)
		})
	}
}

func TestAzureMonitorAppServiceFiltering(t *testing.T) {
	tests := []struct {
		name   string
		modify func(pmetric.ScopeMetrics, pmetric.Metric, pmetric.NumberDataPoint)
	}{
		{"zero", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) { dp.SetDoubleValue(0) }},
		{"negative", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) { dp.SetIntValue(-1) }},
		{"nan", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) {
			dp.SetDoubleValue(math.NaN())
		}},
		{"infinity", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) {
			dp.SetDoubleValue(math.Inf(1))
		}},
		{"no recorded value", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) {
			dp.SetFlags(dp.Flags().WithNoRecordedValue(true))
		}},
		{"no timestamp", func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) { dp.SetTimestamp(0) }},
		{"wrong scope", func(sm pmetric.ScopeMetrics, _ pmetric.Metric, _ pmetric.NumberDataPoint) {
			sm.Scope().SetName("example/azuremonitorreceiver")
		}},
		{"count aggregation", func(_ pmetric.ScopeMetrics, m pmetric.Metric, _ pmetric.NumberDataPoint) {
			m.SetName("azure_cputime_count")
		}},
		{"average aggregation", func(_ pmetric.ScopeMetrics, m pmetric.Metric, _ pmetric.NumberDataPoint) {
			m.SetName("azure_cputime_average")
		}},
		{"functions", func(_ pmetric.ScopeMetrics, m pmetric.Metric, _ pmetric.NumberDataPoint) {
			m.SetName("azure_functionexecutioncount_total")
		}},
		{"sum", func(_ pmetric.ScopeMetrics, m pmetric.Metric, dp pmetric.NumberDataPoint) {
			copy := pmetric.NewNumberDataPoint()
			dp.CopyTo(copy)
			sum := m.SetEmptySum()
			sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
			copy.CopyTo(sum.DataPoints().AppendEmpty())
		}},
	}
	for _, key := range []string{"azuremonitor.resource_id", "resource_group", "name", "type", "metadata_Instance"} {
		tests = append(tests, struct {
			name   string
			modify func(pmetric.ScopeMetrics, pmetric.Metric, pmetric.NumberDataPoint)
		}{"missing " + key, func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) {
			dp.Attributes().Remove(key)
		}})
	}
	for _, test := range []struct{ name, key, value string }{
		{"wrong type", "type", "Microsoft.Web/serverFarms"},
		{"conflicting name", "name", "other-app"},
		{"conflicting group", "resource_group", "other-group"},
		{"empty instance", "metadata_Instance", ""},
		{"whitespace instance", "metadata_Instance", " "},
		{"blank subscription", "azuremonitor.resource_id", "/subscriptions/ /resourceGroups/example-rg/providers/Microsoft.Web/sites/example-app"},
		{"malformed id", "azuremonitor.resource_id", "/subscriptions/sub-1/resourceGroups/example-rg/providers/Microsoft.Web/sites/"},
		{"slot", "azuremonitor.resource_id", "/subscriptions/sub-1/resourceGroups/example-rg/providers/Microsoft.Web/sites/example-app/slots/staging"},
	} {
		tests = append(tests, struct {
			name   string
			modify func(pmetric.ScopeMetrics, pmetric.Metric, pmetric.NumberDataPoint)
		}{test.name, func(_ pmetric.ScopeMetrics, _ pmetric.Metric, dp pmetric.NumberDataPoint) {
			dp.Attributes().PutStr(test.key, test.value)
		}})
	}
	for name, translator := range azureMonitorAppServiceTranslators(t, WithAzureMonitorAppServiceMetrics()) {
		for _, test := range tests {
			t.Run(name+"/"+test.name, func(t *testing.T) {
				md := azureMonitorAppServiceTestMetrics()
				sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
				m := sm.Metrics().At(0)
				test.modify(sm, m, m.Gauge().DataPoints().At(0))
				consumer := newTestConsumer()
				_, err := translator.MapMetrics(t.Context(), md, &consumer, nil)
				require.NoError(t, err)
				assert.Empty(t, azureMonitorProofPoints(consumer))
			})
		}
	}
}

func TestAzureMonitorAppServiceObservationDedupe(t *testing.T) {
	for name, translator := range azureMonitorAppServiceTranslators(t, WithAzureMonitorAppServiceMetrics()) {
		t.Run(name, func(t *testing.T) {
			md := azureMonitorAppServiceTestMetrics()
			rm := md.ResourceMetrics().At(0)
			dps := rm.ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints()
			dp := dps.At(0)
			duplicate := dps.AppendEmpty()
			dp.CopyTo(duplicate)
			setAzureMonitorTestIdentity(duplicate, "sub-1", "example-rg", "example-app", "workera")
			duplicate.SetIntValue(9)
			later := dps.AppendEmpty()
			dp.CopyTo(later)
			later.SetTimestamp(azureMonitorTestTimestamp + 60_000_000_000)
			for _, identity := range [][4]string{
				{"Sub-1", "Example-RG", "Example-App", "WorkerB"},
				{"Sub-1", "Example-RG", "Other-App", "WorkerA"},
				{"Sub-2", "Example-RG", "Example-App", "WorkerA"},
				// Distinct tuples even if underscore-joined keys would collide.
				{"Sub-1", "a_b", "c", "WorkerA"},
				{"Sub-1", "a", "b_c", "WorkerA"},
			} {
				point := dps.AppendEmpty()
				dp.CopyTo(point)
				setAzureMonitorTestIdentity(point, identity[0], identity[1], identity[2], identity[3])
			}
			// Dedupe spans resource/scope boundaries, not just one metric slice.
			rm.CopyTo(md.ResourceMetrics().AppendEmpty())
			for range 2 {
				consumer := newTestConsumer()
				_, err := translator.MapMetrics(t.Context(), md, &consumer, nil)
				require.NoError(t, err)
				points := azureMonitorProofPoints(consumer)
				require.Len(t, points, 7)
				assert.Equal(t, uint64(azureMonitorTestTimestamp), points[0].Timestamp)
				assert.Equal(t, uint64(later.Timestamp()), points[1].Timestamp)
				assert.Equal(t, points[0].Tags, points[1].Tags)
				assert.Contains(t, points[2].Tags, "instance:workerb")
				assert.Contains(t, points[3].Tags, "name:other-app")
				assert.Contains(t, points[4].Tags, "subscription_id:sub-2")
				assert.NotEqual(t, points[5].Tags, points[6].Tags)
			}
		})
	}
}

func TestAzureMonitorAppServiceDisabledByDefault(t *testing.T) {
	for name, translator := range azureMonitorAppServiceTranslators(t) {
		t.Run(name, func(t *testing.T) {
			consumer := &mockAzureFunctionsConsumer{}
			_, err := translator.MapMetrics(t.Context(), azureMonitorAppServiceTestMetrics(), consumer, nil)
			require.NoError(t, err)
			require.Len(t, consumer.metrics, 1)
			assert.Equal(t, "azure_cputime_total", consumer.metrics[0].name)
			assert.Equal(t, 0.25, consumer.metrics[0].value)
			assert.Equal(t, "collector-host", consumer.metrics[0].host)
			assert.Equal(t, []string{"collector-host"}, consumer.hosts)
			assert.Empty(t, consumer.tagSetCalls)
		})
	}
}

func TestAzureMonitorAppServiceAppSideSeriesUnchanged(t *testing.T) {
	for name, translator := range azureMonitorAppServiceTranslators(t, WithAzureMonitorAppServiceMetrics()) {
		t.Run(name, func(t *testing.T) {
			md := azureMonitorAppServiceTestMetrics()
			app := azureFunctionsMetrics(map[string]string{
				"cloud.platform":            "azure.app_service",
				"cloud.account.id":          "sub-1",
				"azure.resource_group.name": "example-rg",
				"service.name":              "example-app",
				"service.instance.id":       "not-a-worker-name",
			})
			app.ResourceMetrics().At(0).CopyTo(md.ResourceMetrics().AppendEmpty())
			consumer := &mockAzureFunctionsConsumer{}
			_, err := translator.MapMetrics(t.Context(), md, consumer, nil)
			require.NoError(t, err)
			require.Len(t, consumer.tagSetCalls, 1)
			assert.Equal(t, "azureappservices", consumer.tagSetCalls[0].metricSuffix)
			assert.ElementsMatch(t, []string{"subscription_id:sub-1", "resource_group:example-rg", "name:example-app"}, consumer.tagSetCalls[0].tags)
			require.Len(t, consumer.metrics, 3)
			assert.Equal(t, azureMonitorAppServiceActiveInstance, consumer.metrics[2].name)
			assert.Contains(t, consumer.metrics[2].tags, "instance:workera")
		})
	}
}
