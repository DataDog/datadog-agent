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

func TestGetDefaultSenderReturnsSameSender(t *testing.T) {
	resetAggregator()

	s, err := GetDefaultSender()
	assert.Nil(t, err)
	defaultSender1 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	s, err = GetDefaultSender()
	assert.Nil(t, err)
	defaultSender2 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)
	assert.Equal(t, defaultSender1.id, defaultSender2.id)
}

func TestGetSenderWithDifferentIDsReturnsDifferentCheckSamplers(t *testing.T) {
	resetAggregator()

	s, err := GetSender(1)
	assert.Nil(t, err)
	sender1 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	s, err = GetSender(2)
	assert.Nil(t, err)
	sender2 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)
	assert.NotEqual(t, sender1.id, sender2.id)

	s, err = GetDefaultSender()
	assert.Nil(t, err)
	defaultSender := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 3)
	assert.NotEqual(t, sender1.id, defaultSender.id)
	assert.NotEqual(t, sender2.id, defaultSender.id)
}

func TestGetSenderWithSameIDsReturnsError(t *testing.T) {
	resetAggregator()

	_, err := GetSender(1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	_, err = GetSender(1)
	assert.NotNil(t, err)

	assert.Len(t, aggregatorInstance.checkSamplers, 1)
}

func TestDestroySender(t *testing.T) {
	resetAggregator()

	_, err := GetSender(1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	_, err = GetSender(2)
	assert.Nil(t, err)

	assert.Len(t, aggregatorInstance.checkSamplers, 2)
	DestroySender(1)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)
}

func TestSenderInterface(t *testing.T) {
	senderSampleChan := make(chan senderSample, 10)
	checkSender := newCheckSender(1, senderSampleChan)
	checkSender.Gauge("my.metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Rate("my.rate_metric", 2.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Commit()

	gaugeSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, gaugeSenderSample.id)
	assert.Equal(t, false, gaugeSenderSample.commit)

	rateSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, rateSenderSample.id)
	assert.Equal(t, false, rateSenderSample.commit)

	commitSenderSample := <-senderSampleChan
	assert.EqualValues(t, 1, commitSenderSample.id)
	assert.Equal(t, true, commitSenderSample.commit)
}
