package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func TestRegisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := GetAggregator()
	err := agg.registerSender(1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	err = agg.registerSender(2)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	// Already registered sender => error
	err = agg.registerSender(2)
	assert.NotNil(t, err)
}

func TestDeregisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := GetAggregator()
	agg.registerSender(1)
	agg.registerSender(2)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	agg.deregisterSender(1)
	if assert.Len(t, aggregatorInstance.checkSamplers, 1) {
		_, ok := agg.checkSamplers[1]
		assert.False(t, ok)
		_, ok = agg.checkSamplers[2]
		assert.True(t, ok)
	}
}
