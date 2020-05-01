package interpreters

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSQLSpanInterpreter(t *testing.T) {
	sqlInterpreter := MakeSQLSpanInterpreter(config.DefaultInterpreterConfig())
	for _, tc := range []struct {
		testCase    string
		interpreter *SQLSpanInterpreter
		span        model.SpanWithMeta
		expected    pb.Span
	}{
		{
			testCase:    "Should set span.serviceType to 'database' when no db.type metadata exists",
			interpreter: sqlInterpreter,
			span: model.SpanWithMeta{
				Span: &pb.Span{
					Name:    "span-name",
					Service: "span-service",
					Meta: map[string]string{
						"span.serviceName": "span-service",
					},
				},
				SpanMetadata: &model.SpanMetadata{
					CreateTime: 1586441095,
					Hostname:   "hostname",
					PID:        10,
					Type:       "sql",
					Kind:       "some-kind",
				},
			},
			expected: pb.Span{
				Name:    "span-name",
				Service: "span-service",
				Meta: map[string]string{
					"span.serviceName": "span-service",
					"span.serviceType": "database",
				},
			},
		},
		{
			testCase:    "Should set span.serviceType to 'postgresql' when the db.type is 'postgresql'",
			interpreter: sqlInterpreter,
			span: model.SpanWithMeta{
				Span: &pb.Span{
					Name:    "span-name",
					Service: "span-service",
					Meta: map[string]string{
						"span.serviceName": "span-service",
						"db.type":          "postgresql",
					},
				},
				SpanMetadata: &model.SpanMetadata{
					CreateTime: 1586441095,
					Hostname:   "hostname",
					PID:        10,
					Type:       "sql",
					Kind:       "some-kind",
				},
			},
			expected: pb.Span{
				Name:    "span-name",
				Service: "span-service",
				Meta: map[string]string{
					"span.serviceName": "span-service",
					"db.type":          "postgresql",
					"span.serviceType": "postgresql",
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			actual := tc.interpreter.Interpret(&tc.span)
			assert.EqualValues(t, tc.expected, *actual)
		})
	}
}
