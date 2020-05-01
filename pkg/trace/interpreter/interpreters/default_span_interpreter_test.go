package interpreters

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDefaultSpanInterpreter(t *testing.T) {
	for _, tc := range []struct {
		testCase    string
		interpreter *DefaultSpanInterpreter
		span        pb.Span
		expected    pb.Span
	}{
		{
			testCase:    "Should not change the service name if there are no matching identifiers",
			interpreter: MakeDefaultSpanInterpreter(&config.Config{}),
			span:        pb.Span{Service: "SpanServiceName", Meta: map[string]string{"some.meta": "MetaValue"}},
			expected:    pb.Span{Service: "SpanServiceName", Meta: map[string]string{"span.serviceName": "SpanServiceName", "some.meta": "MetaValue", "span.serviceURN": "urn:service:/SpanServiceName"}},
		},
		{
			testCase:    "Should add 'db.instance' from metadata to the span.service when generating the span.serviceName",
			interpreter: MakeDefaultSpanInterpreter(config.DefaultInterpreterConfig()),
			span:        pb.Span{Service: "SpanServiceName", Meta: map[string]string{"db.instance": "Instance"}},
			expected:    pb.Span{Service: "SpanServiceName", Meta: map[string]string{"span.serviceName": "SpanServiceName:Instance", "db.instance": "Instance", "span.serviceURN": "urn:service:/SpanServiceName:Instance"}},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			actual := tc.interpreter.Interpret(&tc.span)
			assert.EqualValues(t, tc.expected, *actual)
		})
	}
}
