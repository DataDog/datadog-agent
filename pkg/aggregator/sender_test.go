// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	// stdlib
	"fmt"
	"sync"
	"testing"
	"time"

	// 3p

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func resetAggregator() {
	if aggregatorInstance != nil {
		aggregatorInstance.stopChan <- struct{}{}
	}
	recurrentSeries = metrics.Series{}
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
	InitAggregator(nil, "", "")

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
	InitAggregator(nil, "", "")

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
	InitAggregator(nil, "", "")

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
	InitAggregator(nil, "", "")

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
	InitAggregator(nil, "", "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	testCheckSender := newCheckSender(checkID1, "", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)

	err := SetSender(testCheckSender, checkID1)
	assert.Nil(t, err)

	sender, err := GetSender(checkID1)
	assert.Nil(t, err)
	assert.Equal(t, testCheckSender, sender)
}

func TestGetSenderDefaultHostname(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "testhostname", "")

	sender, err := GetSender(checkID1)
	require.NoError(t, err)

	checksender, ok := sender.(*checkSender)
	require.True(t, ok)

	assert.Equal(t, "testhostname", checksender.defaultHostname)
	assert.Equal(t, false, checksender.defaultHostnameDisabled)
}

func TestGetSenderAddCheckCustomTagsMetrics(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "testhostname", "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)

	// no custom tags
	checkSender.sendMetricSample("metric.test", 42.0, "testhostname", nil, metrics.CounterType)
	sms := <-senderMetricSampleChan
	assert.Nil(t, sms.metricSample.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	checkSender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType)
	sms = <-senderMetricSampleChan
	assert.Equal(t, checkTags, sms.metricSample.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	checkSender.SetCheckCustomTags(customTags)
	assert.Len(t, checkSender.checkTags, 2)

	// only tags coming from the configuration file
	checkSender.sendMetricSample("metric.test", 42.0, "testhostname", nil, metrics.CounterType)
	sms = <-senderMetricSampleChan
	assert.Equal(t, customTags, sms.metricSample.Tags)

	// tags added by the check + tags coming from the configuration file
	checkSender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType)
	sms = <-senderMetricSampleChan
	assert.Equal(t, append(checkTags, customTags...), sms.metricSample.Tags)
}

func TestGetSenderAddCheckCustomTagsService(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "testhostname", "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)

	// no custom tags
	checkSender.ServiceCheck("test", metrics.ServiceCheckOK, "testhostname", nil, "test message")
	sc := <-serviceCheckChan
	assert.Nil(t, sc.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	checkSender.ServiceCheck("test", metrics.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc = <-serviceCheckChan
	assert.Equal(t, checkTags, sc.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	checkSender.SetCheckCustomTags(customTags)
	assert.Len(t, checkSender.checkTags, 2)

	// only tags coming from the configuration file
	checkSender.ServiceCheck("test", metrics.ServiceCheckOK, "testhostname", nil, "test message")
	sc = <-serviceCheckChan
	assert.Equal(t, customTags, sc.Tags)

	// tags added by the check + tags coming from the configuration file
	checkSender.ServiceCheck("test", metrics.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc = <-serviceCheckChan
	assert.Equal(t, append(checkTags, customTags...), sc.Tags)
}

func TestGetSenderAddCheckCustomTagsEvent(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "testhostname", "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)

	event := metrics.Event{
		Title: "title",
		Host:  "testhostname",
		Ts:    time.Now().Unix(),
		Text:  "text",
		Tags:  nil,
	}

	// no custom tags
	checkSender.Event(event)
	e := <-eventChan
	assert.Nil(t, e.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	event.Tags = checkTags
	checkSender.Event(event)
	e = <-eventChan
	assert.Equal(t, checkTags, e.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	checkSender.SetCheckCustomTags(customTags)
	assert.Len(t, checkSender.checkTags, 2)

	// only tags coming from the configuration file
	event.Tags = nil
	checkSender.Event(event)
	e = <-eventChan
	assert.Equal(t, customTags, e.Tags)

	// tags added by the check + tags coming from the configuration file
	event.Tags = checkTags
	checkSender.Event(event)
	e = <-eventChan
	assert.Equal(t, append(checkTags, customTags...), e.Tags)
}

func TestGetSenderAddCheckCustomTagsHistogramBucket(t *testing.T) {
	resetAggregator()
	InitAggregator(nil, "testhostname", "")

	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)

	// no custom tags
	checkSender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", nil)
	bucketSample := <-bucketChan
	assert.Nil(t, bucketSample.bucket.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	checkSender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", checkTags)
	bucketSample = <-bucketChan
	assert.Equal(t, checkTags, bucketSample.bucket.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	checkSender.SetCheckCustomTags(customTags)
	assert.Len(t, checkSender.checkTags, 2)

	// only tags coming from the configuration file
	checkSender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", nil)
	bucketSample = <-bucketChan
	assert.Equal(t, customTags, bucketSample.bucket.Tags)

	// tags added by the check + tags coming from the configuration file
	checkSender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", checkTags)
	bucketSample = <-bucketChan
	assert.Equal(t, append(checkTags, customTags...), bucketSample.bucket.Tags)
}

func TestCheckSenderInterface(t *testing.T) {
	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "default-hostname", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)
	checkSender.Gauge("my.metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Rate("my.rate_metric", 2.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Count("my.count_metric", 123.0, "my-hostname", []string{"foo", "bar"})
	checkSender.MonotonicCount("my.monotonic_count_metric", 12.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Counter("my.counter_metric", 1.0, "my-hostname", []string{"foo", "bar"})
	checkSender.Histogram("my.histo_metric", 3.0, "my-hostname", []string{"foo", "bar"})
	checkSender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", []string{"foo", "bar"})
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
	assert.Equal(t, "my-hostname", gaugeSenderSample.metricSample.Host)
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

	histogramBucket := <-bucketChan
	assert.Equal(t, "my.histogram_bucket", histogramBucket.bucket.Name)
	assert.Equal(t, 42, histogramBucket.bucket.Value)
	assert.Equal(t, 1.0, histogramBucket.bucket.LowerBound)
	assert.Equal(t, 2.0, histogramBucket.bucket.UpperBound)
	assert.Equal(t, true, histogramBucket.bucket.Monotonic)
	assert.Equal(t, "my-hostname", histogramBucket.bucket.Host)
	assert.Equal(t, []string{"foo", "bar"}, histogramBucket.bucket.Tags)
}

func TestCheckSenderHostname(t *testing.T) {
	defaultHostname := "default-host"

	for nb, tc := range []struct {
		defaultHostnameDisabled bool
		submittedHostname       string
		expectedHostname        string
	}{
		{
			defaultHostnameDisabled: false,
			submittedHostname:       "",
			expectedHostname:        defaultHostname,
		},
		{
			defaultHostnameDisabled: false,
			submittedHostname:       "custom",
			expectedHostname:        "custom",
		},
		{
			defaultHostnameDisabled: true,
			submittedHostname:       "",
			expectedHostname:        "",
		},
		{
			defaultHostnameDisabled: true,
			submittedHostname:       "custom",
			expectedHostname:        "custom",
		},
	} {
		t.Run(fmt.Sprintf("case %d: %q -> %q", nb, tc.submittedHostname, tc.expectedHostname), func(t *testing.T) {
			senderMetricSampleChan := make(chan senderMetricSample, 10)
			serviceCheckChan := make(chan metrics.ServiceCheck, 10)
			eventChan := make(chan metrics.Event, 10)
			bucketChan := make(chan senderHistogramBucket, 10)
			checkSender := newCheckSender(checkID1, defaultHostname, senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)
			checkSender.DisableDefaultHostname(tc.defaultHostnameDisabled)

			checkSender.Gauge("my.metric", 1.0, tc.submittedHostname, []string{"foo", "bar"})
			checkSender.Commit()
			checkSender.ServiceCheck("my_service.can_connect", metrics.ServiceCheckOK, tc.submittedHostname, []string{"foo", "bar"}, "message")
			submittedEvent := metrics.Event{
				Title:          "Something happened",
				Text:           "Description of the event",
				Ts:             12,
				Priority:       metrics.EventPriorityLow,
				Host:           tc.submittedHostname,
				Tags:           []string{"foo", "bar"},
				AlertType:      metrics.EventAlertTypeInfo,
				AggregationKey: "event_agg_key",
				SourceTypeName: "docker",
			}
			checkSender.Event(submittedEvent)

			gaugeSenderSample := <-senderMetricSampleChan
			assert.EqualValues(t, checkID1, gaugeSenderSample.id)
			assert.Equal(t, metrics.GaugeType, gaugeSenderSample.metricSample.Mtype)
			assert.Equal(t, tc.expectedHostname, gaugeSenderSample.metricSample.Host)
			assert.Equal(t, false, gaugeSenderSample.commit)

			serviceCheck := <-serviceCheckChan
			assert.Equal(t, "my_service.can_connect", serviceCheck.CheckName)
			assert.Equal(t, metrics.ServiceCheckOK, serviceCheck.Status)
			assert.Equal(t, tc.expectedHostname, serviceCheck.Host)
			assert.Equal(t, []string{"foo", "bar"}, serviceCheck.Tags)
			assert.Equal(t, "message", serviceCheck.Message)

			event := <-eventChan
			assert.Equal(t, "Something happened", event.Title)
			assert.Equal(t, int64(12), event.Ts)
			assert.Equal(t, tc.expectedHostname, event.Host)
			assert.Equal(t, []string{"foo", "bar"}, event.Tags)
		})
	}
}

func TestChangeAllSendersDefaultHostname(t *testing.T) {
	senderMetricSampleChan := make(chan senderMetricSample, 10)
	serviceCheckChan := make(chan metrics.ServiceCheck, 10)
	eventChan := make(chan metrics.Event, 10)
	bucketChan := make(chan senderHistogramBucket, 10)
	checkSender := newCheckSender(checkID1, "hostname1", senderMetricSampleChan, serviceCheckChan, eventChan, bucketChan)
	SetSender(checkSender, checkID1)

	checkSender.Gauge("my.metric", 1.0, "", nil)
	gaugeSenderSample := <-senderMetricSampleChan
	assert.Equal(t, "hostname1", gaugeSenderSample.metricSample.Host)

	changeAllSendersDefaultHostname("hostname2")
	checkSender.Gauge("my.metric", 1.0, "", nil)
	gaugeSenderSample = <-senderMetricSampleChan
	assert.Equal(t, "hostname2", gaugeSenderSample.metricSample.Host)

	changeAllSendersDefaultHostname("hostname1")
	checkSender.Gauge("my.metric", 1.0, "", nil)
	gaugeSenderSample = <-senderMetricSampleChan
	assert.Equal(t, "hostname1", gaugeSenderSample.metricSample.Host)
}
