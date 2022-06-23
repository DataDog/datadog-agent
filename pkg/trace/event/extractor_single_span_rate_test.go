package event

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/assert"
)

func createSpanSamplingTestSpans(serviceName, operationName string, total int, mech, rate, limitRate float64) []*pb.Span {
	spans := make([]*pb.Span, total)
	for i := range spans {
		spans[i] = &pb.Span{TraceID: rand.Uint64(), Service: serviceName, Name: operationName, Metrics: map[string]float64{}}
		setSingleSpanTags(spans[i], mech, rate, limitRate)
	}
	return spans
}

func setSingleSpanTags(span *pb.Span, mech, rate, mps float64) {
	if span.Metrics == nil {
		span.Metrics = map[string]float64{}
	}
	span.Metrics[sampler.KeySpanSamplingMechanism] = mech
	span.Metrics[sampler.KeySpanSamplingRuleRate] = rate
	span.Metrics[sampler.KeySpanSamplingMPS] = mps
}

type sssExtractorTestCase struct {
	name                   string
	spans                  []*pb.Span
	rate                   float64
	mech                   float64
	expectedExtractionRate float64
}

func TestSingleSpanExtractor(t *testing.T) {
	tests := []sssExtractorTestCase{
		// Name: <mechanism>/<rate>/<expected rate/>)
		{name: "0/1/-1", rate: 1, mech: 0, expectedExtractionRate: 0},
		{name: "0/-1/-1", rate: -1, mech: 0, expectedExtractionRate: 0},
		{name: "8/-1/-1", rate: -1, mech: 8, expectedExtractionRate: 0},
		{name: "8/0/0", rate: 0, mech: 8, expectedExtractionRate: 0},
		{name: "8/0.5/1", rate: 0.5, mech: 8, expectedExtractionRate: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spans := createSpanSamplingTestSpans(test.name, "ops", 100, test.mech, test.rate, 100)
			test.spans = spans
			testSingleSpanExtractor(t, NewSingleSpanExtractor(), test)
		})
	}
}

func testSingleSpanExtractor(t *testing.T, extractor Extractor, testCase sssExtractorTestCase) {
	t.Run(testCase.name, func(t *testing.T) {
		assert := assert.New(t)
		extracted := 0
		for _, span := range testCase.spans {
			rate, ok := extractor.Extract(span, 0)
			if !ok {
				rate = 0
				continue
			}
			extracted++
			assert.Equal(testCase.expectedExtractionRate, rate)
		}
	})
}
