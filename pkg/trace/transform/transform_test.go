// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
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
		{
			name: "only in span, only semconv117.HTTPStatusCodeKey",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
			},
			expected: 200,
		},
		{
			name: "only in span, both semconv117.HTTPStatusCodeKey and http.response.status_code, semconv117.HTTPStatusCodeKey wins",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
				"http.response.status_code":          201,
			},
			expected: 200,
		},
		{
			name: "only in resource, only semconv117.HTTPStatusCodeKey",
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 201,
			},
			expected: 201,
		},
		{
			name: "only in resource, both semconv117.HTTPStatusCodeKey and http.response.status_code, semconv117.HTTPStatusCodeKey wins",
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 201,
				"http.response.status_code":          202,
			},
			expected: 201,
		},
		{
			name:     "both set (span wins)",
			sattrs:   map[string]uint32{string(semconv117.HTTPStatusCodeKey): 203},
			rattrs:   map[string]uint32{string(semconv117.HTTPStatusCodeKey): 204},
			expected: 203,
		},
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

func TestGetOTelGRPCStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]any
		rattrs   map[string]any
		expected string
	}{
		{
			name:     "neither set",
			expected: "",
		},
		{
			name:     "only in span, rpc.grpc.status_code as int",
			sattrs:   map[string]any{"rpc.grpc.status_code": int64(0)},
			expected: "0",
		},
		{
			name:     "only in span, rpc.grpc.status_code as string",
			sattrs:   map[string]any{"rpc.grpc.status_code": "2"},
			expected: "2",
		},
		{
			name:     "only in resource, rpc.grpc.status_code",
			rattrs:   map[string]any{"rpc.grpc.status_code": int64(3)},
			expected: "3",
		},
		{
			name:     "both set (span wins)",
			sattrs:   map[string]any{"rpc.grpc.status_code": int64(4)},
			rattrs:   map[string]any{"rpc.grpc.status_code": int64(5)},
			expected: "4",
		},
		{
			name:     "grpc.code fallback",
			sattrs:   map[string]any{"grpc.code": int64(6)},
			expected: "6",
		},
		{
			name:     "rpc.grpc.status.code fallback",
			sattrs:   map[string]any{"rpc.grpc.status.code": int64(7)},
			expected: "7",
		},
		{
			name:     "grpc.status.code fallback",
			sattrs:   map[string]any{"grpc.status.code": int64(8)},
			expected: "8",
		},
		{
			name:     "rpc.response.status_code with rpc.system=grpc",
			sattrs:   map[string]any{"rpc.system": "grpc", "rpc.response.status_code": "DEADLINE_EXCEEDED"},
			expected: "DEADLINE_EXCEEDED",
		},
		{
			name:     "rpc.response.status_code with rpc.system.name=grpc",
			sattrs:   map[string]any{"rpc.system.name": "grpc", "rpc.response.status_code": int64(4)},
			expected: "4",
		},
		{
			name:     "rpc.response.status_code ignored without rpc.system",
			sattrs:   map[string]any{"rpc.response.status_code": "DEADLINE_EXCEEDED"},
			expected: "",
		},
		{
			name:     "rpc.response.status_code ignored for non-grpc system",
			sattrs:   map[string]any{"rpc.system": "jsonrpc", "rpc.response.status_code": "DEADLINE_EXCEEDED"},
			expected: "",
		},
		{
			name:     "rpc.response.status_code ignored for non-grpc system.name",
			sattrs:   map[string]any{"rpc.system.name": "jsonrpc", "rpc.response.status_code": "DEADLINE_EXCEEDED"},
			expected: "",
		},
		{
			name:     "rpc.grpc.status_code takes precedence over rpc.response.status_code",
			sattrs:   map[string]any{"rpc.system": "grpc", "rpc.grpc.status_code": int64(2), "rpc.response.status_code": "DEADLINE_EXCEEDED"},
			expected: "2",
		},
		{
			name:     "grpc.status_code works for jsonrpc system",
			sattrs:   map[string]any{"rpc.system": "jsonrpc", "grpc.status_code": int64(2)},
			expected: "2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				switch val := v.(type) {
				case int64:
					span.Attributes().PutInt(k, val)
				case string:
					span.Attributes().PutStr(k, val)
				}
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				switch val := v.(type) {
				case int64:
					res.Attributes().PutInt(k, val)
				case string:
					res.Attributes().PutStr(k, val)
				}
			}
			assert.Equal(t, tt.expected, GetOTelGRPCStatusCode(span, res))
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
			name:         "db.namespace in both, resource takes precedence",
			sattrs:       map[string]string{string(semconv127.DBNamespaceKey): "span-db"},
			rattrs:       map[string]string{string(semconv127.DBNamespaceKey): "resource-db"},
			expectedName: "resource-db",
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
