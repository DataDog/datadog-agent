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

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestGetOTelEnv(t *testing.T) {
	tests := []struct {
		name                       string
		sattrs                     map[string]string
		rattrs                     map[string]string
		expected                   string
		ignoreMissingDatadogFields bool
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
		{
			name:                       "ignore missing datadog fields",
			sattrs:                     map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-span"},
			rattrs:                     map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-span"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:     "read from datadog fields",
			sattrs:   map[string]string{KeyDatadogEnvironment: "env-span", string(semconv117.DeploymentEnvironmentKey): "env-span-semconv117"},
			rattrs:   map[string]string{KeyDatadogEnvironment: "env-res", string(semconv117.DeploymentEnvironmentKey): "env-res-semconv117"},
			expected: "env-span",
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
			assert.Equal(t, tt.expected, GetOTelEnv(span, res, tt.ignoreMissingDatadogFields))
		})
	}
}

func TestGetOTelHostname(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		rattrs                     map[string]string
		sattrs                     map[string]string
		fallbackHost               string
		expected                   string
		ignoreMissingDatadogFields bool
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
			name:                       "ignore missing datadog fields",
			rattrs:                     map[string]string{string(semconv117.HostNameKey): "test-host"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:     "read from datadog fields",
			sattrs:   map[string]string{KeyDatadogHost: "test-host", string(semconv117.HostNameKey): "test-host-semconv117"},
			rattrs:   map[string]string{KeyDatadogHost: "test-host", string(semconv117.HostNameKey): "test-host-semconv117"},
			expected: "test-host",
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
			actual := GetOTelHostname(span, res, tr, tt.fallbackHost, tt.ignoreMissingDatadogFields)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelVersion(t *testing.T) {
	tests := []struct {
		name                       string
		sattrs                     map[string]string
		rattrs                     map[string]string
		expected                   string
		ignoreMissingDatadogFields bool
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
		{
			name:                       "ignore missing datadog fields",
			sattrs:                     map[string]string{string(semconv127.ServiceVersionKey): "v3"},
			rattrs:                     map[string]string{string(semconv127.ServiceVersionKey): "v4"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:     "read from datadog fields",
			sattrs:   map[string]string{KeyDatadogVersion: "v3", string(semconv127.ServiceVersionKey): "v3-semconv117"},
			rattrs:   map[string]string{KeyDatadogVersion: "v4", string(semconv127.ServiceVersionKey): "v4-semconv117"},
			expected: "v3",
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
			assert.Equal(t, tt.expected, GetOTelVersion(span, res, tt.ignoreMissingDatadogFields))
		})
	}
}

func TestGetOTelContainerID(t *testing.T) {
	tests := []struct {
		name                       string
		sattrs                     map[string]string
		rattrs                     map[string]string
		expected                   string
		ignoreMissingDatadogFields bool
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
		{
			name:                       "ignore missing datadog fields",
			sattrs:                     map[string]string{string(semconv117.ContainerIDKey): "cid-span"},
			rattrs:                     map[string]string{string(semconv117.ContainerIDKey): "cid-span"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:     "read from datadog fields",
			sattrs:   map[string]string{KeyDatadogContainerID: "cid-span", string(semconv117.ContainerIDKey): "cid-span-semconv117"},
			rattrs:   map[string]string{KeyDatadogContainerID: "cid-res", string(semconv117.ContainerIDKey): "cid-res-semconv117"},
			expected: "cid-span",
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
			assert.Equal(t, tt.expected, GetOTelContainerID(span, res, tt.ignoreMissingDatadogFields))
		})
	}
}

func TestGetOTelStatusCode(t *testing.T) {
	tests := []struct {
		name                       string
		sattrs                     map[string]uint32
		rattrs                     map[string]uint32
		expected                   uint32
		ignoreMissingDatadogFields bool
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
		{
			name:                       "ignore missing datadog fields",
			sattrs:                     map[string]uint32{string(semconv117.HTTPStatusCodeKey): 205},
			expected:                   0,
			ignoreMissingDatadogFields: true,
		},
		{
			name:     "read from datadog fields",
			sattrs:   map[string]uint32{KeyDatadogHTTPStatusCode: 206, string(semconv117.HTTPStatusCodeKey): 210},
			rattrs:   map[string]uint32{KeyDatadogHTTPStatusCode: 207, string(semconv117.HTTPStatusCodeKey): 211},
			expected: 206,
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
			assert.Equal(t, tt.expected, GetOTelStatusCode(span, res, tt.ignoreMissingDatadogFields))
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
