package event

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/assert"
)

type extractorTestCase struct {
	name                   string
	spans                  []*agent.WeightedSpan
	priority               sampler.SamplingPriority
	expectedExtractionRate float64
}

func testExtractor(t *testing.T, extractor Extractor, testCase extractorTestCase) {
	t.Run(testCase.name, func(t *testing.T) {
		assert := assert.New(t)

		total := 0

		for _, span := range testCase.spans {
			rate, ok := extractor.Extract(span, testCase.priority)

			total++

			if !ok {
				rate = -1
			}

			assert.EqualValues(testCase.expectedExtractionRate, rate)
		}
	})
}
