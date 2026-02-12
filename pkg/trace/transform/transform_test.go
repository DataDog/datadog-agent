// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"testing"

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
		{
			name: "http.response.status_code in span beats http.status_code in resource",
			sattrs: map[string]uint32{
				"http.response.status_code": 201,
			},
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 500,
			},
			expected: 201,
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
		{
			name: "span semconv117 takes precedence over resource semconv127",
			sattrs: map[string]string{
				string(semconv117.DeploymentEnvironmentKey): "span-env-117",
			},
			rattrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "res-env-127",
			},
			expected: "span-env-117",
		},
		{
			name: "mixed: span has semconv117, resource has both versions",
			sattrs: map[string]string{
				string(semconv117.DeploymentEnvironmentKey): "span-117",
			},
			rattrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "res-127",
				string(semconv117.DeploymentEnvironmentKey):     "res-117",
			},
			expected: "span-117",
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
		{
			name: "both old and new HTTP method - old takes precedence for http.method",
			sattrs: map[string]interface{}{
				"http.method":         "POST",
				"http.request.method": "GET",
			},
			expectedMeta: map[string]string{
				"http.method": "POST",
			},
		},
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
// This is potentially inconsistent with HTTPMappings where http.response.status_code -> http.status_code.
func TestFallbackInconsistency_HTTPStatusCodePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]uint32
		rattrs   map[string]uint32
		expected uint32
		note     string
	}{
		{
			name: "CURRENT: old http.status_code takes precedence over new http.response.status_code in span",
			sattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 200,
				"http.response.status_code":          500,
			},
			expected: 200,
			note:     "Old convention wins when both are in span. May want new convention to win.",
		},
		{
			name: "CURRENT: http.status_code in span wins over http.response.status_code in span",
			sattrs: map[string]uint32{
				"http.status_code":          201,
				"http.response.status_code": 404,
			},
			expected: 201,
			note:     "In GetOTelStatusCode, http.status_code is checked before http.response.status_code.",
		},
		{
			name: "CURRENT: span http.response.status_code wins over resource http.status_code",
			sattrs: map[string]uint32{
				"http.response.status_code": 201,
			},
			rattrs: map[string]uint32{
				string(semconv117.HTTPStatusCodeKey): 500,
			},
			expected: 201,
			note:     "Span attributes take precedence over resource attributes.",
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
			actual := GetOTelStatusCode(span, res)
			assert.Equal(t, tt.expected, actual, "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_DBNamespacePrecedence documents that db.namespace lookup
// uses GetOTelAttrValInResAndSpanAttrs which has RESOURCE precedence (opposite of most other lookups).
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
			name: "CURRENT: resource db.namespace takes precedence over span db.namespace",
			sattrs: map[string]string{
				string(semconv127.DBNamespaceKey): "span-db",
			},
			rattrs: map[string]string{
				string(semconv127.DBNamespaceKey): "resource-db",
			},
			expectedName: "resource-db",
			note:         "Uses GetOTelAttrValInResAndSpanAttrs which has resource-first precedence (inconsistent with most other lookups).",
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
// checks http.response.status_code BEFORE http.status_code (opposite of GetOTelStatusCode).
func TestFallbackInconsistency_Status2ErrorHTTPCodePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		meta        map[string]string
		expectedMsg string
		note        string
	}{
		{
			name: "CURRENT: http.response.status_code checked before http.status_code in Status2Error",
			meta: map[string]string{
				"http.response.status_code": "500",
				"http.status_code":          "200",
			},
			expectedMsg: "500 Internal Server Error",
			note:        "Status2Error uses http.response.status_code first - OPPOSITE of GetOTelStatusCode!",
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
