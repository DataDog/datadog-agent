package aggregator

import (
	// stdlib

	"sync"
	"testing"

	// 3p

	"github.com/stretchr/testify/assert"
)

func resetAggregator() {
	aggregatorInstance = nil
	aggregatorInit = sync.Once{}
	senderInstance = nil
	senderInit = sync.Once{}

	GetAggregator()
}

func TestGetDefaultSenderCreatesOneSender(t *testing.T) {
	resetAggregator()

	s, err := GetDefaultSender()
	assert.Nil(t, err)
	defaultSender1 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	s, err = GetDefaultSender()
	assert.Nil(t, err)
	defaultSender2 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)
	assert.Equal(t, defaultSender1.checkSamplerID, defaultSender2.checkSamplerID)
}

func TestGetSenderCreatesDifferentCheckSamplers(t *testing.T) {
	resetAggregator()

	s, err := GetSender()
	assert.Nil(t, err)
	sender1 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	s, err = GetSender()
	assert.Nil(t, err)
	sender2 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)
	assert.NotEqual(t, sender1.checkSamplerID, sender2.checkSamplerID)

	s, err = GetDefaultSender()
	assert.Nil(t, err)
	defaultSender := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 3)
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
