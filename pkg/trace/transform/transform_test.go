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
			sattrs:   map[string]string{attributes.DDNamespaceKeys.Env(): "env-span", string(semconv117.DeploymentEnvironmentKey): "env-span-semconv117"},
			rattrs:   map[string]string{attributes.DDNamespaceKeys.Env(): "env-res", string(semconv117.DeploymentEnvironmentKey): "env-res-semconv117"},
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
			sattrs:   map[string]string{attributes.DDNamespaceKeys.Host(): "test-host", string(semconv117.HostNameKey): "test-host-semconv117"},
			rattrs:   map[string]string{attributes.DDNamespaceKeys.Host(): "test-host", string(semconv117.HostNameKey): "test-host-semconv117"},
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
			sattrs:   map[string]string{attributes.DDNamespaceKeys.Version(): "v3", string(semconv127.ServiceVersionKey): "v3-semconv117"},
			rattrs:   map[string]string{attributes.DDNamespaceKeys.Version(): "v4", string(semconv127.ServiceVersionKey): "v4-semconv117"},
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
			sattrs:   map[string]string{attributes.DDNamespaceKeys.ContainerID(): "cid-span", string(semconv117.ContainerIDKey): "cid-span-semconv117"},
			rattrs:   map[string]string{attributes.DDNamespaceKeys.ContainerID(): "cid-res", string(semconv117.ContainerIDKey): "cid-res-semconv117"},
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
			sattrs:   map[string]uint32{attributes.DDNamespaceKeys.HTTPStatusCode(): 206, string(semconv117.HTTPStatusCodeKey): 210},
			rattrs:   map[string]uint32{attributes.DDNamespaceKeys.HTTPStatusCode(): 207, string(semconv117.HTTPStatusCodeKey): 211},
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

			ddspan, _ := OtelSpanToDDSpan(span, res, lib, cfg)

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

func TestGetErrorFieldsFromStatusAndEventsAndHTTPCode_HTTPStatusCodeMapping(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     ptrace.StatusCode
		statusMessage  string
		httpCode       int
		expectedErrMsg string
	}{
		// 4xx Client Errors
		{
			name:           "400 Bad Request",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       400,
			expectedErrMsg: "Bad Request",
		},
		{
			name:           "401 Unauthorized",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       401,
			expectedErrMsg: "Unauthorized",
		},
		{
			name:           "402 Payment Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       402,
			expectedErrMsg: "Payment Required",
		},
		{
			name:           "403 Forbidden",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       403,
			expectedErrMsg: "Forbidden",
		},
		{
			name:           "404 Not Found",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       404,
			expectedErrMsg: "Not Found",
		},
		{
			name:           "405 Method Not Allowed",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       405,
			expectedErrMsg: "Method Not Allowed",
		},
		{
			name:           "406 Not Acceptable",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       406,
			expectedErrMsg: "Not Acceptable",
		},
		{
			name:           "407 Proxy Authentication Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       407,
			expectedErrMsg: "Proxy Authentication Required",
		},
		{
			name:           "408 Request Timeout",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       408,
			expectedErrMsg: "Request Timeout",
		},
		{
			name:           "409 Conflict",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       409,
			expectedErrMsg: "Conflict",
		},
		{
			name:           "410 Gone",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       410,
			expectedErrMsg: "Gone",
		},
		{
			name:           "411 Length Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       411,
			expectedErrMsg: "Length Required",
		},
		{
			name:           "412 Precondition Failed",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       412,
			expectedErrMsg: "Precondition Failed",
		},
		{
			name:           "413 Request Entity Too Large",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       413,
			expectedErrMsg: "Request Entity Too Large",
		},
		{
			name:           "414 Request URI Too Long",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       414,
			expectedErrMsg: "Request URI Too Long",
		},
		{
			name:           "415 Unsupported Media Type",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       415,
			expectedErrMsg: "Unsupported Media Type",
		},
		{
			name:           "416 Requested Range Not Satisfiable",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       416,
			expectedErrMsg: "Requested Range Not Satisfiable",
		},
		{
			name:           "417 Expectation Failed",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       417,
			expectedErrMsg: "Expectation Failed",
		},
		{
			name:           "418 I'm a teapot",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       418,
			expectedErrMsg: "I'm a teapot",
		},
		{
			name:           "421 Misdirected Request",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       421,
			expectedErrMsg: "Misdirected Request",
		},
		{
			name:           "422 Unprocessable Entity",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       422,
			expectedErrMsg: "Unprocessable Entity",
		},
		{
			name:           "423 Locked",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       423,
			expectedErrMsg: "Locked",
		},
		{
			name:           "424 Failed Dependency",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       424,
			expectedErrMsg: "Failed Dependency",
		},
		{
			name:           "425 Too Early",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       425,
			expectedErrMsg: "Too Early",
		},
		{
			name:           "426 Upgrade Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       426,
			expectedErrMsg: "Upgrade Required",
		},
		{
			name:           "428 Precondition Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       428,
			expectedErrMsg: "Precondition Required",
		},
		{
			name:           "429 Too Many Requests",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       429,
			expectedErrMsg: "Too Many Requests",
		},
		{
			name:           "431 Request Header Fields Too Large",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       431,
			expectedErrMsg: "Request Header Fields Too Large",
		},
		{
			name:           "451 Unavailable For Legal Reasons",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       451,
			expectedErrMsg: "Unavailable For Legal Reasons",
		},
		// 5xx Server Errors
		{
			name:           "500 Internal Server Error",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       500,
			expectedErrMsg: "Internal Server Error",
		},
		{
			name:           "501 Not Implemented",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       501,
			expectedErrMsg: "Not Implemented",
		},
		{
			name:           "502 Bad Gateway",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       502,
			expectedErrMsg: "Bad Gateway",
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       503,
			expectedErrMsg: "Service Unavailable",
		},
		{
			name:           "504 Gateway Timeout",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       504,
			expectedErrMsg: "Gateway Timeout",
		},
		{
			name:           "505 HTTP Version Not Supported",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       505,
			expectedErrMsg: "HTTP Version Not Supported",
		},
		{
			name:           "506 Variant Also Negotiates",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       506,
			expectedErrMsg: "Variant Also Negotiates",
		},
		{
			name:           "507 Insufficient Storage",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       507,
			expectedErrMsg: "Insufficient Storage",
		},
		{
			name:           "508 Loop Detected",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       508,
			expectedErrMsg: "Loop Detected",
		},
		{
			name:           "510 Not Extended",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       510,
			expectedErrMsg: "Not Extended",
		},
		{
			name:           "511 Network Authentication Required",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       511,
			expectedErrMsg: "Network Authentication Required",
		},
		// Edge cases
		{
			name:           "status message takes precedence over http code",
			statusCode:     ptrace.StatusCodeError,
			statusMessage:  "Custom error message",
			httpCode:       404,
			expectedErrMsg: "Custom error message",
		},
		{
			name:           "unmapped code returns empty string",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       999,
			expectedErrMsg: "",
		},
		{
			name:           "zero http code",
			statusCode:     ptrace.StatusCodeError,
			httpCode:       0,
			expectedErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := ptrace.NewStatus()
			status.SetCode(tt.statusCode)
			status.SetMessage(tt.statusMessage)
			events := ptrace.NewSpanEventSlice()

			errCode, errMsg, _, _ := GetErrorFieldsFromStatusAndEventsAndHTTPCode(status, events, tt.httpCode)

			if tt.statusCode == ptrace.StatusCodeError {
				assert.Equal(t, int32(1), errCode)
			} else {
				assert.Equal(t, int32(0), errCode)
			}
			assert.Equal(t, tt.expectedErrMsg, errMsg)
		})
	}
}
