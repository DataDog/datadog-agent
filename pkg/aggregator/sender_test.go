package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func resetAggregator() {
	_aggregator = nil
	_sender = nil
}

func TestGetDefaultSenderCreatesOneSender(t *testing.T) {
	resetAggregator()

	defaultSender1 := GetDefaultSender().(*checkSender)
	assert.Len(t, _aggregator.checkSamplers, 1)

	defaultSender2 := GetDefaultSender().(*checkSender)
	assert.Len(t, _aggregator.checkSamplers, 1)
	assert.Equal(t, defaultSender1.checkSamplerID, defaultSender2.checkSamplerID)
}

func TestGetSenderCreatesDifferentCheckSamplers(t *testing.T) {
	resetAggregator()

	sender1 := GetSender().(*checkSender)
	assert.Len(t, _aggregator.checkSamplers, 1)

	sender2 := GetSender().(*checkSender)
	assert.Len(t, _aggregator.checkSamplers, 2)
	assert.NotEqual(t, sender1.checkSamplerID, sender2.checkSamplerID)

	defaultSender := GetDefaultSender().(*checkSender)
	assert.Len(t, _aggregator.checkSamplers, 3)
	assert.NotEqual(t, sender1.checkSamplerID, defaultSender.checkSamplerID)
	assert.NotEqual(t, sender2.checkSamplerID, defaultSender.checkSamplerID)
}

func TestSenderInterface(t *testing.T) {
	senderSampleChan := make(chan senderSample, 10)
	checkSender := newCheckSender(1, senderSampleChan)
	checkSender.Gauge("my.metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Rate("my.rate_metric", 2.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Commit()

	gaugeSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, gaugeSenderSample.checkSamplerID)
	assert.Equal(t, false, gaugeSenderSample.commit)

	rateSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, rateSenderSample.checkSamplerID)
	assert.Equal(t, false, rateSenderSample.commit)

	commitSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, commitSenderSample.checkSamplerID)
	assert.Equal(t, true, commitSenderSample.commit)
}
