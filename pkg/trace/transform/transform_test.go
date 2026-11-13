// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"encoding/base64"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// TestCopyOTelAttributeTags checks that NewOTelSpanTagKeys + copyOTelAttributeTags
// matches a per-key lookup across the span and resource attribute maps.
func TestCopyOTelAttributeTags(t *testing.T) {
	sattr := pcommon.NewMap()
	require.NoError(t, sattr.FromRaw(map[string]any{
		"both": "span", "span-only": "span", "empty-span": "",
		"int": int64(13), "bool": true, "unrequested": "span",
	}))
	rattr := pcommon.NewMap()
	require.NoError(t, rattr.FromRaw(map[string]any{
		"both": "resource", "resource-only": "resource", "empty-span": "resource",
		"empty-resource": "", "unrequested": "resource",
	}))
	keys := []string{"both", "span-only", "resource-only", "empty-span", "empty-resource", "int", "bool", "missing"}

	want := map[string]string{}
	for _, key := range keys {
		if val := GetOTelAttrFromEitherMap(sattr, rattr, false, key); val != "" {
			want[key] = val
		}
	}
	require.Equal(t, map[string]string{
		"both": "span", "span-only": "span", "resource-only": "resource",
		"empty-span": "resource", "int": "13", "bool": "true",
	}, want)

	peer := map[string]string{}
	copyOTelAttributeTags(sattr, rattr, NewOTelSpanTagKeys(keys, nil).peer, peer)
	assert.Equal(t, want, peer)

	primary := map[string]string{}
	copyOTelAttributeTags(sattr, rattr, NewOTelSpanTagKeys(nil, keys).primary, primary)
	assert.Equal(t, want, primary)
}

func TestOtelSpanToDDSpanStructuredBytes(t *testing.T) {
	// {"value": [1, "two"]}, as emitted by the Datadog meta_struct encoder.
	report := []byte{0x81, 0xa5, 'v', 'a', 'l', 'u', 'e', 0x92, 0x01, 0xa3, 't', 'w', 'o'}
	for _, tt := range []struct {
		name       string
		key        string
		sdk        string
		data       []byte
		structured bool
	}{
		{name: "appsec", key: "appsec", sdk: "datadog", data: report, structured: true},
		{name: "iast", key: "iast", sdk: "datadog", data: report, structured: true},
		{name: "stack", key: "_dd.stack", data: report, structured: true},
		{name: "shadow deployment", key: "_dd.appsec_shadow_deployed", data: report, structured: true},
		{name: "threat intelligence", key: "threat_intel", data: report, structured: true},
		{name: "API security", key: "api.security", data: report, structured: true},
		{name: "API entity", key: "api.entity", data: report, structured: true},
		{name: "API definition", key: "api.definition", data: report, structured: true},
		{name: "API endpoint", key: "api.endpoint", data: report, structured: true},
		{name: "request body", key: "http.request.body", data: report, structured: true},
		{name: "AI Guard", key: "ai_guard", data: report, structured: true},
		{name: "LLM Observability", key: "_llmobs", data: report, structured: true},
		{name: "dotted descendant", key: "appsec.triggers", data: report, structured: true},
		{name: "unknown report", key: "custom.report", sdk: "datadog", data: report},
		{name: "prefix collision", key: "appsec_other", data: report},
		{name: "custom events", key: "appsec.events", data: report},
		{name: "nested custom event", key: "appsec.events.users.login", data: report},
		{name: "fingerprint", key: "appsec.fingerprint", data: report},
		{name: "nested fingerprint", key: "appsec.fingerprint.http", data: report},
		{name: "empty map", key: "appsec", sdk: "datadog", data: []byte{0x80}, structured: true},
		{name: "upstream SDK", key: "appsec", sdk: "opentelemetry", data: report, structured: true},
		{name: "no SDK identity", key: "appsec", data: report, structured: true},
		{name: "raw binary", key: "appsec", sdk: "datadog", data: []byte{0xde, 0xad}},
		{name: "empty bytes", key: "appsec", sdk: "datadog", data: []byte{}},
		{name: "scalar", key: "appsec", sdk: "datadog", data: []byte{0x01}},
		{name: "truncated map", key: "appsec", sdk: "datadog", data: []byte{0x81}},
		{name: "trailing bytes", key: "appsec", sdk: "datadog", data: append(append([]byte{}, report...), 0x01)},
		{name: "dedicated field", key: "resource.name", sdk: "datadog", data: report},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.Attributes().PutEmptyBytes(tt.key).FromRaw(tt.data)
			res := pcommon.NewResource()
			if tt.sdk != "" {
				res.Attributes().PutStr("telemetry.sdk.name", tt.sdk)
			}
			if tt.structured {
				res.Attributes().PutStr(tt.key, "resource fallback")
			}
			cfg := &config.AgentConfig{Features: map[string]struct{}{}, OTLPReceiver: &config.OTLP{}}
			cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

			ddspan := OtelSpanToDDSpan(span, res, pcommon.NewInstrumentationScope(), cfg)

			if tt.structured {
				require.Equal(t, tt.data, ddspan.MetaStruct[tt.key])
				assert.NotContains(t, ddspan.Meta, tt.key)
				value, _ := span.Attributes().Get(tt.key)
				value.Bytes().AsRaw()[0] = 0
				assert.Equal(t, tt.data, ddspan.MetaStruct[tt.key], "the converted span must own its bytes")
			} else {
				assert.Empty(t, ddspan.MetaStruct)
				if tt.key != "resource.name" {
					assert.Equal(t, base64.StdEncoding.EncodeToString(tt.data), ddspan.Meta[tt.key])
				}
			}
		})
	}
}

func TestGetOTelEnv(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
	}{
		{
			name:     "neither set",
			expected: "",
		},
		{
			name:     "only in resource (semconv127)",
			rattrs:   map[string]string{string(semconv127.DeploymentEnvironmentNameKey): "env-res-127"},
			expected: "env-res-127",
		},
		{
			name:     "only in resource (semconv117)",
			rattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-res"},
			expected: "env-res",
		},
		{
			name:     "only in span (semconv127)",
			sattrs:   map[string]string{string(semconv127.DeploymentEnvironmentNameKey): "env-span-127"},
			expected: "env-span-127",
		},
		{
			name:     "only in span (semconv117)",
			sattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-span"},
			expected: "env-span",
		},
		{
			name:     "both set (span wins)",
			sattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-span"},
			rattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-res"},
			expected: "env-span",
		},
		{
			name:     "normalization",
			sattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "  ENV "},
			expected: "_env",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetOTelEnv(span, res))
		})
	}
}

func TestGetOTelHostname(t *testing.T) {
	for _, tt := range []struct {
		name         string
		rattrs       map[string]string
		sattrs       map[string]string
		fallbackHost string
		expected     string
	}{
		{
			name:     "datadog.host.name",
			rattrs:   map[string]string{"datadog.host.name": "test-host"},
			expected: "test-host",
		},
		{
			name:     "_dd.hostname",
			rattrs:   map[string]string{"_dd.hostname": "test-host"},
			expected: "test-host",
		},
		{
			name:         "fallback hostname",
			fallbackHost: "test-host",
			expected:     "test-host",
		},
		{
			name: "complete Azure App Service identity is hostless",
			rattrs: map[string]string{
				"cloud.provider":    "azure",
				"cloud.platform":    "azure.app_service",
				"cloud.resource_id": "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Web/sites/my-app",
			},
			fallbackHost: "collector-host",
		},
		{
			name: "complete AWS Lambda identity is hostless",
			rattrs: map[string]string{
				"cloud.provider": "aws",
				"cloud.platform": "aws_lambda",
				"faas.id":        "arn:aws:lambda:us-east-1:123456789012:function:orders",
			},
			fallbackHost: "collector-host",
		},
		{
			name: "complete Cloud Run Jobs identity is hostless",
			rattrs: map[string]string{
				"cloud.provider":              "gcp",
				"cloud.platform":              "gcp_cloud_run",
				"cloud.resource_id":           "//run.googleapis.com/projects/project-1/locations/us-central1/jobs/daily-job",
				"faas.instance":               "instance-1",
				"gcp.cloud_run.job.execution": "daily-job-abc",
			},
			fallbackHost: "collector-host",
		},
		{
			name: "complete GKE Autopilot identity is hostless",
			rattrs: map[string]string{
				"cloud.provider":   "gcp",
				"cloud.platform":   "gcp_kubernetes_engine",
				"cloud.account.id": "project-1",
				"k8s.cluster.name": "autopilot-cluster",
				"host.name":        "gk3-autopilot-cluster-pool-abc",
			},
			fallbackHost: "collector-host",
		},
		{
			name: "incomplete Lambda identity keeps current host fallback",
			rattrs: map[string]string{
				"cloud.provider": "aws",
				"cloud.platform": "aws_lambda",
				"host.id":        "existing-host",
			},
			fallbackHost: "collector-host",
			expected:     "existing-host",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			set := componenttest.NewNopTelemetrySettings()
			set.MeterProvider = noop.NewMeterProvider()
			tr, err := attributes.NewTranslator(set)
			assert.NoError(t, err)
			actual := GetOTelHostname(span, res, tr, tt.fallbackHost)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelVersion(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
	}{
		{
			name:     "neither set",
			expected: "",
		},
		{
			name:     "only in resource",
			rattrs:   map[string]string{string(semconv127.ServiceVersionKey): "v1"},
			expected: "v1",
		},
		{
			name:     "only in span",
			sattrs:   map[string]string{string(semconv127.ServiceVersionKey): "v3"},
			expected: "v3",
		},
		{
			name:     "both set (span wins)",
			sattrs:   map[string]string{string(semconv127.ServiceVersionKey): "v3"},
			rattrs:   map[string]string{string(semconv127.ServiceVersionKey): "v4"},
			expected: "v3",
		},
		{
			name:     "normalization",
			sattrs:   map[string]string{string(semconv127.ServiceVersionKey): "  V1 "},
			expected: "_v1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetOTelVersion(span, res))
		})
	}
}

func TestGetOTelContainerID(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
	}{
		{
			name:     "neither set",
			expected: "",
		},
		{
			name:     "only in resource",
			rattrs:   map[string]string{string(semconv117.ContainerIDKey): "cid-res"},
			expected: "cid-res",
		},
		{
			name:     "only in span",
			sattrs:   map[string]string{string(semconv117.ContainerIDKey): "cid-span"},
			expected: "cid-span",
		},
		{
			name:     "both set (span wins)",
			sattrs:   map[string]string{string(semconv117.ContainerIDKey): "cid-span"},
			rattrs:   map[string]string{string(semconv117.ContainerIDKey): "cid-res"},
			expected: "cid-span",
		},
		{
			name:     "normalization",
			sattrs:   map[string]string{string(semconv117.ContainerIDKey): "  CID "},
			expected: "_cid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetOTelContainerID(span, res))
		})
	}
}

func TestGetOTelStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]uint32
		rattrs   map[string]uint32
		expected uint32
	}{
		{
			name:     "neither set",
			expected: 0,
		},
		// http.status_code (semconv 1.6.1 - 1.17) tests
		{
			name: "http.status_code only in span",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
			},
			expected: 200,
		},
		{
			name: "http.status_code only in resource",
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 201,
			},
			expected: 201,
		},
		{
			name:     "http.status_code in both span and resource (span wins)",
			sattrs:   map[string]uint32{string(semconv117.HTTPStatusCodeKey): 203},
			rattrs:   map[string]uint32{string(semconv117.HTTPStatusCodeKey): 204},
			expected: 203,
		},
		// http.response.status_code (semconv 1.23+) tests
		{
			name: "http.response.status_code only in span",
			sattrs: map[string]uint32{
				"http.response.status_code": 200,
			},
			expected: 200,
		},
		{
			name: "http.response.status_code only in resource",
			rattrs: map[string]uint32{
				"http.response.status_code": 404,
			},
			expected: 404,
		},
		{
			name: "http.response.status_code in both span and resource (span wins)",
			sattrs: map[string]uint32{
				"http.response.status_code": 201,
			},
			rattrs: map[string]uint32{
				"http.response.status_code": 500,
			},
			expected: 201,
		},
		// Precedence between old and new attributes
		{
			name: "http.status_code takes precedence over http.response.status_code in span",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
				"http.response.status_code":          201,
			},
			expected: 200,
		},
		{
			name: "http.status_code takes precedence over http.response.status_code in resource",
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 201,
				"http.response.status_code":          202,
			},
			expected: 201,
		},
		{
			name: "http.status_code in span beats http.response.status_code in resource",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
			},
			rattrs: map[string]uint32{
				"http.response.status_code": 500,
			},
			expected: 200,
		},
		// NOTE: The following test case documents the OLD behavior before semantic lookup migration.
		// With semantic lookup, http.status_code (higher precedence key) in resource takes
		// precedence over http.response.status_code (lower precedence key) in span.
		// This test is commented out as the behavior has intentionally changed.
		// {
		// 	name: "http.response.status_code in span beats http.status_code in resource",
		// 	sattrs: map[string]uint32{
		// 		"http.response.status_code": 201,
		// 	},
		// 	rattrs: map[string]uint32{
		// 		string(semconv117.HTTPStatusCodeKey): 500,
		// 	},
		// 	expected: 201, // OLD behavior: span attribute always wins
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutInt(k, int64(v))
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutInt(k, int64(v))
			}
			assert.Equal(t, tt.expected, GetOTelStatusCode(span, res))
		})
	}
}

func TestOtelSpanToDDSpanDBNameMapping(t *testing.T) {
	tests := []struct {
		name         string
		sattrs       map[string]string
		rattrs       map[string]string
		expectedName string
		shouldMap    bool
	}{
		{
			name:         "db.namespace in span attributes, no db.name",
			sattrs:       map[string]string{string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "testdb",
			shouldMap:    true,
		},
		{
			name:         "db.namespace in resource attributes, no db.name",
			rattrs:       map[string]string{string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "testdb",
			shouldMap:    true,
		},
		{
			name:         "db.namespace in both, span takes precedence",
			sattrs:       map[string]string{string(semconv127.DBNamespaceKey): "span-db"},
			rattrs:       map[string]string{string(semconv127.DBNamespaceKey): "resource-db"},
			expectedName: "span-db",
			shouldMap:    true,
		},
		{
			name:         "db.name already exists, should not map",
			sattrs:       map[string]string{"db.name": "existing-db", string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "existing-db",
			shouldMap:    false,
		},
		{
			name:      "no db.namespace, should not map",
			sattrs:    map[string]string{},
			shouldMap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}

			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			cfg := &config.AgentConfig{}
			cfg.OTLPReceiver = &config.OTLP{}
			cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			if tt.shouldMap {
				assert.Equal(t, tt.expectedName, ddspan.Meta["db.name"])
			} else {
				if tt.expectedName != "" {
					assert.Equal(t, tt.expectedName, ddspan.Meta["db.name"])
				} else {
					assert.Empty(t, ddspan.Meta["db.name"])
				}
			}
		})
	}
}

// TestOtelSpanToDDSpanTraceStatePreservation verifies that the raw W3C
// tracestate is preserved on the DD span's Meta for both the minimal and full
// conversions. The minimal conversion feeds the APM stats Concentrator, which
// relies on this value to recover head-sampling probability.
func TestOtelSpanToDDSpanTraceStatePreservation(t *testing.T) {
	tests := []struct {
		name       string
		tracestate string
		expectKey  bool
	}{
		{
			name:       "tracestate present",
			tracestate: "ot=th:8",
			expectKey:  true,
		},
		{
			name:       "tracestate present with multiple members",
			tracestate: "ot=th:8;rv:abcdefabcdefab,foo=bar",
			expectKey:  true,
		},
		{
			name:       "empty tracestate omits key",
			tracestate: "",
			expectKey:  false,
		},
	}

	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			newSpan := func() ptrace.Span {
				span := ptrace.NewSpan()
				span.SetName("test-span")
				span.TraceState().FromRaw(tt.tracestate)
				return span
			}

			// Minimal conversion (APM stats path).
			minSpan := OtelSpanToDDSpanMinimalWithTagKeys(newSpan(), pcommon.NewResource(), lib, false, false, newCfg(), OTelSpanTagKeys{})
			// Full conversion.
			fullSpan := OtelSpanToDDSpan(newSpan(), pcommon.NewResource(), lib, newCfg())

			if tt.expectKey {
				assert.Equal(t, tt.tracestate, minSpan.Meta["w3c.tracestate"])
				assert.Equal(t, tt.tracestate, fullSpan.Meta["w3c.tracestate"])
			} else {
				assert.NotContains(t, minSpan.Meta, "w3c.tracestate")
				assert.NotContains(t, fullSpan.Meta, "w3c.tracestate")
			}
		})
	}
}

// TestOtelSpanToDDSpanSampleRateInjection verifies that the head-based sampling
// probability decoded from the W3C tracestate is injected as _sample_rate on the
// converted DD span for both the minimal (APM stats) and full conversions, and
// that an explicit upstream _sample_rate is never overwritten.
func TestOtelSpanToDDSpanSampleRateInjection(t *testing.T) {
	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("test-lib")

	t.Run("injects _sample_rate from tracestate", func(t *testing.T) {
		tests := []struct {
			name       string
			tracestate string
			wantRate   float64
		}{
			{"th encoding 50%", "ot=th:8", 0.5},
			{"p encoding 50%", "ot=p:1;r:1", 0.5},
			{"p encoding 6.25%", "ot=p:4;r:4", 0.0625},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				newSpan := func() ptrace.Span {
					span := ptrace.NewSpan()
					span.SetName("test-span")
					span.TraceState().FromRaw(tt.tracestate)
					return span
				}

				minSpan := OtelSpanToDDSpanMinimalWithTagKeys(newSpan(), pcommon.NewResource(), lib, false, false, newCfg(), OTelSpanTagKeys{})
				rate, ok := minSpan.Metrics["_sample_rate"]
				require.True(t, ok, "minimal conversion must set _sample_rate")
				assert.InDelta(t, tt.wantRate, rate, 1e-9)

				fullSpan := OtelSpanToDDSpan(newSpan(), pcommon.NewResource(), lib, newCfg())
				rate, ok = fullSpan.Metrics["_sample_rate"]
				require.True(t, ok, "full conversion must set _sample_rate")
				assert.InDelta(t, tt.wantRate, rate, 1e-9)
			})
		}
	})

	t.Run("no tracestate leaves _sample_rate unset", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, pcommon.NewResource(), lib, false, false, newCfg(), OTelSpanTagKeys{})
		_, ok := minSpan.Metrics["_sample_rate"]
		assert.False(t, ok)
	})

	t.Run("tracestate with no probability leaves _sample_rate unset", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.TraceState().FromRaw("foo=bar")
		fullSpan := OtelSpanToDDSpan(span, pcommon.NewResource(), lib, newCfg())
		_, ok := fullSpan.Metrics["_sample_rate"]
		assert.False(t, ok)
	})
}

// TestOtelSpanToDDSpanMinimalPrimaryTags verifies that span-derived primary tag
// keys are copied into the minimal span's Meta (so the APM stats Concentrator's
// matchingAdditionalMetricTags can aggregate on them), that span attributes take
// precedence over resource attributes, and that unlisted keys are not copied.
func TestOtelSpanToDDSpanMinimalPrimaryTags(t *testing.T) {
	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("test-lib")

	t.Run("copies configured keys from span and resource attrs", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "checkout")
		res := pcommon.NewResource()
		res.Attributes().PutStr("region", "us-east-1")

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), NewOTelSpanTagKeys(nil, []string{"team", "region"}))

		assert.Equal(t, "checkout", minSpan.Meta["team"])
		assert.Equal(t, "us-east-1", minSpan.Meta["region"])
	})

	t.Run("span attribute takes precedence over resource attribute", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "span-team")
		res := pcommon.NewResource()
		res.Attributes().PutStr("team", "resource-team")

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), NewOTelSpanTagKeys(nil, []string{"team"}))

		assert.Equal(t, "span-team", minSpan.Meta["team"])
	})

	t.Run("unlisted keys are not copied and empty values are skipped", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "checkout")
		span.Attributes().PutStr("not-a-primary-tag", "ignored")
		span.Attributes().PutStr("empty", "")

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, pcommon.NewResource(), lib, false, false, newCfg(), NewOTelSpanTagKeys(nil, []string{"team", "empty", "missing"}))

		assert.Equal(t, "checkout", minSpan.Meta["team"])
		assert.NotContains(t, minSpan.Meta, "not-a-primary-tag")
		assert.NotContains(t, minSpan.Meta, "empty")
		assert.NotContains(t, minSpan.Meta, "missing")
	})

	t.Run("empty span attribute falls back to resource attribute", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "")
		res := pcommon.NewResource()
		res.Attributes().PutStr("team", "platform")

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), NewOTelSpanTagKeys(nil, []string{"team"}))

		// An empty span value is treated as absent, so the resource value is used.
		assert.Equal(t, "platform", minSpan.Meta["team"])
	})

	t.Run("nil primaryTagKeys copies nothing", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "checkout")

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, pcommon.NewResource(), lib, false, false, newCfg(), OTelSpanTagKeys{})

		assert.NotContains(t, minSpan.Meta, "team")
	})
}

// TestOtelSpanToDDSpanMinimalNormalization verifies that spans produced by
// OtelSpanToDDSpanMinimal are sanitized the same way the full trace-agent
// pipeline (Agent.normalize) would sanitize them, since minimal spans are fed
// directly into the APM stats Concentrator and never reach that pipeline.
func TestOtelSpanToDDSpanMinimalNormalization(t *testing.T) {
	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}
	lib := pcommon.NewInstrumentationScope()

	t.Run("name is normalized under operation name v2", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.Attributes().PutStr("operation.name", "invalid-op-name")
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), OTelSpanTagKeys{})

		// Hyphens aren't valid in span names; NormalizeName replaces them with underscores.
		assert.Equal(t, "invalid_op_name", minSpan.Name)
	})

	t.Run("negative duration is reset to zero", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		span.SetEndTimestamp(pcommon.Timestamp(int64(span.StartTimestamp()) - 1))
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), OTelSpanTagKeys{})

		assert.EqualValues(t, 0, minSpan.Duration)
	})

	t.Run("garbage start time is reset to now", func(t *testing.T) {
		minStart := time.Now().UnixNano()
		span := ptrace.NewSpan()
		span.SetStartTimestamp(42)
		span.SetEndTimestamp(pcommon.Timestamp(200000000))
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), OTelSpanTagKeys{})

		assert.GreaterOrEqual(t, minSpan.Start, minStart-200000000)
		assert.LessOrEqual(t, minSpan.Start, time.Now().UnixNano())
	})

	t.Run("peer.service is truncated to the max service length", func(t *testing.T) {
		span := ptrace.NewSpan()
		longPeerSvc := strings.Repeat("a", 150)
		span.Attributes().PutStr("peer.service", longPeerSvc)
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), NewOTelSpanTagKeys([]string{"peer.service"}, nil))

		assert.Len(t, minSpan.Meta["peer.service"], normalizeutil.MaxServiceLen)
		assert.Equal(t, strings.Repeat("a", normalizeutil.MaxServiceLen), minSpan.Meta["peer.service"])
	})

	t.Run("_dd.base_service is truncated to the max service length", func(t *testing.T) {
		span := ptrace.NewSpan()
		longBaseSvc := strings.Repeat("a", 150)
		span.Attributes().PutStr("_dd.base_service", longBaseSvc)
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimalWithTagKeys(span, res, lib, false, false, newCfg(), NewOTelSpanTagKeys([]string{"_dd.base_service"}, nil))

		assert.Len(t, minSpan.Meta["_dd.base_service"], normalizeutil.MaxServiceLen)
		assert.Equal(t, strings.Repeat("a", normalizeutil.MaxServiceLen), minSpan.Meta["_dd.base_service"])
	})
}

// TestGetOTelEnv_SemconvVersionPrecedence tests environment extraction with multiple semconv versions.
// Semconv 1.27+ uses deployment.environment.name, 1.17+ uses deployment.environment.
func TestGetOTelEnv_SemconvVersionPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
	}{
		{
			name:     "semconv127 deployment.environment.name only",
			rattrs:   map[string]string{string(semconv127.DeploymentEnvironmentNameKey): "prod-127"},
			expected: "prod-127",
		},
		{
			name:     "semconv117 deployment.environment only",
			rattrs:   map[string]string{string(semconv117.DeploymentEnvironmentKey): "prod-117"},
			expected: "prod-117",
		},
		{
			name: "semconv127 takes precedence over semconv117",
			rattrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "prod-127",
				string(semconv117.DeploymentEnvironmentKey):     "prod-117",
			},
			expected: "prod-127",
		},
		{
			name: "span semconv127 takes precedence over resource semconv127",
			sattrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "span-env",
			},
			rattrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "res-env",
			},
			expected: "span-env",
		},
		// NOTE: The following test cases document the OLD behavior before semantic lookup migration.
		// With semantic lookup, deployment.environment.name (semconv127) has higher precedence
		// than deployment.environment (semconv117), so the higher precedence key wins regardless
		// of whether it's in span or resource attributes.
		// These tests are commented out as the behavior has intentionally changed.
		// {
		// 	name: "span semconv117 takes precedence over resource semconv127",
		// 	sattrs: map[string]string{
		// 		string(semconv117.DeploymentEnvironmentKey): "span-env-117",
		// 	},
		// 	rattrs: map[string]string{
		// 		string(semconv127.DeploymentEnvironmentNameKey): "res-env-127",
		// 	},
		// 	expected: "span-env-117", // OLD behavior: span attribute always wins
		// },
		// {
		// 	name: "mixed: span has semconv117, resource has both versions",
		// 	sattrs: map[string]string{
		// 		string(semconv117.DeploymentEnvironmentKey): "span-117",
		// 	},
		// 	rattrs: map[string]string{
		// 		string(semconv127.DeploymentEnvironmentNameKey): "res-127",
		// 		string(semconv117.DeploymentEnvironmentKey):     "res-117",
		// 	},
		// 	expected: "span-117", // OLD behavior: span attribute always wins
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetOTelEnv(span, res))
		})
	}
}

// TestOtelSpanToDDSpan_HTTPAttributeMappings tests full span conversion with HTTP attribute mappings
// (http.request.method → http.method, http.response.status_code → http.status_code, server.address → http.server_name).
func TestOtelSpanToDDSpan_HTTPAttributeMappings(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]interface{}
		rattrs       map[string]interface{}
		expectedMeta map[string]string
	}{
		{
			name: "http.request.method (semconv 1.23+) mapped to http.method",
			sattrs: map[string]interface{}{
				"http.request.method": "GET",
			},
			expectedMeta: map[string]string{
				"http.method": "GET",
			},
		},
		{
			// http.response.status_code is mapped to http.status_code via HTTPMappings.
			// When passed as a string, it goes to meta; when int64, it goes to metrics.
			name: "http.response.status_code (semconv 1.23+) mapped to http.status_code",
			sattrs: map[string]interface{}{
				"http.response.status_code": "200", // String attribute
			},
			expectedMeta: map[string]string{
				"http.status_code": "200", // Mapped key
			},
		},
		// NOTE: The following test case documents the OLD behavior before semantic lookup migration.
		// With semantic lookup, http.request.method (semconv 1.21+) has higher precedence
		// than http.method (old semconv). This test is commented out as the behavior
		// has intentionally changed - NEW semconv keys now take precedence.
		// {
		// 	name: "both old and new HTTP method - old takes precedence for http.method",
		// 	sattrs: map[string]interface{}{
		// 		"http.method":         "POST",
		// 		"http.request.method": "GET",
		// 	},
		// 	expectedMeta: map[string]string{
		// 		"http.method": "POST", // OLD behavior: old convention wins
		// 	},
		// },
		{
			name: "server.address (semconv 1.17+) mapped to http.server_name",
			rattrs: map[string]interface{}{
				string(semconv127.ServerAddressKey): "api.example.com",
			},
			expectedMeta: map[string]string{
				"http.server_name": "api.example.com",
			},
		},
		{
			name: "client.address (semconv 1.27+) mapped to http.client_ip",
			rattrs: map[string]interface{}{
				string(semconv127.ClientAddressKey): "192.168.1.1",
			},
			expectedMeta: map[string]string{
				"http.client_ip": "192.168.1.1",
			},
		},
		{
			name: "url.full (semconv 1.27+) mapped to http.url",
			rattrs: map[string]interface{}{
				string(semconv127.URLFullKey): "https://api.example.com/users",
			},
			expectedMeta: map[string]string{
				"http.url": "https://api.example.com/users",
			},
		},
		{
			name: "user_agent.original (semconv 1.27+) mapped to http.useragent",
			rattrs: map[string]interface{}{
				string(semconv127.UserAgentOriginalKey): "Mozilla/5.0",
			},
			expectedMeta: map[string]string{
				"http.useragent": "Mozilla/5.0",
			},
		},
		{
			name: "network.protocol.version (semconv 1.27+) mapped to http.version",
			rattrs: map[string]interface{}{
				string(semconv127.NetworkProtocolVersionKey): "1.1",
			},
			expectedMeta: map[string]string{
				"http.version": "1.1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindServer)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			for k, v := range tt.sattrs {
				switch val := v.(type) {
				case string:
					span.Attributes().PutStr(k, val)
				case int64:
					span.Attributes().PutInt(k, val)
				}
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				switch val := v.(type) {
				case string:
					res.Attributes().PutStr(k, val)
				case int64:
					res.Attributes().PutInt(k, val)
				}
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
		})
	}
}

// TestOtelSpanToDDSpan_DBAttributeMappings tests full span conversion with database attribute mappings
// (db.query.text, db.statement preservation, db.namespace → db.name).
func TestOtelSpanToDDSpan_DBAttributeMappings(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]interface{}
		rattrs       map[string]interface{}
		expectedMeta map[string]string
	}{
		{
			name: "db.query.text (semconv 1.26+) preserved",
			sattrs: map[string]interface{}{
				string(semconv126.DBQueryTextKey): "SELECT * FROM users",
				"db.system":                       "postgresql",
			},
			expectedMeta: map[string]string{
				"db.query.text": "SELECT * FROM users",
			},
		},
		{
			name: "db.statement (semconv 1.6.1) preserved",
			sattrs: map[string]interface{}{
				"db.statement": "SELECT * FROM orders",
				"db.system":    "postgresql",
			},
			expectedMeta: map[string]string{
				"db.statement": "SELECT * FROM orders",
			},
		},
		{
			name: "both db.query.text and db.statement - both preserved",
			sattrs: map[string]interface{}{
				string(semconv126.DBQueryTextKey): "SELECT * FROM users",
				"db.statement":                    "SELECT * FROM orders",
				"db.system":                       "postgresql",
			},
			expectedMeta: map[string]string{
				"db.query.text": "SELECT * FROM users",
				"db.statement":  "SELECT * FROM orders",
			},
		},
		{
			name: "db.namespace (semconv 1.26+) mapped to db.name",
			rattrs: map[string]interface{}{
				string(semconv127.DBNamespaceKey): "production_db",
				"db.system":                       "postgresql",
			},
			expectedMeta: map[string]string{
				"db.name":      "production_db",
				"db.namespace": "production_db",
			},
		},
		{
			name: "db.name takes precedence over db.namespace",
			rattrs: map[string]interface{}{
				"db.name":                         "existing_db",
				string(semconv127.DBNamespaceKey): "namespace_db",
				"db.system":                       "postgresql",
			},
			expectedMeta: map[string]string{
				"db.name":      "existing_db",
				"db.namespace": "namespace_db",
			},
		},
		{
			name: "db.collection.name (semconv 1.26+) preserved",
			sattrs: map[string]interface{}{
				string(semconv126.DBCollectionNameKey): "users",
				"db.system":                            "mongodb",
			},
			expectedMeta: map[string]string{
				"db.collection.name": "users",
			},
		},
		{
			name: "db.operation.name (semconv 1.26+) preserved",
			sattrs: map[string]interface{}{
				string(semconv126.DBOperationNameKey): "findAndModify",
				"db.system":                           "mongodb",
			},
			expectedMeta: map[string]string{
				"db.operation.name": "findAndModify",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindClient)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			for k, v := range tt.sattrs {
				switch val := v.(type) {
				case string:
					span.Attributes().PutStr(k, val)
				case int64:
					span.Attributes().PutInt(k, val)
				}
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				switch val := v.(type) {
				case string:
					res.Attributes().PutStr(k, val)
				case int64:
					res.Attributes().PutInt(k, val)
				}
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				require.Contains(t, ddspan.Meta, key, "expected key %s to exist", key)
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
		})
	}
}

// TestOtelSpanToDDSpan_MessagingAttributePreservation tests messaging attribute preservation
// (messaging.destination, messaging.destination.name, messaging.operation).
func TestOtelSpanToDDSpan_MessagingAttributePreservation(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]interface{}
		rattrs       map[string]interface{}
		expectedMeta map[string]string
	}{
		{
			name: "messaging.destination (semconv 1.6.1) preserved",
			sattrs: map[string]interface{}{
				"messaging.destination": "my-queue",
				"messaging.system":      "kafka",
			},
			expectedMeta: map[string]string{
				"messaging.destination": "my-queue",
			},
		},
		{
			name: "messaging.destination.name (semconv 1.17+) preserved",
			sattrs: map[string]interface{}{
				string(semconv117.MessagingDestinationNameKey): "my-topic",
				"messaging.system": "kafka",
			},
			expectedMeta: map[string]string{
				string(semconv117.MessagingDestinationNameKey): "my-topic",
			},
		},
		{
			name: "both messaging.destination variants preserved",
			sattrs: map[string]interface{}{
				"messaging.destination":                        "old-queue",
				string(semconv117.MessagingDestinationNameKey): "new-topic",
				"messaging.system":                             "kafka",
			},
			expectedMeta: map[string]string{
				"messaging.destination":                        "old-queue",
				string(semconv117.MessagingDestinationNameKey): "new-topic",
			},
		},
		{
			name: "messaging.operation preserved",
			sattrs: map[string]interface{}{
				"messaging.operation": "publish",
				"messaging.system":    "rabbitmq",
			},
			expectedMeta: map[string]string{
				"messaging.operation": "publish",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindProducer)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			for k, v := range tt.sattrs {
				switch val := v.(type) {
				case string:
					span.Attributes().PutStr(k, val)
				case int64:
					span.Attributes().PutInt(k, val)
				}
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				switch val := v.(type) {
				case string:
					res.Attributes().PutStr(k, val)
				case int64:
					res.Attributes().PutInt(k, val)
				}
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				require.Contains(t, ddspan.Meta, key, "expected key %s to exist", key)
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
		})
	}
}

// TestOtelSpanToDDSpan_NetworkAttributeMappings tests network/server attribute mappings
// (net.peer.name, server.address → http.server_name, server.port).
func TestOtelSpanToDDSpan_NetworkAttributeMappings(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]interface{}
		rattrs       map[string]interface{}
		expectedMeta map[string]string
	}{
		{
			name: "net.peer.name (semconv 1.6.1) preserved",
			sattrs: map[string]interface{}{
				"net.peer.name": "remote-host.example.com",
			},
			expectedMeta: map[string]string{
				"net.peer.name": "remote-host.example.com",
			},
		},
		{
			name: "net.peer.port (semconv 1.6.1) preserved",
			sattrs: map[string]interface{}{
				"net.peer.port": int64(8080),
			},
			expectedMeta: map[string]string{},
		},
		{
			name: "server.address (semconv 1.17+) mapped to http.server_name",
			rattrs: map[string]interface{}{
				string(semconv127.ServerAddressKey): "server.example.com",
			},
			expectedMeta: map[string]string{
				"http.server_name": "server.example.com",
			},
		},
		{
			name: "server.port (semconv 1.17+) preserved",
			rattrs: map[string]interface{}{
				string(semconv127.ServerPortKey): int64(443),
			},
			expectedMeta: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindClient)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			for k, v := range tt.sattrs {
				switch val := v.(type) {
				case string:
					span.Attributes().PutStr(k, val)
				case int64:
					span.Attributes().PutInt(k, val)
				}
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				switch val := v.(type) {
				case string:
					res.Attributes().PutStr(k, val)
				case int64:
					res.Attributes().PutInt(k, val)
				}
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				require.Contains(t, ddspan.Meta, key, "expected key %s to exist", key)
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
		})
	}
}

// =============================================================================
// FALLBACK INCONSISTENCY TESTS
// These tests document the CURRENT behavior of hardcoded fallbacks.
// Some of these behaviors may be inconsistent and are candidates for cleanup
// when the new semantics library is introduced.
// =============================================================================

// TestFallbackInconsistency_HTTPStatusCodePrecedence documents that http.status_code (old)
// takes precedence over http.response.status_code (new) in GetOTelStatusCode.
// With semantic lookup, key precedence is checked across both span and resource attributes.
func TestFallbackInconsistency_HTTPStatusCodePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]uint32
		rattrs   map[string]uint32
		expected uint32
		note     string
	}{
		{
			name: "http.status_code takes precedence over http.response.status_code in span",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
				"http.response.status_code":          500,
			},
			expected: 200,
			note:     "Old convention wins when both are in span (higher precedence key).",
		},
		{
			name: "http.status_code in span wins over http.response.status_code in span",
			sattrs: map[string]uint32{
				"http.status_code":          201,
				"http.response.status_code": 404,
			},
			expected: 201,
			note:     "http.status_code is checked before http.response.status_code.",
		},
		// NOTE: The following test case documents the OLD behavior before semantic lookup migration.
		// With semantic lookup, http.status_code (higher precedence key) in resource takes
		// precedence over http.response.status_code (lower precedence key) in span.
		// This test is commented out as the behavior has intentionally changed.
		// {
		// 	name: "span http.response.status_code wins over resource http.status_code",
		// 	sattrs: map[string]uint32{
		// 		"http.response.status_code": 201,
		// 	},
		// 	rattrs: map[string]uint32{
		// 		string(semconv117.HTTPStatusCodeKey): 500,
		// 	},
		// 	expected: 201, // OLD behavior: span attribute always wins
		// 	note:     "Span attributes take precedence over resource attributes.",
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutInt(k, int64(v))
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutInt(k, int64(v))
			}
			actual := GetOTelStatusCode(span, res)
			assert.Equal(t, tt.expected, actual, "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_DBNamespacePrecedence documents the precedence behavior for db.namespace.
func TestFallbackInconsistency_DBNamespacePrecedence(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]string
		rattrs       map[string]string
		expectedName string
		note         string
	}{
		{
			name: "span db.namespace takes precedence over resource db.namespace",
			sattrs: map[string]string{
				string(semconv127.DBNamespaceKey): "span-db",
			},
			rattrs: map[string]string{
				string(semconv127.DBNamespaceKey): "resource-db",
			},
			expectedName: "span-db",
			note:         "Uses OTelSpanAccessor which has span-first precedence (consistent with other lookups).",
		},
		{
			name: "db.namespace only in span - works correctly",
			sattrs: map[string]string{
				string(semconv127.DBNamespaceKey): "span-only-db",
			},
			rattrs:       map[string]string{},
			expectedName: "span-only-db",
			note:         "Falls back to span when not in resource.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetTraceID([16]byte{1})
			span.SetSpanID([8]byte{1})
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}

			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}

			lib := pcommon.NewInstrumentationScope()
			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			assert.Equal(t, tt.expectedName, ddspan.Meta["db.name"], "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_ContainerIDFallback documents that GetOTelContainerID
// only checks container.id and does NOT fall back to k8s.pod.uid or other identifiers.
// Use GetOTelContainerOrPodID if k8s.pod.uid fallback is needed.
func TestFallbackInconsistency_ContainerIDFallback(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
		note     string
	}{
		{
			name:     "container.id only",
			rattrs:   map[string]string{string(semconv117.ContainerIDKey): "container-123"},
			expected: "container-123",
			note:     "Primary key works.",
		},
		{
			name:     "CURRENT: no fallback to k8s.pod.uid",
			rattrs:   map[string]string{string(semconv117.K8SPodUIDKey): "pod-uid-456"},
			expected: "",
			note:     "GetOTelContainerID does NOT fall back to k8s.pod.uid. Use GetOTelContainerOrPodID for that.",
		},
		{
			name:     "CURRENT: no fallback to container.runtime",
			rattrs:   map[string]string{"container.runtime": "docker"},
			expected: "",
			note:     "container.runtime is NOT used as a fallback for container ID.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelContainerID(span, res)
			assert.Equal(t, tt.expected, actual, "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_VersionNoFallback documents that version lookup
// only checks service.version with NO fallback to other version attributes.
func TestFallbackInconsistency_VersionNoFallback(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		rattrs   map[string]string
		expected string
		note     string
	}{
		{
			name:     "service.version only",
			rattrs:   map[string]string{string(semconv127.ServiceVersionKey): "1.2.3"},
			expected: "1.2.3",
			note:     "Primary key works.",
		},
		{
			name:     "CURRENT: no fallback to telemetry.sdk.version",
			rattrs:   map[string]string{"telemetry.sdk.version": "1.0.0"},
			expected: "",
			note:     "telemetry.sdk.version is NOT used as a fallback for version.",
		},
		{
			name:     "CURRENT: no fallback to app.version",
			rattrs:   map[string]string{"app.version": "2.0.0"},
			expected: "",
			note:     "app.version is NOT used as a fallback for version.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelVersion(span, res)
			assert.Equal(t, tt.expected, actual, "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence documents that Status2Error
// now checks http.status_code BEFORE http.response.status_code (matching the semantic registry).
// OLD BEHAVIOR (commented out): Previously checked http.response.status_code first.
func TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		meta        map[string]string
		expectedMsg string
		note        string
	}{
		// COMMENTED OUT: Old behavior test - Status2Error previously used NEW-first precedence
		// Now uses OLD-first precedence to match the semantic registry (http.status_code before http.response.status_code)
		// {
		// 	name: "OLD BEHAVIOR: http.response.status_code checked before http.status_code in Status2Error",
		// 	meta: map[string]string{
		// 		"http.response.status_code": "500",
		// 		"http.status_code":          "200",
		// 	},
		// 	expectedMsg: "500 Internal Server Error",
		// 	note:        "OLD: Status2Error used http.response.status_code first - OPPOSITE of GetOTelStatusCode!",
		// },
		{
			name: "NEW: http.status_code checked before http.response.status_code in Status2Error",
			meta: map[string]string{
				"http.response.status_code": "500",
				"http.status_code":          "200",
			},
			expectedMsg: "200 OK",
			note:        "NEW: Status2Error now uses http.status_code first (matching semantic registry)",
		},
		{
			name: "http.status_code used when http.response.status_code not present",
			meta: map[string]string{
				"http.status_code": "404",
			},
			expectedMsg: "404 Not Found",
			note:        "Falls back to http.status_code when newer key not present.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := ptrace.NewStatus()
			status.SetCode(ptrace.StatusCodeError)
			events := ptrace.NewSpanEventSlice()
			metaCopy := make(map[string]string)
			for k, v := range tt.meta {
				metaCopy[k] = v
			}
			Status2Error(status, events, metaCopy)
			assert.Equal(t, tt.expectedMsg, metaCopy["error.msg"], "Note: %s", tt.note)
		})
	}
}

// TestScopeConvention verifies OtelSpanToDDSpan always reports the deprecated
// otel.library.* aliases, and adds the otel.scope.* keys unless disable_otel_scope_convention is set.
func TestScopeConvention(t *testing.T) {
	tests := []struct {
		name             string
		disableScopeConv bool
		expectScopeKeys  bool
	}{
		{
			name:             "default: emits both otel.scope and otel.library conventions",
			disableScopeConv: false,
			expectScopeKeys:  true,
		},
		{
			name:             "disable_otel_scope_convention: only emits otel.library convention",
			disableScopeConv: true,
			expectScopeKeys:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetTraceID([16]byte{1})
			span.SetSpanID([8]byte{1})

			res := pcommon.NewResource()

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("my-lib")
			lib.SetVersion("1.2.3")

			cfg := &config.AgentConfig{}
			cfg.OTLPReceiver = &config.OTLP{}
			cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
			cfg.Features = make(map[string]struct{})
			if tt.disableScopeConv {
				cfg.Features["disable_otel_scope_convention"] = struct{}{}
			}

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			// The deprecated otel.library.* aliases must always be reported.
			assert.Equal(t, "my-lib", ddspan.Meta[string(semconv117.OtelLibraryNameKey)])
			assert.Equal(t, "1.2.3", ddspan.Meta[string(semconv117.OtelLibraryVersionKey)])

			if tt.expectScopeKeys {
				assert.Equal(t, "my-lib", ddspan.Meta[string(semconv117.OtelScopeNameKey)])
				assert.Equal(t, "1.2.3", ddspan.Meta[string(semconv117.OtelScopeVersionKey)])
			} else {
				assert.NotContains(t, ddspan.Meta, string(semconv117.OtelScopeNameKey))
				assert.NotContains(t, ddspan.Meta, string(semconv117.OtelScopeVersionKey))
			}
		})
	}
}

// buildReadOnlySafetyCorpus builds traces exercising every span feature the DDOT traces
// path touches.
func buildReadOnlySafetyCorpus() ptrace.Traces {
	traces := ptrace.NewTraces()
	rspan := traces.ResourceSpans().AppendEmpty()

	rattrs := rspan.Resource().Attributes()
	rattrs.PutStr("service.name", "readonly-svc")
	rattrs.PutStr("deployment.environment.name", "prod")
	rattrs.PutStr("service.version", "1.2.3")
	rattrs.PutStr("host.name", "test-host")
	rattrs.PutStr("container.id", "abcdef0123456789")
	rattrs.PutStr("container.name", "test-container")
	rattrs.PutStr("container.image.name", "datadog/agent")
	rattrs.PutStr("container.image.tag", "7")
	rattrs.PutStr("k8s.pod.uid", "pod-uid-1")
	rattrs.PutStr("k8s.namespace.name", "default")
	rattrs.PutStr("k8s.node.name", "node-1")
	rattrs.PutStr("k8s.deployment.name", "deploy-1")
	rattrs.PutStr("aws.ecs.task.arn", "arn:aws:ecs:us-east-1:123456789012:task/abc")
	rattrs.PutStr("cloud.provider", "aws")
	rattrs.PutStr("cloud.region", "us-east-1")

	sspan := rspan.ScopeSpans().AppendEmpty()
	sspan.Scope().SetName("test-scope")
	sspan.Scope().SetVersion("v0.1.0")
	sspan.Scope().Attributes().PutStr("scope.attr", "scope-value")

	span := sspan.Spans().AppendEmpty()
	span.SetName("readonly-span")
	span.SetKind(ptrace.SpanKindServer)
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.TraceState().FromRaw("dd=p:0123456789abcdef;s:1")
	// Post-2000 so normalize.FixStartTime leaves them alone: it rewrites older start
	// times to time.Now(), which would make the converted span non-deterministic.
	span.SetStartTimestamp(pcommon.Timestamp(1700000000000000100))
	span.SetEndTimestamp(pcommon.Timestamp(1700000000000000200))
	span.SetDroppedAttributesCount(7)
	span.SetDroppedEventsCount(3)
	span.SetDroppedLinksCount(2)
	span.Status().SetCode(ptrace.StatusCodeError)
	span.Status().SetMessage("boom")

	sattrs := span.Attributes()
	sattrs.PutStr("http.request.method", "GET")
	sattrs.PutStr("url.full", "https://example.com/a?b=c")
	sattrs.PutInt("http.response.status_code", 500)
	sattrs.PutStr("db.system.name", "postgresql")
	sattrs.PutStr("db.namespace", "testdb")
	sattrs.PutStr("db.query.text", "SELECT 1")
	sattrs.PutStr("messaging.system", "kafka")
	sattrs.PutStr("messaging.operation.name", "publish")
	sattrs.PutStr("network.peer.address", "10.0.0.1")
	sattrs.PutInt("network.peer.port", 5432)
	sattrs.PutStr("peer.service", "upstream")

	// One attribute per pcommon.ValueType, plus a dropped-attribute count.
	event := span.Events().AppendEmpty()
	event.SetName("exception")
	event.SetTimestamp(pcommon.Timestamp(150))
	event.SetDroppedAttributesCount(5)
	eattrs := event.Attributes()
	eattrs.PutStr("exception.message", "OOM")
	eattrs.PutStr("exception.type", "mem")
	eattrs.PutStr("exception.stacktrace", "1/2/3")
	eattrs.PutInt("int.attr", 42)
	eattrs.PutDouble("double.attr", 2.5)
	eattrs.PutBool("bool.attr", true)
	eattrs.PutEmptyBytes("bytes.attr").FromRaw([]byte{0xde, 0xad})
	eattrs.PutEmpty("empty.attr")
	nested := eattrs.PutEmptyMap("map.attr")
	nested.PutStr("inner", "value")
	arr := eattrs.PutEmptySlice("slice.attr")
	arr.AppendEmpty().SetStr("one")
	arr.AppendEmpty().SetInt(2)

	// Non-finite doubles, which some encodings (JSON) can't represent.
	nonfinite := span.Events().AppendEmpty()
	nonfinite.SetName("nonfinite")
	nfattrs := nonfinite.Attributes()
	nfattrs.PutDouble("nan.attr", math.NaN())
	nfattrs.PutDouble("posinf.attr", math.Inf(1))
	nfattrs.PutDouble("neginf.attr", math.Inf(-1))
	nfattrs.PutStr("finite.attr", "kept")
	nfattrs.PutEmptySlice("nan.in.slice").AppendEmpty().SetDouble(math.NaN())
	nfattrs.PutEmptyMap("nan.in.map").PutDouble("inner", math.NaN())

	// An event with a dropped count but no attributes, and one with neither.
	bare := span.Events().AppendEmpty()
	bare.SetName("bare")
	bare.SetDroppedAttributesCount(1)
	span.Events().AppendEmpty().SetName("empty")

	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID([16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}))
	link.SetSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
	link.TraceState().FromRaw("dd=p:fedcba9876543210")
	link.SetDroppedAttributesCount(4)
	link.Attributes().PutStr("link.attr", "link-value")

	return traces
}

func readOnlySafetyTestConfig(t *testing.T) *config.AgentConfig {
	t.Helper()
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	var err error
	cfg.OTLPReceiver.AttributesTranslator, err = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	return cfg
}

// TestOtelSpanToDDSpanReadOnlyTraces gates sharing pdata between the two consumers of the
// default DDOT traces pipeline: once the exporter is non-mutating the fanout consumer marks
// the shared ptrace.Traces read-only, and pdata setters panic on read-only data. Both
// conversions must stay read-only and produce the same output as on mutable data.
func TestOtelSpanToDDSpanReadOnlyTraces(t *testing.T) {
	cfg := readOnlySafetyTestConfig(t)

	extract := func(traces ptrace.Traces) (ptrace.Span, pcommon.Resource, pcommon.InstrumentationScope) {
		rspan := traces.ResourceSpans().At(0)
		sspan := rspan.ScopeSpans().At(0)
		return sspan.Spans().At(0), rspan.Resource(), sspan.Scope()
	}

	// Baseline, to compare the read-only run against.
	mutable := buildReadOnlySafetyCorpus()
	mSpan, mRes, mLib := extract(mutable)
	wantFull := OtelSpanToDDSpan(mSpan, mRes, mLib, cfg)
	wantMinimal := OtelSpanToDDSpanMinimalWithTagKeys(mSpan, mRes, mLib, true, true, cfg, OTelSpanTagKeys{})

	readOnly := buildReadOnlySafetyCorpus()
	readOnly.MarkReadOnly()
	require.True(t, readOnly.IsReadOnly(), "corpus must be read-only for this test to mean anything")
	roSpan, roRes, roLib := extract(readOnly)

	// The exporter path and the connector path. Neither may write to pdata.
	require.NotPanics(t, func() {
		got := OtelSpanToDDSpan(roSpan, roRes, roLib, cfg)
		// Compared as serialized bytes because the span events hold the corpus's NaN
		// attributes, which are never reflect.DeepEqual to themselves.
		deterministic := proto.MarshalOptions{Deterministic: true}
		wantBytes, err := deterministic.Marshal(wantFull)
		require.NoError(t, err)
		gotBytes, err := deterministic.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, wantBytes, gotBytes)
	}, "OtelSpanToDDSpan must not mutate read-only pdata")

	require.NotPanics(t, func() {
		got := OtelSpanToDDSpanMinimalWithTagKeys(roSpan, roRes, roLib, true, true, cfg, OTelSpanTagKeys{})
		assert.Equal(t, wantMinimal, got)
	}, "OtelSpanToDDSpanMinimal must not mutate read-only pdata")

	// Not merely panic-free: the input must come out untouched. Compared as serialized
	// bytes because the corpus holds NaN attributes, which are never reflect.DeepEqual
	// to themselves; this is also a stricter comparison than struct equality.
	marshaler := &ptrace.ProtoMarshaler{}
	wantBytes, err := marshaler.MarshalTraces(buildReadOnlySafetyCorpus())
	require.NoError(t, err)
	gotBytes, err := marshaler.MarshalTraces(mutable)
	require.NoError(t, err)
	assert.Equal(t, wantBytes, gotBytes, "conversion must not modify input pdata")
}

func TestOTelSpanEventsToDD(t *testing.T) {
	str := func(s string) *pb.AttributeAnyValue {
		return &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: s}
	}
	for _, tt := range []struct {
		name string
		in   func(events ptrace.SpanEventSlice)
		want []*pb.SpanEvent
	}{
		{
			name: "name and timestamp",
			in: func(events ptrace.SpanEventSlice) {
				e := events.AppendEmpty()
				e.SetName("boom")
				e.SetTimestamp(123)
			},
			want: []*pb.SpanEvent{{TimeUnixNano: 123, Name: "boom"}},
		},
		{
			name: "empty event",
			in:   func(events ptrace.SpanEventSlice) { events.AppendEmpty() },
			want: []*pb.SpanEvent{{}},
		},
		{
			name: "dropped attributes count is not reported",
			in: func(events ptrace.SpanEventSlice) {
				e := events.AppendEmpty()
				e.SetName("boom")
				e.SetDroppedAttributesCount(3)
			},
			want: []*pb.SpanEvent{{Name: "boom"}},
		},
		{
			name: "scalar attributes keep their type",
			in: func(events ptrace.SpanEventSlice) {
				attrs := events.AppendEmpty().Attributes()
				attrs.PutStr("str", `a "quoted" \ value`)
				attrs.PutBool("bool", true)
				attrs.PutInt("int", -42)
				attrs.PutDouble("double", 2.5)
			},
			want: []*pb.SpanEvent{{Attributes: map[string]*pb.AttributeAnyValue{
				"str":    str(`a "quoted" \ value`),
				"bool":   {Type: pb.AttributeAnyValue_BOOL_VALUE, BoolValue: true},
				"int":    {Type: pb.AttributeAnyValue_INT_VALUE, IntValue: -42},
				"double": {Type: pb.AttributeAnyValue_DOUBLE_VALUE, DoubleValue: 2.5},
			}}},
		},
		{
			name: "bytes are base64 encoded and empty values dropped",
			in: func(events ptrace.SpanEventSlice) {
				attrs := events.AppendEmpty().Attributes()
				attrs.PutEmptyBytes("bytes").FromRaw([]byte{0xde, 0xad})
				attrs.PutEmpty("empty")
			},
			want: []*pb.SpanEvent{{Attributes: map[string]*pb.AttributeAnyValue{"bytes": str("3q0=")}}},
		},
		{
			name: "slices of a single scalar type are kept as arrays",
			in: func(events ptrace.SpanEventSlice) {
				attrs := events.AppendEmpty().Attributes()
				s := attrs.PutEmptySlice("strs")
				s.AppendEmpty().SetStr("a")
				s.AppendEmpty().SetStr("b")
				b := attrs.PutEmptySlice("bools")
				b.AppendEmpty().SetBool(true)
				b.AppendEmpty().SetBool(false)
				i := attrs.PutEmptySlice("ints")
				i.AppendEmpty().SetInt(1)
				i.AppendEmpty().SetInt(2)
				d := attrs.PutEmptySlice("doubles")
				d.AppendEmpty().SetDouble(1.5)
				attrs.PutEmptySlice("empty")
			},
			want: []*pb.SpanEvent{{Attributes: map[string]*pb.AttributeAnyValue{
				"strs": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{
					{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: "a"},
					{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: "b"},
				}}},
				"bools": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{
					{Type: pb.AttributeArrayValue_BOOL_VALUE, BoolValue: true},
					{Type: pb.AttributeArrayValue_BOOL_VALUE, BoolValue: false},
				}}},
				"ints": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{
					{Type: pb.AttributeArrayValue_INT_VALUE, IntValue: 1},
					{Type: pb.AttributeArrayValue_INT_VALUE, IntValue: 2},
				}}},
				"doubles": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{
					{Type: pb.AttributeArrayValue_DOUBLE_VALUE, DoubleValue: 1.5},
				}}},
				"empty": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{}},
			}}},
		},
		{
			name: "other slices and maps are flattened",
			in: func(events ptrace.SpanEventSlice) {
				attrs := events.AppendEmpty().Attributes()
				mixed := attrs.PutEmptySlice("mixed")
				mixed.AppendEmpty().SetStr("one")
				mixed.AppendEmpty().SetInt(2)
				nested := attrs.PutEmptySlice("nested")
				inner := nested.AppendEmpty().SetEmptySlice()
				inner.AppendEmpty().SetStr("x")
				inner.AppendEmpty().SetStr("y")
				nested.AppendEmpty().SetEmptyMap().PutBool("flag", true)
				bytesSlice := attrs.PutEmptySlice("bytes")
				bytesSlice.AppendEmpty().SetEmptyBytes().FromRaw([]byte{0xde, 0xad})
				withEmpty := attrs.PutEmptySlice("with.empty")
				withEmpty.AppendEmpty().SetEmptySlice()
				withEmpty.AppendEmpty().SetStr("x")
				attrs.PutEmptyMap("empty.map")
				m := attrs.PutEmptyMap("map")
				m.PutStr("inner", "value")
				m.PutEmptyMap("deeper").PutInt("n", 7)
			},
			want: []*pb.SpanEvent{{Attributes: map[string]*pb.AttributeAnyValue{
				"mixed.0": str("one"),
				"mixed.1": {Type: pb.AttributeAnyValue_INT_VALUE, IntValue: 2},
				"nested.0": {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{
					{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: "x"},
					{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: "y"},
				}}},
				"nested.1.flag": {Type: pb.AttributeAnyValue_BOOL_VALUE, BoolValue: true},
				"bytes.0":       str("3q0="),
				"with.empty.0":  {Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: &pb.AttributeArray{}},
				"with.empty.1":  str("x"),
				"map.inner":     str("value"),
				"map.deeper.n":  {Type: pb.AttributeAnyValue_INT_VALUE, IntValue: 7},
			}}},
		},
		{
			name: "multiple events keep their order",
			in: func(events ptrace.SpanEventSlice) {
				events.AppendEmpty().SetName("first")
				events.AppendEmpty().SetName("second")
			},
			want: []*pb.SpanEvent{{Name: "first"}, {Name: "second"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := ptrace.NewSpanEventSlice()
			tt.in(events)
			assert.Equal(t, tt.want, OTelSpanEventsToDD(events))
		})
	}
}

// TestOTelSpanEventsToDDNonFinite: non-finite doubles, which the former JSON encoding had to
// redact, are kept as they are.
func TestOTelSpanEventsToDDNonFinite(t *testing.T) {
	events := ptrace.NewSpanEventSlice()
	attrs := events.AppendEmpty().Attributes()
	attrs.PutDouble("nan", math.NaN())
	attrs.PutDouble("inf", math.Inf(1))
	attrs.PutEmptySlice("nan.in.slice").AppendEmpty().SetDouble(math.NaN())

	got := OTelSpanEventsToDD(events)
	require.Len(t, got, 1)
	assert.True(t, math.IsNaN(got[0].Attributes["nan"].DoubleValue))
	assert.True(t, math.IsInf(got[0].Attributes["inf"].DoubleValue, 1))
	assert.True(t, math.IsNaN(got[0].Attributes["nan.in.slice"].ArrayValue.Values[0].DoubleValue))
}

// TestOTelSpanEventsToDDReadOnly asserts OTelSpanEventsToDD is safe on read-only pdata and
// doesn't modify the events.
func TestOTelSpanEventsToDDReadOnly(t *testing.T) {
	traces := buildReadOnlySafetyCorpus()
	want := OTelSpanEventsToDD(traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events())

	readOnly := buildReadOnlySafetyCorpus()
	readOnly.MarkReadOnly()
	events := readOnly.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events()

	var got []*pb.SpanEvent
	require.NotPanics(t, func() { got = OTelSpanEventsToDD(events) })
	require.Len(t, got, len(want))
	// The second event holds NaN values, which never compare equal.
	assert.Equal(t, want[0], got[0])
	assert.Equal(t, want[2:], got[2:])
	assert.Equal(t, uint32(5), events.At(0).DroppedAttributesCount())
	assert.Equal(t, uint32(1), events.At(2).DroppedAttributesCount())
}

func TestOTelSpanLinksToDD(t *testing.T) {
	traceID := pcommon.TraceID([16]byte{0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef})
	spanID := pcommon.SpanID([8]byte{0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89})
	newLink := func(links ptrace.SpanLinkSlice) ptrace.SpanLink {
		l := links.AppendEmpty()
		l.SetTraceID(traceID)
		l.SetSpanID(spanID)
		return l
	}
	baseLink := func() *pb.SpanLink {
		return &pb.SpanLink{TraceID: 0x0123456789abcdef, TraceIDHigh: 0xfedcba9876543210, SpanID: 0xabcdef0123456789}
	}

	for _, tt := range []struct {
		name string
		in   func(links ptrace.SpanLinkSlice)
		want func() []*pb.SpanLink
	}{
		{
			name: "ids only",
			in:   func(links ptrace.SpanLinkSlice) { newLink(links) },
			want: func() []*pb.SpanLink { return []*pb.SpanLink{baseLink()} },
		},
		{
			name: "tracestate with special characters",
			in: func(links ptrace.SpanLinkSlice) {
				newLink(links).TraceState().FromRaw(`dd=s:1;o:"rum",other=a\b`)
			},
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Tracestate = `dd=s:1;o:"rum",other=a\b`
				return []*pb.SpanLink{l}
			},
		},
		{
			name: "sampled flag",
			in:   func(links ptrace.SpanLinkSlice) { newLink(links).SetFlags(0x01) },
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Flags = 0x01 | 1<<31
				return []*pb.SpanLink{l}
			},
		},
		{
			// Bit 8 (has_is_remote) and bit 9 (is_remote) are OTLP-specific and dropped, but they
			// show the sender set the flags, so a zero trace flags byte means "not sampled".
			name: "unsampled flag with is_remote bits",
			in:   func(links ptrace.SpanLinkSlice) { newLink(links).SetFlags(0x300) },
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Flags = 1 << 31
				return []*pb.SpanLink{l}
			},
		},
		{
			name: "scalar attributes are stringified",
			in: func(links ptrace.SpanLinkSlice) {
				m := newLink(links).Attributes()
				m.PutStr("str", `say "hi" \ there`)
				m.PutInt("int", 42)
				m.PutDouble("double", 2.5)
				m.PutDouble("nan", math.NaN())
				m.PutBool("bool", true)
				m.PutEmptyBytes("bytes").FromRaw([]byte{0xde, 0xad})
				m.PutEmpty("empty")
			},
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Attributes = map[string]string{
					"str": `say "hi" \ there`, "int": "42", "double": "2.5", "nan": "NaN", "bool": "true", "bytes": "3q0=", "empty": "",
				}
				return []*pb.SpanLink{l}
			},
		},
		{
			name: "slices and maps are flattened",
			in: func(links ptrace.SpanLinkSlice) {
				m := newLink(links).Attributes()
				s := m.PutEmptySlice("slice")
				s.AppendEmpty().SetStr("a")
				s.AppendEmpty().SetInt(1)
				s.AppendEmpty().SetEmptySlice().AppendEmpty().SetBool(false)
				nested := m.PutEmptyMap("map")
				nested.PutStr("inner", "v")
				nested.PutEmptySlice("list").AppendEmpty().SetBool(true)
				m.PutEmptySlice("empty_slice")
				m.PutEmptyMap("empty_map")
			},
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Attributes = map[string]string{
					"slice.0": "a", "slice.1": "1", "slice.2.0": "false", "map.inner": "v", "map.list.0": "true",
				}
				return []*pb.SpanLink{l}
			},
		},
		{
			name: "multiple links",
			in: func(links ptrace.SpanLinkSlice) {
				newLink(links).Attributes().PutStr("k1", "v1")
				l := links.AppendEmpty()
				l.SetTraceID(pcommon.TraceID([16]byte{15: 1}))
				l.SetSpanID(pcommon.SpanID([8]byte{7: 2}))
				l.TraceState().FromRaw("dd=asdf256")
				l.SetDroppedAttributesCount(3)
			},
			want: func() []*pb.SpanLink {
				l := baseLink()
				l.Attributes = map[string]string{"k1": "v1"}
				return []*pb.SpanLink{l, {TraceID: 1, SpanID: 2, Tracestate: "dd=asdf256"}}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			links := ptrace.NewSpanLinkSlice()
			tt.in(links)
			assert.Equal(t, tt.want(), OTelSpanLinksToDD(links))
		})
	}
}

// TestOtelSpanToDDSpanSpanAttributePrecedence verifies that a span attribute wins over a resource
// attribute with the same key, including when the span value is numeric and stored in Metrics.
func TestOtelSpanToDDSpanSpanAttributePrecedence(t *testing.T) {
	for _, tt := range []struct {
		name        string
		setSpanAttr func(pcommon.Map)
		wantMeta    string
		wantMetric  float64
	}{
		{name: "string", setSpanAttr: func(m pcommon.Map) { m.PutStr("team", "infra") }, wantMeta: "infra"},
		{name: "int", setSpanAttr: func(m pcommon.Map) { m.PutInt("team", 7) }, wantMetric: 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			tt.setSpanAttr(span.Attributes())
			res := pcommon.NewResource()
			res.Attributes().PutStr("team", "core")
			cfg := &config.AgentConfig{Features: map[string]struct{}{}, OTLPReceiver: &config.OTLP{}}
			cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

			ddspan := OtelSpanToDDSpan(span, res, pcommon.NewInstrumentationScope(), cfg)

			if tt.wantMeta != "" {
				assert.Equal(t, tt.wantMeta, ddspan.Meta["team"])
			} else {
				assert.NotContains(t, ddspan.Meta, "team")
			}
			if tt.wantMetric != 0 {
				assert.Equal(t, tt.wantMetric, ddspan.Metrics["team"])
			} else {
				assert.NotContains(t, ddspan.Metrics, "team")
			}
		})
	}
}
