// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type senderWithChans struct {
	itemChan                 chan senderItem
	serviceCheckChan         chan servicecheck.ServiceCheck
	eventChan                chan event.Event
	orchestratorChan         chan senderOrchestratorMetadata
	orchestratorManifestChan chan senderOrchestratorManifest
	eventPlatformEventChan   chan senderEventPlatformEvent
	sender                   *checkSender
}

func initSender(id checkid.ID, defaultHostname string) (s senderWithChans) {
	s.itemChan = make(chan senderItem, 10)
	s.serviceCheckChan = make(chan servicecheck.ServiceCheck, 10)
	s.eventChan = make(chan event.Event, 10)
	s.orchestratorChan = make(chan senderOrchestratorMetadata, 10)
	s.orchestratorManifestChan = make(chan senderOrchestratorManifest, 10)
	s.eventPlatformEventChan = make(chan senderEventPlatformEvent, 10)
	s.sender = newCheckSender(id, defaultHostname, s.itemChan, s.serviceCheckChan, s.eventChan, s.orchestratorChan, s.orchestratorManifestChan, s.eventPlatformEventChan)
	return s
}

func testDemux(log log.Component) *AgentDemultiplexer {
	opts := DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true
	demux := initAgentDemultiplexer(log, NewForwarderTest(log), opts, defaultHostname)
	return demux
}

func assertAggSamplersLen(t *testing.T, agg *BufferedAggregator, n int) {
	assert.Eventually(t, func() bool {
		agg.mu.Lock()
		defer agg.mu.Unlock()
		return len(agg.checkSamplers) == n
	}, time.Second, 10*time.Millisecond)

	agg.mu.Lock()
	defer agg.mu.Unlock()
	// This provides a nicer error message than Eventually if the test fails
	assert.Len(t, agg.checkSamplers, n)
}

func TestGetDefaultSenderReturnsSameSender(t *testing.T) {
	// this test not using anything global
	// -
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)
	aggregatorInstance := demux.Aggregator()
	go aggregatorInstance.run()
	defer aggregatorInstance.Stop()

	s, err := demux.GetDefaultSender()
	assert.Nil(t, err)
	defaultSender1 := s.(*checkSender)
	assertAggSamplersLen(t, aggregatorInstance, 1)

	s, err = demux.GetDefaultSender()
	assert.Nil(t, err)
	defaultSender2 := s.(*checkSender)
	assert.Equal(t, defaultSender1.id, defaultSender2.id)
}

func TestGetSenderWithDifferentIDsReturnsDifferentCheckSamplers(t *testing.T) {
	// this test not using anything global
	// -
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)

	aggregatorInstance := demux.Aggregator()
	go aggregatorInstance.run()
	defer aggregatorInstance.Stop()

	s, err := demux.GetSender(checkID1)
	assert.Nil(t, err)
	sender1 := s.(*checkSender)
	assertAggSamplersLen(t, aggregatorInstance, 1)

	s, err = demux.GetSender(checkID2)
	assert.Nil(t, err)
	sender2 := s.(*checkSender)
	assertAggSamplersLen(t, aggregatorInstance, 2)
	assert.NotEqual(t, sender1.id, sender2.id)

	s, err = demux.GetDefaultSender()
	assert.Nil(t, err)
	defaultSender := s.(*checkSender)
	assertAggSamplersLen(t, aggregatorInstance, 3)
	assert.NotEqual(t, sender1.id, defaultSender.id)
	assert.NotEqual(t, sender2.id, defaultSender.id)
}

func TestGetSenderWithSameIDsReturnsSameSender(t *testing.T) {
	// this test not using anything global
	// -

	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)
	aggregatorInstance := demux.Aggregator()
	go aggregatorInstance.run()
	defer aggregatorInstance.Stop()

	sender1, err := demux.GetSender(checkID1)
	assert.Nil(t, err)
	assertAggSamplersLen(t, aggregatorInstance, 1)

	assert.Len(t, demux.senderPool.senders, 1)

	sender2, err := demux.GetSender(checkID1)
	assert.Nil(t, err)
	assert.Equal(t, sender1, sender2)

	assert.Len(t, demux.senderPool.senders, 1)
}

func TestDestroySender(t *testing.T) {
	// this test not using anything global
	// -

	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)
	aggregatorInstance := demux.Aggregator()
	go aggregatorInstance.run()
	defer aggregatorInstance.Stop()

	_, err := demux.GetSender(checkID1)
	assert.Nil(t, err)
	assertAggSamplersLen(t, aggregatorInstance, 1)

	_, err = demux.GetSender(checkID2)
	assert.Nil(t, err)
	assertAggSamplersLen(t, aggregatorInstance, 2)

	demux.DestroySender(checkID1)

	assert.Eventually(t, func() bool {
		aggregatorInstance.mu.Lock()
		defer aggregatorInstance.mu.Unlock()
		return aggregatorInstance.checkSamplers[checkID1].deregistered
	}, time.Second, 10*time.Millisecond)

	aggregatorInstance.Flush(testNewFlushTrigger(time.Now(), false))
	assertAggSamplersLen(t, aggregatorInstance, 1)
}

func TestGetAndSetSender(t *testing.T) {
	// this test not using anything global
	// -

	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)

	itemChan := make(chan senderItem, 10)
	serviceCheckChan := make(chan servicecheck.ServiceCheck, 10)
	eventChan := make(chan event.Event, 10)
	orchestratorChan := make(chan senderOrchestratorMetadata, 10)
	orchestratorManifestChan := make(chan senderOrchestratorManifest, 10)
	eventPlatformChan := make(chan senderEventPlatformEvent, 10)
	testCheckSender := newCheckSender(checkID1, "", itemChan, serviceCheckChan, eventChan, orchestratorChan, orchestratorManifestChan, eventPlatformChan)

	err := demux.SetSender(testCheckSender, checkID1)
	assert.Nil(t, err)

	sender, err := demux.GetSender(checkID1)
	assert.Nil(t, err)
	assert.Equal(t, testCheckSender, sender)
}

func TestGetSenderDefaultHostname(t *testing.T) {
	// this test not using anything global
	// -

	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := testDemux(log)
	aggregatorInstance := demux.Aggregator()
	go aggregatorInstance.run()

	sender, err := demux.GetSender(checkID1)
	require.NoError(t, err)

	checksender, ok := sender.(*checkSender)
	require.True(t, ok)

	assert.Equal(t, demux.Aggregator().hostname, checksender.defaultHostname)
	assert.Equal(t, false, checksender.defaultHostnameDisabled)

	aggregatorInstance.Stop()
}

func TestGetSenderServiceTagMetrics(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")
	checkTags := []string{"check:tag1", "check:tag2"}

	// only tags added by the check
	s.sender.SetCheckService("")
	s.sender.FinalizeCheckServiceTag()

	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType, false, false)
	sms := (<-s.itemChan).(*senderMetricSample)
	assert.Equal(t, checkTags, sms.metricSample.Tags)

	// only last call is added as a tag
	s.sender.SetCheckService("service1")
	s.sender.SetCheckService("service2")
	s.sender.FinalizeCheckServiceTag()
	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType, false, false)
	sms = (<-s.itemChan).(*senderMetricSample)
	assert.Equal(t, append(checkTags, "service:service2"), sms.metricSample.Tags)
}

func TestGetSenderServiceTagServiceCheck(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")
	checkTags := []string{"check:tag1", "check:tag2"}

	// only tags added by the check
	s.sender.SetCheckService("")
	s.sender.FinalizeCheckServiceTag()
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc := <-s.serviceCheckChan
	assert.Equal(t, checkTags, sc.Tags)

	// only last call is added as a tag
	s.sender.SetCheckService("service1")
	s.sender.SetCheckService("service2")
	s.sender.FinalizeCheckServiceTag()
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc = <-s.serviceCheckChan
	assert.Equal(t, append(checkTags, "service:service2"), sc.Tags)
}

func TestGetSenderServiceTagEvent(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")
	checkTags := []string{"check:tag1", "check:tag2"}

	event := event.Event{
		Title: "title",
		Host:  "testhostname",
		Ts:    time.Now().Unix(),
		Text:  "text",
		Tags:  checkTags,
	}

	// only tags added by the check
	s.sender.SetCheckService("")
	s.sender.FinalizeCheckServiceTag()
	s.sender.Event(event)
	e := <-s.eventChan
	assert.Equal(t, checkTags, e.Tags)

	// only last call is added as a tag
	s.sender.SetCheckService("service1")
	s.sender.SetCheckService("service2")
	s.sender.FinalizeCheckServiceTag()
	s.sender.Event(event)
	e = <-s.eventChan
	assert.Equal(t, append(checkTags, "service:service2"), e.Tags)
}

func TestGetSenderAddCheckCustomTagsMetrics(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")
	// no custom tags

	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", nil, metrics.CounterType, false, false)
	sms := (<-s.itemChan).(*senderMetricSample)
	assert.Nil(t, sms.metricSample.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType, false, false)
	sms = (<-s.itemChan).(*senderMetricSample)
	assert.Equal(t, checkTags, sms.metricSample.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	s.sender.SetCheckCustomTags(customTags)
	assert.Len(t, s.sender.checkTags, 2)

	// only tags coming from the configuration file
	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", nil, metrics.CounterType, false, false)
	sms = (<-s.itemChan).(*senderMetricSample)
	assert.Equal(t, customTags, sms.metricSample.Tags)

	// tags added by the check + tags coming from the configuration file
	s.sender.sendMetricSample("metric.test", 42.0, "testhostname", checkTags, metrics.CounterType, false, false)
	sms = (<-s.itemChan).(*senderMetricSample)
	assert.Equal(t, append(checkTags, customTags...), sms.metricSample.Tags)
}

func TestGetSenderAddCheckCustomTagsService(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")

	// no custom tags
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", nil, "test message")
	sc := <-s.serviceCheckChan
	assert.Nil(t, sc.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc = <-s.serviceCheckChan
	assert.Equal(t, checkTags, sc.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	s.sender.SetCheckCustomTags(customTags)
	assert.Len(t, s.sender.checkTags, 2)

	// only tags coming from the configuration file
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", nil, "test message")
	sc = <-s.serviceCheckChan
	assert.Equal(t, customTags, sc.Tags)

	// tags added by the check + tags coming from the configuration file
	s.sender.ServiceCheck("test", servicecheck.ServiceCheckOK, "testhostname", checkTags, "test message")
	sc = <-s.serviceCheckChan
	assert.Equal(t, append(checkTags, customTags...), sc.Tags)
}

func TestGetSenderAddCheckCustomTagsEvent(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")

	event := event.Event{
		Title: "title",
		Host:  "testhostname",
		Ts:    time.Now().Unix(),
		Text:  "text",
		Tags:  nil,
	}

	// no custom tags
	s.sender.Event(event)
	e := <-s.eventChan
	assert.Nil(t, e.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	event.Tags = checkTags
	s.sender.Event(event)
	e = <-s.eventChan
	assert.Equal(t, checkTags, e.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	s.sender.SetCheckCustomTags(customTags)
	assert.Len(t, s.sender.checkTags, 2)

	// only tags coming from the configuration file
	event.Tags = nil
	s.sender.Event(event)
	e = <-s.eventChan
	assert.Equal(t, customTags, e.Tags)

	// tags added by the check + tags coming from the configuration file
	event.Tags = checkTags
	s.sender.Event(event)
	e = <-s.eventChan
	assert.Equal(t, append(checkTags, customTags...), e.Tags)
}

func TestGetSenderAddCheckCustomTagsHistogramBucket(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "")

	// no custom tags
	s.sender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", nil, false)
	bucketSample := (<-s.itemChan).(*senderHistogramBucket)
	assert.Nil(t, bucketSample.bucket.Tags)

	// only tags added by the check
	checkTags := []string{"check:tag1", "check:tag2"}
	s.sender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", checkTags, false)
	bucketSample = (<-s.itemChan).(*senderHistogramBucket)
	assert.Equal(t, checkTags, bucketSample.bucket.Tags)

	// simulate tags in the configuration file
	customTags := []string{"custom:tag1", "custom:tag2"}
	s.sender.SetCheckCustomTags(customTags)
	assert.Len(t, s.sender.checkTags, 2)

	// only tags coming from the configuration file
	s.sender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", nil, false)
	bucketSample = (<-s.itemChan).(*senderHistogramBucket)
	assert.Equal(t, customTags, bucketSample.bucket.Tags)

	// tags added by the check + tags coming from the configuration file
	s.sender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", checkTags, false)
	bucketSample = (<-s.itemChan).(*senderHistogramBucket)
	assert.Equal(t, append(checkTags, customTags...), bucketSample.bucket.Tags)
}

func TestCheckSenderInterface(t *testing.T) {
	// this test not using anything global
	// -

	s := initSender(checkID1, "default-hostname")
	s.sender.Gauge("my.metric", 1.0, "my-hostname", []string{"foo", "bar"})
	s.sender.Rate("my.rate_metric", 2.0, "my-hostname", []string{"foo", "bar"})
	s.sender.Count("my.count_metric", 123.0, "my-hostname", []string{"foo", "bar"})
	s.sender.MonotonicCount("my.monotonic_count_metric", 12.0, "my-hostname", []string{"foo", "bar"})
	s.sender.MonotonicCountWithFlushFirstValue("my.monotonic_count_metric", 12.0, "my-hostname", []string{"foo", "bar"}, true)
	s.sender.Counter("my.counter_metric", 1.0, "my-hostname", []string{"foo", "bar"})
	s.sender.Histogram("my.histo_metric", 3.0, "my-hostname", []string{"foo", "bar"})
	s.sender.HistogramBucket("my.histogram_bucket", 42, 1.0, 2.0, true, "my-hostname", []string{"foo", "bar"}, true)
	s.sender.Distribution("my.distribution", 43.0, "my-hostname", []string{"foo", "bar"})
	s.sender.Commit()
	s.sender.ServiceCheck("my_service.can_connect", servicecheck.ServiceCheckOK, "my-hostname", []string{"foo", "bar"}, "message")
	s.sender.EventPlatformEvent([]byte("raw-event"), "dbm-sample")
	submittedEvent := event.Event{
		Title:          "Something happened",
		Text:           "Description of the event",
		Ts:             12,
		Priority:       event.EventPriorityLow,
		Host:           "my-hostname",
		Tags:           []string{"foo", "bar"},
		AlertType:      event.EventAlertTypeInfo,
		AggregationKey: "event_agg_key",
		SourceTypeName: "docker",
	}
	s.sender.Event(submittedEvent)

	gaugeSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, gaugeSenderSample.id)
	assert.Equal(t, metrics.GaugeType, gaugeSenderSample.metricSample.Mtype)
	assert.Equal(t, "my-hostname", gaugeSenderSample.metricSample.Host)
	assert.Equal(t, false, gaugeSenderSample.commit)

	rateSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, rateSenderSample.id)
	assert.Equal(t, metrics.RateType, rateSenderSample.metricSample.Mtype)
	assert.Equal(t, false, rateSenderSample.commit)

	countSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, countSenderSample.id)
	assert.Equal(t, metrics.CountType, countSenderSample.metricSample.Mtype)
	assert.Equal(t, false, countSenderSample.commit)

	monotonicCountSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, monotonicCountSenderSample.id)
	assert.Equal(t, metrics.MonotonicCountType, monotonicCountSenderSample.metricSample.Mtype)
	assert.Equal(t, false, monotonicCountSenderSample.commit)

	monotonicCountWithFlushFirstValueSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, monotonicCountWithFlushFirstValueSenderSample.id)
	assert.Equal(t, metrics.MonotonicCountType, monotonicCountWithFlushFirstValueSenderSample.metricSample.Mtype)
	assert.Equal(t, true, monotonicCountWithFlushFirstValueSenderSample.metricSample.FlushFirstValue)
	assert.Equal(t, false, monotonicCountWithFlushFirstValueSenderSample.commit)

	CounterSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, CounterSenderSample.id)
	assert.Equal(t, metrics.CounterType, CounterSenderSample.metricSample.Mtype)
	assert.Equal(t, false, CounterSenderSample.commit)

	histoSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, histoSenderSample.id)
	assert.Equal(t, metrics.HistogramType, histoSenderSample.metricSample.Mtype)
	assert.Equal(t, false, histoSenderSample.commit)

	histogramBucket := (<-s.itemChan).(*senderHistogramBucket)
	assert.Equal(t, "my.histogram_bucket", histogramBucket.bucket.Name)
	assert.Equal(t, int64(42), histogramBucket.bucket.Value)
	assert.Equal(t, 1.0, histogramBucket.bucket.LowerBound)
	assert.Equal(t, 2.0, histogramBucket.bucket.UpperBound)
	assert.Equal(t, true, histogramBucket.bucket.Monotonic)
	assert.Equal(t, "my-hostname", histogramBucket.bucket.Host)
	assert.Equal(t, []string{"foo", "bar"}, histogramBucket.bucket.Tags)
	assert.Equal(t, true, histogramBucket.bucket.FlushFirstValue)

	distributionSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, distributionSample.id)
	assert.Equal(t, metrics.DistributionType, distributionSample.metricSample.Mtype)
	assert.Equal(t, false, distributionSample.commit)

	commitSenderSample := (<-s.itemChan).(*senderMetricSample)
	assert.EqualValues(t, checkID1, commitSenderSample.id)
	assert.Equal(t, true, commitSenderSample.commit)

	serviceCheck := <-s.serviceCheckChan
	assert.Equal(t, "my_service.can_connect", serviceCheck.CheckName)
	assert.Equal(t, servicecheck.ServiceCheckOK, serviceCheck.Status)
	assert.Equal(t, "my-hostname", serviceCheck.Host)
	assert.Equal(t, []string{"foo", "bar"}, serviceCheck.Tags)
	assert.Equal(t, "message", serviceCheck.Message)

	event := <-s.eventChan
	assert.Equal(t, submittedEvent, event)

	eventPlatformEvent := <-s.eventPlatformEventChan
	assert.Equal(t, checkID1, eventPlatformEvent.id)
	assert.Equal(t, []byte("raw-event"), eventPlatformEvent.rawEvent)
	assert.Equal(t, "dbm-sample", eventPlatformEvent.eventType)
}

func TestCheckSenderHostname(t *testing.T) {
	// this test not using anything global
	// -

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
			s := initSender(checkID1, defaultHostname)
			s.sender.DisableDefaultHostname(tc.defaultHostnameDisabled)

			s.sender.Gauge("my.metric", 1.0, tc.submittedHostname, []string{"foo", "bar"})
			s.sender.Commit()
			s.sender.ServiceCheck("my_service.can_connect", servicecheck.ServiceCheckOK, tc.submittedHostname, []string{"foo", "bar"}, "message")
			submittedEvent := event.Event{
				Title:          "Something happened",
				Text:           "Description of the event",
				Ts:             12,
				Priority:       event.EventPriorityLow,
				Host:           tc.submittedHostname,
				Tags:           []string{"foo", "bar"},
				AlertType:      event.EventAlertTypeInfo,
				AggregationKey: "event_agg_key",
				SourceTypeName: "docker",
			}
			s.sender.Event(submittedEvent)

			gaugeSenderSample := (<-s.itemChan).(*senderMetricSample)
			assert.EqualValues(t, checkID1, gaugeSenderSample.id)
			assert.Equal(t, metrics.GaugeType, gaugeSenderSample.metricSample.Mtype)
			assert.Equal(t, tc.expectedHostname, gaugeSenderSample.metricSample.Host)
			assert.Equal(t, false, gaugeSenderSample.commit)

			serviceCheck := <-s.serviceCheckChan
			assert.Equal(t, "my_service.can_connect", serviceCheck.CheckName)
			assert.Equal(t, servicecheck.ServiceCheckOK, serviceCheck.Status)
			assert.Equal(t, tc.expectedHostname, serviceCheck.Host)
			assert.Equal(t, []string{"foo", "bar"}, serviceCheck.Tags)
			assert.Equal(t, "message", serviceCheck.Message)

			event := <-s.eventChan
			assert.Equal(t, "Something happened", event.Title)
			assert.Equal(t, int64(12), event.Ts)
			assert.Equal(t, tc.expectedHostname, event.Host)
			assert.Equal(t, []string{"foo", "bar"}, event.Tags)
		})
	}
}
