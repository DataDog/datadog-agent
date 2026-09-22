// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
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

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

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
			minSpan := OtelSpanToDDSpanMinimal(newSpan(), pcommon.NewResource(), lib, false, false, newCfg(), nil, nil)
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

				minSpan := OtelSpanToDDSpanMinimal(newSpan(), pcommon.NewResource(), lib, false, false, newCfg(), nil, nil)
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
		minSpan := OtelSpanToDDSpanMinimal(span, pcommon.NewResource(), lib, false, false, newCfg(), nil, nil)
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

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, []string{"team", "region"})

		assert.Equal(t, "checkout", minSpan.Meta["team"])
		assert.Equal(t, "us-east-1", minSpan.Meta["region"])
	})

	t.Run("span attribute takes precedence over resource attribute", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "span-team")
		res := pcommon.NewResource()
		res.Attributes().PutStr("team", "resource-team")

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, []string{"team"})

		assert.Equal(t, "span-team", minSpan.Meta["team"])
	})

	t.Run("unlisted keys are not copied and empty values are skipped", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "checkout")
		span.Attributes().PutStr("not-a-primary-tag", "ignored")
		span.Attributes().PutStr("empty", "")

		minSpan := OtelSpanToDDSpanMinimal(span, pcommon.NewResource(), lib, false, false, newCfg(), nil, []string{"team", "empty", "missing"})

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

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, []string{"team"})

		// An empty span value is treated as absent, so the resource value is used.
		assert.Equal(t, "platform", minSpan.Meta["team"])
	})

	t.Run("nil primaryTagKeys copies nothing", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.Attributes().PutStr("team", "checkout")

		minSpan := OtelSpanToDDSpanMinimal(span, pcommon.NewResource(), lib, false, false, newCfg(), nil, nil)

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

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, nil)

		// Hyphens aren't valid in span names; NormalizeName replaces them with underscores.
		assert.Equal(t, "invalid_op_name", minSpan.Name)
	})

	t.Run("negative duration is reset to zero", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		span.SetEndTimestamp(pcommon.Timestamp(int64(span.StartTimestamp()) - 1))
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, nil)

		assert.EqualValues(t, 0, minSpan.Duration)
	})

	t.Run("garbage start time is reset to now", func(t *testing.T) {
		minStart := time.Now().UnixNano()
		span := ptrace.NewSpan()
		span.SetStartTimestamp(42)
		span.SetEndTimestamp(pcommon.Timestamp(200000000))
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), nil, nil)

		assert.GreaterOrEqual(t, minSpan.Start, minStart-200000000)
		assert.LessOrEqual(t, minSpan.Start, time.Now().UnixNano())
	})

	t.Run("peer.service is truncated to the max service length", func(t *testing.T) {
		span := ptrace.NewSpan()
		longPeerSvc := strings.Repeat("a", 150)
		span.Attributes().PutStr("peer.service", longPeerSvc)
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), []string{"peer.service"}, nil)

		assert.Len(t, minSpan.Meta["peer.service"], normalizeutil.MaxServiceLen)
		assert.Equal(t, strings.Repeat("a", normalizeutil.MaxServiceLen), minSpan.Meta["peer.service"])
	})

	t.Run("_dd.base_service is truncated to the max service length", func(t *testing.T) {
		span := ptrace.NewSpan()
		longBaseSvc := strings.Repeat("a", 150)
		span.Attributes().PutStr("_dd.base_service", longBaseSvc)
		res := pcommon.NewResource()

		minSpan := OtelSpanToDDSpanMinimal(span, res, lib, false, false, newCfg(), []string{"_dd.base_service"}, nil)

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

// TestOtelSpanToDDSpan_AWSIdentityAttributeMappings verifies that OpenTelemetry
// attributes carrying AWS resource identity are aliased to the tag names the
// Datadog tracers and Agent tagger use, while the OpenTelemetry keys are kept.
func TestOtelSpanToDDSpan_AWSIdentityAttributeMappings(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	const (
		lambdaARN  = "arn:aws:lambda:us-east-1:123456789012:function:orders"
		taskARN    = "arn:aws:ecs:us-east-1:123456789012:task/orders-cluster/0123456789abcdef0123456789abcdef"
		clusterARN = "arn:aws:ecs:us-east-1:123456789012:cluster/orders-cluster"
		eksARN     = "arn:aws:eks:us-east-1:123456789012:cluster/OrdersCluster"
	)

	tests := []struct {
		name         string
		sattrs       map[string]any
		rattrs       map[string]any
		expectedMeta map[string]string
		absentMeta   []string
	}{
		{
			name: "resource identity attributes are aliased and the otel keys are kept",
			rattrs: map[string]any{
				string(semconv127.CloudProviderKey):    semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey):   "123456789012",
				string(semconv127.CloudRegionKey):      "us-east-1",
				string(semconv127.AWSECSTaskARNKey):    taskARN,
				string(semconv127.AWSECSClusterARNKey): clusterARN,
				string(semconv127.AWSEKSClusterARNKey): eksARN,
			},
			expectedMeta: map[string]string{
				"aws_account":     "123456789012",
				"region":          "us-east-1",
				"task_arn":        taskARN,
				"cluster_arn":     clusterARN,
				"eks_cluster_arn": eksARN,
				// OpenTelemetry keys are preserved.
				string(semconv127.CloudAccountIDKey):   "123456789012",
				string(semconv127.CloudRegionKey):      "us-east-1",
				string(semconv127.AWSECSTaskARNKey):    taskARN,
				string(semconv127.AWSECSClusterARNKey): clusterARN,
				string(semconv127.AWSEKSClusterARNKey): eksARN,
			},
		},
		{
			name: "lambda invoked arn is aliased to function_arn",
			sattrs: map[string]any{
				string(semconv127.AWSLambdaInvokedARNKey): lambdaARN,
			},
			expectedMeta: map[string]string{
				"function_arn": lambdaARN,
				string(semconv127.AWSLambdaInvokedARNKey): lambdaARN,
			},
		},
		{
			// aws.s3.bucket is deliberately NOT mapped: it is already a declared
			// precursor of peer.aws.s3.bucket alongside bucketname, so adding
			// bucketname would make the span carry both precursors and change its
			// APM stats aggregation keys without matching natively instrumented
			// spans, which carry bucketname alone.
			name: "s3 bucket is not mapped to bucketname",
			sattrs: map[string]any{
				string(semconv127.AWSS3BucketKey): "orders-archive",
			},
			expectedMeta: map[string]string{
				string(semconv127.AWSS3BucketKey): "orders-archive",
			},
			absentMeta: []string{"bucketname"},
		},
		{
			name: "single dynamodb table name is aliased to tablename",
			sattrs: map[string]any{
				string(semconv127.AWSDynamoDBTableNamesKey): []string{"orders"},
			},
			expectedMeta: map[string]string{
				"tablename": "orders",
			},
		},
		{
			name: "dynamodb table name as a plain string is aliased to tablename",
			sattrs: map[string]any{
				string(semconv127.AWSDynamoDBTableNamesKey): "orders",
			},
			expectedMeta: map[string]string{
				"tablename": "orders",
			},
		},
		{
			name: "multiple dynamodb table names have no single target",
			sattrs: map[string]any{
				string(semconv127.AWSDynamoDBTableNamesKey): []string{"orders", "customers"},
			},
			absentMeta: []string{"tablename"},
		},
		{
			name: "cloud.account.id is not aws_account for other providers",
			rattrs: map[string]any{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderGCP.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "my-gcp-project",
				string(semconv127.CloudRegionKey):    "us-central1",
			},
			expectedMeta: map[string]string{
				"region":                             "us-central1",
				string(semconv127.CloudAccountIDKey): "my-gcp-project",
			},
			absentMeta: []string{"aws_account"},
		},
		{
			name: "cloud.account.id is not aws_account without a provider",
			rattrs: map[string]any{
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			absentMeta: []string{"aws_account"},
		},
		{
			name: "span attributes take precedence over resource attributes",
			sattrs: map[string]any{
				string(semconv127.CloudRegionKey): "eu-west-1",
			},
			rattrs: map[string]any{
				string(semconv127.CloudRegionKey): "us-east-1",
			},
			expectedMeta: map[string]string{
				"region": "eu-west-1",
			},
		},
		{
			name: "an existing datadog tag is never overwritten",
			sattrs: map[string]any{
				"aws_account":                        "999999999999",
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account": "999999999999",
			},
		},
	}

	putAttrs := func(m pcommon.Map, attrs map[string]any) {
		for k, v := range attrs {
			switch val := v.(type) {
			case string:
				m.PutStr(k, val)
			case []string:
				s := m.PutEmptySlice(k)
				for _, e := range val {
					s.AppendEmpty().SetStr(e)
				}
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindClient)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			putAttrs(span.Attributes(), tt.sattrs)

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			putAttrs(res.Attributes(), tt.rattrs)

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
			for _, key := range tt.absentMeta {
				assert.NotContains(t, ddspan.Meta, key)
			}
		})
	}
}

// TestOtelSpanToDDSpan_AWSAccountProviderScopePrecedence verifies that the
// cloud.provider gate on aws_account is evaluated within the same attribute
// scope that supplies cloud.account.id. Resolving the provider and the account
// id independently lets a provider from one scope authorize an account id from
// another, so a non-AWS account id can be mislabeled as aws_account (or a real
// AWS account id can be dropped). The alias must only fire when the scope that
// wins the account id also declares cloud.provider=aws.
func TestOtelSpanToDDSpan_AWSAccountProviderScopePrecedence(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	tests := []struct {
		name         string
		sattrs       map[string]string
		rattrs       map[string]string
		expectedMeta map[string]string
		absentMeta   []string
	}{
		{
			// Bug case. The span marks itself AWS but carries no account id; the
			// resource is a different cloud (GCP) and carries a GCP project id.
			// Resolving provider and account independently makes the span provider
			// (aws, which wins by scope precedence) authorize the resource account
			// id (the GCP project), labeling a GCP project as aws_account. The gate
			// must be decided in the scope that supplies the account id: the GCP
			// resource scope, which is not AWS, so no aws_account is produced.
			name: "span aws provider must not authorize a gcp account id on the resource",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey): semconv127.CloudProviderAWS.Value.AsString(),
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderGCP.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "my-gcp-project",
			},
			absentMeta: []string{"aws_account"},
		},
		{
			// Split-scope recovery. cloud.provider=aws sits on the span and
			// cloud.account.id on the resource, each in a scope that carries only
			// one of the two. The account id's own scope declares no provider, so
			// the other scope's aws provider supplies it; no scope declares a
			// non-aws provider, so the account is proven AWS and aliased. (Dropping
			// it here would lose a legitimate AWS account on the conventional
			// provider-on-resource, id-on-span layout.)
			name: "aws provider on span and account id on resource are aliased",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey): semconv127.CloudProviderAWS.Value.AsString(),
			},
			rattrs: map[string]string{
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account": "123456789012",
			},
		},
		{
			// Guard: span declares an AWS provider and account together; the
			// resource carries a different (GCP) provider and account. The span
			// scope supplies both the provider and the account, so the span AWS
			// account is aliased and the GCP resource account is ignored.
			name: "span aws provider and account win over resource gcp provider and account",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "111111111111",
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderGCP.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "my-gcp-project",
			},
			expectedMeta: map[string]string{
				"aws_account": "111111111111",
			},
		},
		{
			// Guard: resource declares an AWS provider and account together; no
			// cloud attributes on the span. The resource scope supplies both, so
			// the AWS account is aliased from the resource scope.
			name: "resource aws provider and account are aliased when span has none",
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "222222222222",
			},
			expectedMeta: map[string]string{
				"aws_account": "222222222222",
			},
		},
		{
			// Guard: span declares an AWS provider and account; the resource
			// carries no cloud attributes. The span scope supplies both.
			name: "span aws provider and account are aliased when resource has none",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "333333333333",
			},
			expectedMeta: map[string]string{
				"aws_account": "333333333333",
			},
		},
		{
			// aws_account must agree with the span's own cloud.account.id. The span
			// supplies cloud.account.id (so that value is what the copy loop writes
			// to Meta) but no provider; the resource declares aws with a different
			// account id. Reading the account from the resource would put a value in
			// aws_account that contradicts the cloud.account.id on the same span.
			// The owning scope (span) is provider-less and the only provider in play
			// is the resource's aws, so the span account is the one aliased, keeping
			// the two tags consistent.
			name: "aws_account agrees with the span cloud.account.id it is aliased from",
			sattrs: map[string]string{
				string(semconv127.CloudAccountIDKey): "999999999999",
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account":      "999999999999",
				"cloud.account.id": "999999999999",
			},
		},
		{
			// A non-aws provider on the account id's own scope blocks the alias
			// outright. The span owns cloud.account.id and declares gcp, so the id
			// is a GCP identity; the aws provider on the resource cannot license it.
			name: "non-aws provider on the account scope blocks the alias",
			sattrs: map[string]string{
				string(semconv127.CloudAccountIDKey): "123456789012",
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderGCP.Value.AsString(),
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey): semconv127.CloudProviderAWS.Value.AsString(),
			},
			absentMeta: []string{"aws_account"},
		},
		{
			// An empty cloud.account.id on the span must not shadow a real account
			// id on the resource. The Meta copy loop skips the empty value, so the
			// owning scope is the resource; its aws provider licenses the account
			// and aws_account is aliased from it.
			name: "empty span account id does not shadow a real resource aws account",
			sattrs: map[string]string{
				string(semconv127.CloudAccountIDKey): "",
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account": "123456789012",
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
				span.Attributes().PutStr(k, v)
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
			for _, key := range tt.absentMeta {
				assert.NotContains(t, ddspan.Meta, key, "did not expect %s", key)
			}
		})
	}
}

// TestOtelSpanToDDSpan_ScopeAttributesDoNotOverwriteAWSIdentity verifies the
// precedence between instrumentation-scope attributes and the AWS identity tags.
//
// The V2 conversion copies scope attributes into Meta with an unconditional
// assignment, so a scope attribute that happens to use a Datadog identity tag
// name (region, aws_account, an ARN tag, ...) would replace the identity the
// span or resource actually carries, reporting one resource's telemetry under
// another resource's identity. The identity mapping therefore runs after the
// scope copy, and its write guard reads the producer's span and resource
// attributes rather than Meta, so that a scope value cannot pass for a producer
// tag and suppress the mapping.
//
// Scope attributes keep their normal effect everywhere else: a scope attribute
// under an identity tag name still lands when the span carries no AWS identity
// for that tag, and a scope attribute under any other name is untouched.
func TestOtelSpanToDDSpan_ScopeAttributesDoNotOverwriteAWSIdentity(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	const (
		taskARN    = "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123"
		clusterARN = "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
		eksARN     = "arn:aws:eks:us-east-1:123456789012:cluster/my-eks"
		lambdaARN  = "arn:aws:lambda:us-east-1:123456789012:function:my-fn"
	)

	tests := []struct {
		name         string
		sattrs       map[string]any
		rattrs       map[string]any
		scopeAttrs   map[string]string
		expectedMeta map[string]string
		absentMeta   []string
	}{
		{
			// Regression case: every identity tag is shadowed by a scope
			// attribute of the same name. The span and resource identity must win.
			name: "scope attributes must not overwrite identity from span or resource",
			sattrs: map[string]any{
				string(semconv127.AWSS3BucketKey):           "real-bucket",
				string(semconv127.AWSDynamoDBTableNamesKey): []string{"real-table"},
				string(semconv127.AWSLambdaInvokedARNKey):   lambdaARN,
				string(semconv127.AWSECSTaskARNKey):         taskARN,
			},
			rattrs: map[string]any{
				string(semconv127.CloudProviderKey):    semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey):   "123456789012",
				string(semconv127.CloudRegionKey):      "us-east-1",
				string(semconv127.AWSECSClusterARNKey): clusterARN,
				string(semconv127.AWSEKSClusterARNKey): eksARN,
			},
			scopeAttrs: map[string]string{
				"region":          "scope-region",
				"aws_account":     "000000000000",
				"bucketname":      "scope-bucket",
				"tablename":       "scope-table",
				"function_arn":    "arn:aws:lambda:us-east-1:000000000000:function:scope-fn",
				"task_arn":        "arn:aws:ecs:us-east-1:000000000000:task/scope/scope",
				"cluster_arn":     "arn:aws:ecs:us-east-1:000000000000:cluster/scope",
				"eks_cluster_arn": "arn:aws:eks:us-east-1:000000000000:cluster/scope",
			},
			expectedMeta: map[string]string{
				"region":          "us-east-1",
				"aws_account":     "123456789012",
				"tablename":       "real-table",
				"function_arn":    lambdaARN,
				"task_arn":        taskARN,
				"cluster_arn":     clusterARN,
				"eks_cluster_arn": eksARN,
				// aws.s3.bucket is not mapped, so the scope value stands for
				// bucketname, which is ordinary scope-attribute behavior.
				"bucketname": "scope-bucket",
				// The OpenTelemetry keys are still copied verbatim.
				string(semconv127.CloudRegionKey):      "us-east-1",
				string(semconv127.CloudAccountIDKey):   "123456789012",
				string(semconv127.AWSS3BucketKey):      "real-bucket",
				string(semconv127.AWSECSTaskARNKey):    taskARN,
				string(semconv127.AWSECSClusterARNKey): clusterARN,
				string(semconv127.AWSEKSClusterARNKey): eksARN,
			},
		},
		{
			// A scope attribute still supplies the tag when there is no AWS
			// identity to map, so the fix does not swallow scope attributes.
			name: "scope attributes supply identity tag names when no aws identity exists",
			scopeAttrs: map[string]string{
				"region":      "scope-region",
				"aws_account": "000000000000",
				"bucketname":  "scope-bucket",
			},
			expectedMeta: map[string]string{
				"region":      "scope-region",
				"aws_account": "000000000000",
				"bucketname":  "scope-bucket",
			},
		},
		{
			// Per-tag resolution: cloud.region is present so region comes from
			// the resource, while bucketname has no OpenTelemetry source on this
			// span and keeps the scope value.
			name: "mapped tags win per tag while unmapped tags keep the scope value",
			rattrs: map[string]any{
				string(semconv127.CloudRegionKey): "eu-west-1",
			},
			scopeAttrs: map[string]string{
				"region":     "scope-region",
				"bucketname": "scope-bucket",
			},
			expectedMeta: map[string]string{
				"region":     "eu-west-1",
				"bucketname": "scope-bucket",
			},
		},
		{
			// A producer tag set on the span attributes still wins over the
			// mapping, which is the pre-existing rule: ordering the mapping after
			// the scope copy must not turn it into an unconditional overwrite of
			// the producer's own tag.
			name: "producer tag on span attributes wins over the mapping",
			sattrs: map[string]any{
				"region":                          "producer-region",
				string(semconv127.CloudRegionKey): "us-east-1",
			},
			expectedMeta: map[string]string{
				"region":                          "producer-region",
				string(semconv127.CloudRegionKey): "us-east-1",
			},
		},
		{
			// Same rule for a producer tag on the resource attributes. The tag
			// name must not be a container tag name, since those are excluded
			// from the resource-side check for V1/V2 parity; tablename is not.
			name: "producer tag on resource attributes wins over the mapping",
			rattrs: map[string]any{
				"tablename": "producer-table",
				string(semconv127.AWSDynamoDBTableNamesKey): []string{"real-table"},
			},
			expectedMeta: map[string]string{
				"tablename": "producer-table",
			},
		},
		{
			// When the producer sets the Datadog tag itself, the mapping does not
			// fire, so the scope copy's value is what remains. That the scope copy
			// overwrites a producer attribute of the same name is pre-existing
			// behavior of the unconditional scope assignment and applies to every
			// attribute name, not just these: it is unchanged by this mapping
			// either way. What this case pins is that the mapping stays silent and
			// never substitutes the cloud.region value for the producer's own tag.
			name: "mapping stays silent when the producer sets the tag, scope copy unchanged",
			sattrs: map[string]any{
				"region":                          "producer-region",
				string(semconv127.CloudRegionKey): "us-east-1",
			},
			scopeAttrs: map[string]string{
				"region": "scope-region",
			},
			expectedMeta: map[string]string{
				"region":                          "scope-region",
				string(semconv127.CloudRegionKey): "us-east-1",
			},
		},
		{
			// Scope attributes that are not identity tag names are unaffected.
			name: "unrelated scope attributes are preserved",
			rattrs: map[string]any{
				string(semconv127.CloudRegionKey): "us-east-1",
			},
			scopeAttrs: map[string]string{
				"region":       "scope-region",
				"scope.custom": "scope-value",
			},
			expectedMeta: map[string]string{
				"region":       "us-east-1",
				"scope.custom": "scope-value",
			},
		},
		{
			// A numeric span attribute under a Datadog tag name is routed to
			// Metrics, not Meta, so it leaves the tag unset and must not suppress
			// the mapping. An account id is plausibly sent as an integer.
			name: "numeric span attribute does not suppress the mapping",
			sattrs: map[string]any{
				"aws_account": int64(999999999999),
			},
			rattrs: map[string]any{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account": "123456789012",
			},
		},
		{
			// A numeric resource attribute does reach Meta (the resource loop
			// copies every type as a string), so it counts as producer-set.
			name: "numeric resource attribute is producer-set and wins",
			rattrs: map[string]any{
				"aws_account":                        int64(999999999999),
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): "123456789012",
			},
			expectedMeta: map[string]string{
				"aws_account": "999999999999",
			},
		},
		{
			// An empty producer tag does not shadow the mapping, matching the
			// Meta copy loops, which fall through on an empty value.
			name: "empty producer tag does not shadow the mapping",
			sattrs: map[string]any{
				"region": "",
			},
			rattrs: map[string]any{
				string(semconv127.CloudRegionKey): "us-east-1",
			},
			expectedMeta: map[string]string{
				"region": "us-east-1",
			},
		},
	}

	putAttrs := func(m pcommon.Map, attrs map[string]any) {
		for k, v := range attrs {
			switch val := v.(type) {
			case string:
				m.PutStr(k, val)
			case int64:
				m.PutInt(k, val)
			case []string:
				s := m.PutEmptySlice(k)
				for _, e := range val {
					s.AppendEmpty().SetStr(e)
				}
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindClient)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			putAttrs(span.Attributes(), tt.sattrs)

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			putAttrs(res.Attributes(), tt.rattrs)

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")
			for k, v := range tt.scopeAttrs {
				lib.Attributes().PutStr(k, v)
			}

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
			for _, key := range tt.absentMeta {
				assert.NotContains(t, ddspan.Meta, key, "did not expect %s", key)
			}
		})
	}
}

// TestOtelSpanToDDSpanMinimal_AWSIdentityStatsParity verifies that the minimal
// conversion used by the APM stats path resolves the same AWS identity tags the
// full trace conversion does, for the keys stats can aggregate on.
//
// The stats path builds its spans from the minimal conversion, which copies only
// peer tags and additional metric tags out of the attributes. peer.aws.dynamodb.table
// has a single precursor, tablename, and no OpenTelemetry producer sets that key,
// so without the mapping an OTLP DynamoDB span resolves that peer tag on the
// trace path (whose span carries tablename) and not on the OTLP stats path. A
// tag that stats does not aggregate on is not added, so the minimal conversion
// stays minimal.
func TestOtelSpanToDDSpanMinimal_AWSIdentityStatsParity(t *testing.T) {
	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}

	const (
		accountID = "123456789012"
		region    = "us-east-1"
		taskARN   = "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123"
		bucket    = "real-bucket"
		table     = "real-table"
	)

	newSpan := func() ptrace.Span {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
		span.Attributes().PutStr(string(semconv127.AWSS3BucketKey), bucket)
		s := span.Attributes().PutEmptySlice(string(semconv127.AWSDynamoDBTableNamesKey))
		s.AppendEmpty().SetStr(table)
		span.Attributes().PutStr(string(semconv127.AWSLambdaInvokedARNKey), "arn:aws:lambda:us-east-1:123456789012:function:my-fn")
		return span
	}
	newRes := func() pcommon.Resource {
		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudProviderKey), semconv127.CloudProviderAWS.Value.AsString())
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), accountID)
		res.Attributes().PutStr(string(semconv127.CloudRegionKey), region)
		res.Attributes().PutStr(string(semconv127.AWSECSTaskARNKey), taskARN)
		return res
	}
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("test-lib")

	// The peer tag precursors the semantics registry lists for S3 and DynamoDB.
	// aws.s3.bucket is already a declared precursor, which is why it is not
	// mapped: peer.aws.s3.bucket resolves from it on both paths without help.
	peerTagKeys := []string{"bucketname", string(semconv127.AWSS3BucketKey), "tablename"}

	fullSpan := OtelSpanToDDSpan(newSpan(), newRes(), lib, newCfg())
	minSpan := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(), peerTagKeys, nil)

	// The peer tags agree between the trace span and the stats span.
	for _, key := range peerTagKeys {
		assert.Equal(t, fullSpan.Meta[key], minSpan.Meta[key], "trace and stats disagree on %s", key)
	}
	// tablename is the only precursor of peer.aws.dynamodb.table, and no
	// OpenTelemetry producer sets it, so the mapping is what makes that concept
	// resolve for stats at all.
	assert.Equal(t, table, minSpan.Meta["tablename"])
	// aws.s3.bucket resolves directly as its own precursor, on both paths.
	assert.Equal(t, bucket, minSpan.Meta[string(semconv127.AWSS3BucketKey)])
	// bucketname is not mapped, so neither path invents it: the two stay equal
	// and the span's S3 aggregation keys are unchanged by this PR.
	assert.NotContains(t, minSpan.Meta, "bucketname")
	assert.NotContains(t, fullSpan.Meta, "bucketname")

	// A tag stats does not aggregate on is not added to the minimal span.
	assert.NotContains(t, minSpan.Meta, "region")
	assert.NotContains(t, minSpan.Meta, "aws_account")
	assert.NotContains(t, minSpan.Meta, "function_arn")

	// Configured as additional metric tags, the identity tags DO participate in
	// stats aggregation, and then they must agree with the trace span too.
	primaryTagKeys := []string{"region", "task_arn"}
	minWithPrimary := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(), peerTagKeys, primaryTagKeys)
	for _, key := range primaryTagKeys {
		assert.Equal(t, fullSpan.Meta[key], minWithPrimary.Meta[key], "trace and stats disagree on %s", key)
	}
	assert.Equal(t, region, minWithPrimary.Meta["region"])
	assert.Equal(t, taskARN, minWithPrimary.Meta["task_arn"])

	// Requesting no keys maps nothing, so the default stats configuration is
	// unchanged by this PR.
	minNoKeys := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(), nil, nil)
	for _, key := range []string{"region", "aws_account", "task_arn", "bucketname", "tablename", "function_arn"} {
		assert.NotContains(t, minNoKeys.Meta, key)
	}
}

// TestOtelSpanToDDSpan_AWSIdentityDynamoDBSpanScopeOnly covers the scope
// rule for aws.dynamodb.table_names: it is read from the span scope only.
//
// The attribute describes the tables one call targeted, not the resource the
// process runs as, so a resource-scoped value does not describe any particular
// span. Borrowing it would give every sibling span on that resource, including
// spans that touch no table, a tablename tag and the peer.aws.dynamodb.table
// aggregation key for a table they never touched. The attribute is also an array
// projected onto a single tag, so only a value naming exactly one table is used.
func TestOtelSpanToDDSpan_AWSIdentityDynamoDBSpanScopeOnly(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	const resourceTable = "resource-table"

	tests := []struct {
		name          string
		spanValue     any
		withResource  bool
		expectedTable string
	}{
		{
			name:          "single element span slice is used",
			spanValue:     []string{"span-table"},
			withResource:  true,
			expectedTable: "span-table",
		},
		{
			name:          "plain single string is used",
			spanValue:     "orders",
			withResource:  false,
			expectedTable: "orders",
		},
		{
			// The resource scope is never consulted, so a span that does not
			// name a table does not inherit the resource's table.
			name:          "absent span value does not inherit the resource table",
			spanValue:     nil,
			withResource:  true,
			expectedTable: "",
		},
		{
			name:          "empty span slice does not inherit the resource table",
			spanValue:     []string{},
			withResource:  true,
			expectedTable: "",
		},
		{
			name:          "empty span string does not inherit the resource table",
			spanValue:     "",
			withResource:  true,
			expectedTable: "",
		},
		{
			// Naming two tables is positive evidence that the operation has no
			// single peer table.
			name:          "span slice naming two tables yields no table",
			spanValue:     []string{"a", "b"},
			withResource:  true,
			expectedTable: "",
		},
		{
			name:          "unusable span type yields no table",
			spanValue:     int64(7),
			withResource:  true,
			expectedTable: "",
		},
		{
			// A plain string that is a rendered list names more than one table.
			// Some instrumentations send the array this way, so it must not be
			// taken as a single table name verbatim.
			name:          "comma separated string names more than one table",
			spanValue:     "orders,customers",
			withResource:  false,
			expectedTable: "",
		},
		{
			name:          "bracket rendered slice names more than one table",
			spanValue:     "[orders customers]",
			withResource:  false,
			expectedTable: "",
		},
		{
			name:          "json rendered array names more than one table",
			spanValue:     `["orders","customers"]`,
			withResource:  false,
			expectedTable: "",
		},
		{
			// A single-element rendered array is still not accepted verbatim:
			// the brackets are not part of the table name.
			name:          "bracket rendered single element is not a table name",
			spanValue:     "[orders]",
			withResource:  false,
			expectedTable: "",
		},
		{
			name:          "slice with an empty element yields no table",
			spanValue:     []string{""},
			withResource:  true,
			expectedTable: "",
		},
		{
			// An instrumentation that renders the whole array into one element
			// names no single table either, so the list check applies to a
			// single-element slice as well as to a plain string.
			name:          "single element slice holding a rendered list yields no table",
			spanValue:     []string{"orders,customers"},
			withResource:  false,
			expectedTable: "",
		},
		{
			// A space-joined render names no single table either. A DynamoDB
			// table name cannot contain a space.
			name:          "space separated string names more than one table",
			spanValue:     "orders customers",
			withResource:  false,
			expectedTable: "",
		},
		{
			// A one-element numeric array stringifies to something that is not a
			// table name: a DynamoDB name is at least three characters of
			// letters, digits, underscore, hyphen and dot.
			name:          "single element numeric slice is not a table name",
			spanValue:     []int64{5},
			withResource:  false,
			expectedTable: "",
		},
		{
			name:          "too short a name is rejected",
			spanValue:     "ab",
			withResource:  false,
			expectedTable: "",
		},
		{
			name:          "a name using the full allowed charset is accepted",
			spanValue:     "orders_table-v2.prod",
			withResource:  false,
			expectedTable: "orders_table-v2.prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(ptrace.SpanKindClient)
			span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
			key := string(semconv127.AWSDynamoDBTableNamesKey)
			switch v := tt.spanValue.(type) {
			case string:
				span.Attributes().PutStr(key, v)
			case int64:
				span.Attributes().PutInt(key, v)
			case []string:
				s := span.Attributes().PutEmptySlice(key)
				for _, e := range v {
					s.AppendEmpty().SetStr(e)
				}
			case []int64:
				s := span.Attributes().PutEmptySlice(key)
				for _, e := range v {
					s.AppendEmpty().SetInt(e)
				}
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			if tt.withResource {
				s := res.Attributes().PutEmptySlice(key)
				s.AppendEmpty().SetStr(resourceTable)
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			if tt.expectedTable == "" {
				assert.NotContains(t, ddspan.Meta, "tablename")
			} else {
				assert.Equal(t, tt.expectedTable, ddspan.Meta["tablename"])
			}
		})
	}
}

// TestOtelSpanToDDSpan_AWSAccountEmptyProvider pins that an empty
// cloud.provider is treated as absent rather than as a conflicting provider.
// The attribute loops skip an empty value, so an empty cloud.provider asserts
// nothing about which cloud the account id belongs to: on its own it cannot
// prove the account is AWS, and it must not deny an aws provider declared by
// the other scope.
func TestOtelSpanToDDSpan_AWSAccountEmptyProvider(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	const accountID = "123456789012"

	tests := []struct {
		name         string
		sattrs       map[string]string
		rattrs       map[string]string
		expectedMeta map[string]string
		absentMeta   []string
	}{
		{
			name: "empty provider on the account scope proves nothing",
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  "",
				string(semconv127.CloudAccountIDKey): accountID,
			},
			absentMeta: []string{"aws_account"},
		},
		{
			name: "empty provider on the account scope does not deny an aws provider on the other scope",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey): semconv127.CloudProviderAWS.Value.AsString(),
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  "",
				string(semconv127.CloudAccountIDKey): accountID,
			},
			expectedMeta: map[string]string{"aws_account": accountID},
		},
		{
			name: "empty provider elsewhere does not deny an aws provider on the account scope",
			sattrs: map[string]string{
				string(semconv127.CloudProviderKey): "",
			},
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderAWS.Value.AsString(),
				string(semconv127.CloudAccountIDKey): accountID,
			},
			expectedMeta: map[string]string{"aws_account": accountID},
		},
		{
			name: "a real non-aws provider still denies the mapping",
			rattrs: map[string]string{
				string(semconv127.CloudProviderKey):  semconv127.CloudProviderGCP.Value.AsString(),
				string(semconv127.CloudAccountIDKey): accountID,
			},
			absentMeta: []string{"aws_account"},
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
				span.Attributes().PutStr(k, v)
			}

			res := pcommon.NewResource()
			res.Attributes().PutStr("service.name", "test-svc")
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

			for key, expectedVal := range tt.expectedMeta {
				assert.Equal(t, expectedVal, ddspan.Meta[key], "expected %s=%s", key, expectedVal)
			}
			for _, key := range tt.absentMeta {
				assert.NotContains(t, ddspan.Meta, key, "did not expect %s", key)
			}
		})
	}
}

// TestOtelSpanToDDSpan_AWSIdentityNumericSourceAttributes pins that a numeric
// OpenTelemetry source attribute on the span does not supply a mapped tag.
//
// Both receiver paths route numeric span attributes to Metrics and never to
// Meta, so a numeric cloud.account.id on the span leaves Meta's
// cloud.account.id coming from the resource. Reading the numeric span value
// would make aws_account and cloud.account.id disagree on one span, which is
// exactly the skew the scope selection exists to prevent: the two tags are
// documented as carrying the same identity.
func TestOtelSpanToDDSpan_AWSIdentityNumericSourceAttributes(t *testing.T) {
	cfg := &config.AgentConfig{}
	cfg.OTLPReceiver = &config.OTLP{}
	cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

	t.Run("numeric span account id does not shadow the resource account id", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
		span.Attributes().PutInt(string(semconv127.CloudAccountIDKey), 999999999999)

		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudProviderKey), semconv127.CloudProviderAWS.Value.AsString())
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), "123456789012")

		lib := pcommon.NewInstrumentationScope()
		lib.SetName("test-lib")

		ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

		// Meta's cloud.account.id comes from the resource, because the numeric
		// span value went to Metrics; aws_account must agree with it.
		assert.Equal(t, "123456789012", ddspan.Meta[string(semconv127.CloudAccountIDKey)])
		assert.Equal(t, "123456789012", ddspan.Meta["aws_account"])
		assert.Equal(t, float64(999999999999), ddspan.Metrics[string(semconv127.CloudAccountIDKey)])
	})

	t.Run("numeric span region does not shadow the resource region", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
		span.Attributes().PutInt(string(semconv127.CloudRegionKey), 42)

		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudRegionKey), "us-east-1")

		lib := pcommon.NewInstrumentationScope()
		lib.SetName("test-lib")

		ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

		assert.Equal(t, "us-east-1", ddspan.Meta[string(semconv127.CloudRegionKey)])
		assert.Equal(t, "us-east-1", ddspan.Meta["region"])
	})

	t.Run("numeric provider declares nothing and does not authorize an account", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})

		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutInt(string(semconv127.CloudProviderKey), 1)
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), "123456789012")

		lib := pcommon.NewInstrumentationScope()
		lib.SetName("test-lib")

		ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

		assert.NotContains(t, ddspan.Meta, "aws_account")
	})

	t.Run("differently cased provider is not the convention's aws", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})

		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudProviderKey), "AWS")
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), "123456789012")

		lib := pcommon.NewInstrumentationScope()
		lib.SetName("test-lib")

		ddspan := OtelSpanToDDSpan(span, res, lib, cfg)

		assert.NotContains(t, ddspan.Meta, "aws_account")
	})
}

// TestOtelSpanToDDSpan_AWSIdentityDisableFeature pins the rollback switch: with
// disable_otlp_aws_identity_tags set, no identity tag is added on the trace path
// or the stats path, so an operator can restore the previous span tags and APM
// stats aggregation keys without downgrading the Agent.
func TestOtelSpanToDDSpan_AWSIdentityDisableFeature(t *testing.T) {
	newCfg := func(disabled bool) *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		if disabled {
			cfg.Features = map[string]struct{}{"disable_otlp_aws_identity_tags": {}}
		}
		return cfg
	}

	newSpan := func() ptrace.Span {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
		span.Attributes().PutStr(string(semconv127.AWSDynamoDBTableNamesKey), "orders")
		return span
	}
	newRes := func() pcommon.Resource {
		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudProviderKey), semconv127.CloudProviderAWS.Value.AsString())
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), "123456789012")
		res.Attributes().PutStr(string(semconv127.CloudRegionKey), "us-east-1")
		return res
	}
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("test-lib")
	identityTags := []string{"region", "aws_account", "tablename"}

	enabled := OtelSpanToDDSpan(newSpan(), newRes(), lib, newCfg(false))
	for _, tag := range identityTags {
		assert.NotEmpty(t, enabled.Meta[tag], "%s should be present by default", tag)
	}

	disabled := OtelSpanToDDSpan(newSpan(), newRes(), lib, newCfg(true))
	for _, tag := range identityTags {
		assert.NotContains(t, disabled.Meta, tag, "%s should be absent when disabled", tag)
	}
	// The OpenTelemetry keys are untouched by the switch.
	assert.Equal(t, "us-east-1", disabled.Meta[string(semconv127.CloudRegionKey)])
	assert.Equal(t, "123456789012", disabled.Meta[string(semconv127.CloudAccountIDKey)])

	// Same on the stats path.
	peerTagKeys := []string{"tablename"}
	primaryTagKeys := []string{"region", "aws_account"}
	minEnabled := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(false), peerTagKeys, primaryTagKeys)
	minDisabled := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(true), peerTagKeys, primaryTagKeys)
	for _, tag := range identityTags {
		assert.NotEmpty(t, minEnabled.Meta[tag], "%s should be present by default", tag)
		assert.NotContains(t, minDisabled.Meta, tag, "%s should be absent when disabled", tag)
	}
}

// TestOtelSpanToDDSpanMinimal_AWSIdentityRestrictedSelection exercises the
// restricted selection used by the stats path for every tag the mapping can
// produce, so that the tag-bit mask cannot silently fail to select one.
func TestOtelSpanToDDSpanMinimal_AWSIdentityRestrictedSelection(t *testing.T) {
	newCfg := func() *config.AgentConfig {
		cfg := &config.AgentConfig{}
		cfg.OTLPReceiver = &config.OTLP{}
		cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
		return cfg
	}

	const (
		accountID  = "123456789012"
		region     = "us-east-1"
		lambdaARN  = "arn:aws:lambda:us-east-1:123456789012:function:my-fn"
		taskARN    = "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123"
		clusterARN = "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
		eksARN     = "arn:aws:eks:us-east-1:123456789012:cluster/my-eks"
		table      = "orders"
	)

	newSpan := func() ptrace.Span {
		span := ptrace.NewSpan()
		span.SetName("test-span")
		span.SetKind(ptrace.SpanKindClient)
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
		span.Attributes().PutStr(string(semconv127.AWSLambdaInvokedARNKey), lambdaARN)
		span.Attributes().PutStr(string(semconv127.AWSECSTaskARNKey), taskARN)
		span.Attributes().PutStr(string(semconv127.AWSDynamoDBTableNamesKey), table)
		return span
	}
	newRes := func() pcommon.Resource {
		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "test-svc")
		res.Attributes().PutStr(string(semconv127.CloudProviderKey), semconv127.CloudProviderAWS.Value.AsString())
		res.Attributes().PutStr(string(semconv127.CloudAccountIDKey), accountID)
		res.Attributes().PutStr(string(semconv127.CloudRegionKey), region)
		res.Attributes().PutStr(string(semconv127.AWSECSClusterARNKey), clusterARN)
		res.Attributes().PutStr(string(semconv127.AWSEKSClusterARNKey), eksARN)
		return res
	}
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("test-lib")

	want := map[string]string{
		"region":          region,
		"aws_account":     accountID,
		"function_arn":    lambdaARN,
		"task_arn":        taskARN,
		"cluster_arn":     clusterARN,
		"eks_cluster_arn": eksARN,
		"tablename":       table,
	}

	for tag, expected := range want {
		t.Run(tag, func(t *testing.T) {
			// Requested on its own, each tag is selected and resolves to the
			// same value the full conversion produces.
			minSpan := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(), []string{tag}, nil)
			assert.Equal(t, expected, minSpan.Meta[tag])

			fullSpan := OtelSpanToDDSpan(newSpan(), newRes(), lib, newCfg())
			assert.Equal(t, fullSpan.Meta[tag], minSpan.Meta[tag], "trace and stats disagree on %s", tag)

			// Requesting only this tag selects nothing else.
			for other := range want {
				if other != tag {
					assert.NotContains(t, minSpan.Meta, other)
				}
			}
		})
	}

	// A requested key that is not a mapping target selects nothing.
	minSpan := OtelSpanToDDSpanMinimal(newSpan(), newRes(), lib, false, false, newCfg(), []string{"peer.service", "db.name"}, nil)
	for tag := range want {
		assert.NotContains(t, minSpan.Meta, tag)
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
