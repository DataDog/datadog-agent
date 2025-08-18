package rum

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.5.0"
	"go.uber.org/zap"
)

func TestToTraces(t *testing.T) {
	tests := []struct {
		name        string
		payload     map[string]any
		expectError bool
		validate    func(t *testing.T, traces ptrace.Traces)
	}{
		{
			name: "successful trace conversion",
			payload: map[string]any{
				"type": "action",
				"date": 1640995200000.0,
				"_dd": map[string]any{
					"trace_id": "16976667969123787577",
					"span_id":  "2791337267577444227",
				},
				"resource": map[string]any{
					"duration": 10500000.0,
				},
				"service": "test-service",
				"version": "1.0.0",
				"usr": map[string]any{
					"email": "test@test.com",
				},
				"test-not-mapped-attribute": "test-value",
			},
			validate: func(t *testing.T, traces ptrace.Traces) {
				assert.Equal(t, 1, traces.ResourceSpans().Len())
				rs := traces.ResourceSpans().At(0)
				assert.Equal(t, semconv.SchemaURL, rs.SchemaUrl())

				assert.Equal(t, 1, rs.ScopeSpans().Len())
				scopeSpans := rs.ScopeSpans().At(0)
				assert.Equal(t, InstrumentationScopeName, scopeSpans.Scope().Name())

				assert.Equal(t, 1, scopeSpans.Spans().Len())
				span := scopeSpans.Spans().At(0)
				assert.Equal(t, "datadog.rum.action", span.Name())

				// Check trace and span IDs
				expectedTraceID := pcommon.TraceID([16]byte{0x0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xeb, 0x99, 0x3d, 0x92, 0x59, 0x60, 0x23, 0x39})
				expectedSpanID := pcommon.SpanID([8]byte{0x26, 0xbc, 0xd2, 0x74, 0xaa, 0x43, 0x57, 0x83})
				assert.Equal(t, expectedTraceID, span.TraceID())
				assert.Equal(t, expectedSpanID, span.SpanID())

				// Check timestamps
				expectedStartTime := pcommon.Timestamp(1640995200000000000)
				expectedEndTime := pcommon.Timestamp(1640995200010500000)
				assert.Equal(t, expectedStartTime, span.StartTimestamp())
				assert.Equal(t, expectedEndTime, span.EndTimestamp())

				// Check attributes
				attributes := span.Attributes()
				serviceName, _ := attributes.Get("service.name")
				assert.Equal(t, "test-service", serviceName.AsString())
				serviceVersion, _ := attributes.Get("service.version")
				assert.Equal(t, "1.0.0", serviceVersion.AsString())
				usrEmail, _ := attributes.Get("user.email")
				assert.Equal(t, "test@test.com", usrEmail.AsString())
				testNotMappedAttribute, _ := attributes.Get("datadog.test-not-mapped-attribute")
				assert.Equal(t, "test-value", testNotMappedAttribute.AsString())
				testTraceID, _ := attributes.Get("datadog._dd.trace_id")
				assert.Equal(t, "16976667969123787577", testTraceID.AsString())
				testSpanID, _ := attributes.Get("datadog._dd.span_id")
				assert.Equal(t, "2791337267577444227", testSpanID.AsString())
			},
		},
		{
			name: "missing trace_id in _dd",
			payload: map[string]any{
				"type": "action",
				"date": 1640995200000.0,
				"_dd": map[string]any{
					"span_id": "12345678901234567890",
				},
			},
			expectError: true,
		},
		{
			name: "invalid trace_id format",
			payload: map[string]any{
				"type": "action",
				"date": 1640995200000.0,
				"_dd": map[string]any{
					"trace_id": "invalid_trace_id",
					"span_id":  "12345678901234567890",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			req := &http.Request{
				Header: http.Header{
					"Content-Type": []string{"application/json; charset=utf-8"},
				},
				URL: &url.URL{
					RawQuery: "ddforward=test-browser",
				},
			}

			traces, err := ToTraces(logger, tt.payload, req)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, traces)

			if tt.validate != nil {
				tt.validate(t, traces)
			}
		})
	}
}

func TestSetDateForSpan(t *testing.T) {
	tests := []struct {
		name          string
		payload       map[string]any
		expectedStart pcommon.Timestamp
		expectedEnd   pcommon.Timestamp
	}{
		{
			name: "with date and duration",
			payload: map[string]any{
				"date": 1640995200000.0, // 2022-01-01 00:00:00 UTC
				"resource": map[string]any{
					"duration": 10500000.0,
				},
			},
			expectedStart: pcommon.Timestamp(1640995200000000000),
			expectedEnd:   pcommon.Timestamp(1640995200010500000),
		},
		{
			name: "with date only",
			payload: map[string]any{
				"date": 1640995200000.0,
			},
			expectedStart: pcommon.Timestamp(1640995200000000000),
			expectedEnd:   pcommon.Timestamp(1640995200000000000), // same as start
		},
		{
			name: "without date",
			payload: map[string]any{
				"resource": map[string]any{
					"duration": 10500000.0,
				},
			},
			expectedStart: pcommon.Timestamp(0),
			expectedEnd:   pcommon.Timestamp(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			setDateForSpan(tt.payload, span)

			assert.Equal(t, tt.expectedStart, span.StartTimestamp())
			assert.Equal(t, tt.expectedEnd, span.EndTimestamp())
		})
	}
}
