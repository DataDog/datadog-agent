package event

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func createTestSpans(serviceName string, operationName string) []*agent.WeightedSpan {
	spans := make([]*agent.WeightedSpan, 1000)
	for i := range spans {
		spans[i] = &agent.WeightedSpan{Span: &pb.Span{TraceID: rand.Uint64(), Service: serviceName, Name: operationName}}
	}
	return spans
}

func TestAnalyzedExtractor(t *testing.T) {
	config := make(map[string]map[string]float64)
	config["serviceA"] = make(map[string]float64)
	config["serviceA"]["opA"] = 0

	config["serviceB"] = make(map[string]float64)
	config["serviceB"]["opB"] = 0.5

	config["serviceC"] = make(map[string]float64)
	config["serviceC"]["opC"] = 1

	tests := []extractorTestCase{
		// Name: <priority>/(<no match reason>/<extraction rate>)
		{"none/noservice", createTestSpans("serviceZ", "opA"), 0, -1},
		{"none/noname", createTestSpans("serviceA", "opZ"), 0, -1},
		{"none/0", createTestSpans("serviceA", "opA"), 0, 0},
		{"none/0.5", createTestSpans("serviceB", "opB"), 0, 0.5},
		{"none/1", createTestSpans("serviceC", "opC"), 0, 1},
		{"1/noservice", createTestSpans("serviceZ", "opA"), 1, -1},
		{"1/noname", createTestSpans("serviceA", "opZ"), 1, -1},
		{"1/0", createTestSpans("serviceA", "opA"), 1, 0},
		{"1/0.5", createTestSpans("serviceB", "opB"), 1, 0.5},
		{"1/1", createTestSpans("serviceC", "opC"), 1, 1},
		{"2/noservice", createTestSpans("serviceZ", "opA"), 2, -1},
		{"2/noname", createTestSpans("serviceA", "opZ"), 2, -1},
		{"2/0", createTestSpans("serviceA", "opA"), 2, 0},
		{"2/0.5", createTestSpans("serviceB", "opB"), 2, 1},
		{"2/1", createTestSpans("serviceC", "opC"), 2, 1},
	}

	for _, test := range tests {
		testExtractor(t, NewFixedRateExtractor(config), test)
	}
}
