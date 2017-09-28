// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	// stdlib
	"sync"
	"testing"

	// 3p

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func resetAggregator() {
	aggregatorInstance = nil
	aggregatorInit = sync.Once{}
	senderInstance = nil
	senderInit = sync.Once{}
	senderPool = &checkSenderPool{
		senders: make(map[check.ID]Sender),
	}
}

func TestGetDefaultSenderReturnsSameSender(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "")

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
	InitAggregator(nil, "")

	s, err := GetSender(checkID1)
	assert.Nil(t, err)
	sender1 := s.(*checkSender)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	s, err = GetSender(checkID2)
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

func TestGetSenderWithSameIDsReturnsSameSender(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "")

	sender1, err := GetSender(checkID1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)
	assert.Len(t, senderPool.senders, 1)

	sender2, err := GetSender(checkID1)
	assert.Nil(t, err)
	assert.Equal(t, sender1, sender2)

	assert.Len(t, aggregatorInstance.checkSamplers, 1)
	assert.Len(t, senderPool.senders, 1)
}

func TestDestroySender(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "")

	_, err := GetSender(checkID1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	_, err = GetSender(checkID2)
	assert.Nil(t, err)

	assert.Len(t, aggregatorInstance.checkSamplers, 2)
	DestroySender(checkID1)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)
}

func TestGetAndSetSender(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	testCheckSender := newCheckSender(checkID1, senderMetricSampleChan, serviceCheckChan, eventChan)

	err := SetSender(testCheckSender, checkID1)
	assert.Nil(t, err)

	sender, err := GetSender(checkID1)
	assert.Nil(t, err)
	assert.Equal(t, testCheckSender, sender)

}

func TestCheckSenderInterface(t *testing.T) {
	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	checkSender := newCheckSender(checkID1, senderMetricSampleChan, serviceCheckChan, eventChan)
	checkSender.Gauge("my.metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Rate("my.rate_metric", 2.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Count("my.count_metric", 123.0, "my-hostname", []string{"foo", "bar"})
	checkSender.MonotonicCount("my.monotonic_count_metric", 12.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Counter("my.counter_metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Histogram("my.histo_metric", 3.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Commit()
	checkSender.ServiceCheck("my_service.can_connect", metrics.ServiceCheckOK, "my-hostname", []string{"foo", "bar"}, "message")
	submittedEvent := metrics.Event{
		Title:          "Something happened",
		Text:           "Description of the event",
		Ts:             12,
		Priority:       metrics.EventPriorityLow,
		Host:           "my-hostname",
		Tags:           []string{"foo", "bar"},
		AlertType:      metrics.EventAlertTypeInfo,
		AggregationKey: "event_agg_key",
		SourceTypeName: "docker",
	}
	checkSender.Event(submittedEvent)

	gaugeSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, gaugeSenderSample.id)
	assert.Equal(t, metrics.GaugeType, gaugeSenderSample.metricSample.Mtype)
	assert.Equal(t, false, gaugeSenderSample.commit)

	rateSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, rateSenderSample.id)
	assert.Equal(t, metrics.RateType, rateSenderSample.metricSample.Mtype)
	assert.Equal(t, false, rateSenderSample.commit)

	countSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, countSenderSample.id)
	assert.Equal(t, metrics.CountType, countSenderSample.metricSample.Mtype)
	assert.Equal(t, false, countSenderSample.commit)

	monotonicCountSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, monotonicCountSenderSample.id)
	assert.Equal(t, metrics.MonotonicCountType, monotonicCountSenderSample.metricSample.Mtype)
	assert.Equal(t, false, monotonicCountSenderSample.commit)

	CounterSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, CounterSenderSample.id)
	assert.Equal(t, metrics.CounterType, CounterSenderSample.metricSample.Mtype)
	assert.Equal(t, false, CounterSenderSample.commit)

	histoSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, histoSenderSample.id)
	assert.Equal(t, metrics.HistogramType, histoSenderSample.metricSample.Mtype)
	assert.Equal(t, false, histoSenderSample.commit)

	commitSenderSample := <-senderMetricSampleChan
	assert.EqualValues(t, checkID1, commitSenderSample.id)
	assert.Equal(t, true, commitSenderSample.commit)

	serviceCheck := <-serviceCheckChan
	assert.Equal(t, "my_service.can_connect", serviceCheck.CheckName)
	assert.Equal(t, metrics.ServiceCheckOK, serviceCheck.Status)
	assert.Equal(t, "my-hostname", serviceCheck.Host)
	assert.Equal(t, []string{"foo", "bar"}, serviceCheck.Tags)
	assert.Equal(t, "message", serviceCheck.Message)

	event := <-eventChan
	assert.Equal(t, submittedEvent, event)
}
