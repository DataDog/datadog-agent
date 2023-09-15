// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RawSender interface to submit samples to aggregator directly
type RawSender interface {
	SendRawMetricSample(sample *metrics.MetricSample)
	SendRawServiceCheck(sc *servicecheck.ServiceCheck)
	Event(e event.Event)
}

// checkSender implements Sender
type checkSender struct {
	id                      checkid.ID
	defaultHostname         string
	defaultHostnameDisabled bool
	metricStats             stats.SenderStats
	priormetricStats        stats.SenderStats
	statsLock               sync.RWMutex
	itemsOut                chan<- senderItem
	serviceCheckOut         chan<- servicecheck.ServiceCheck
	eventOut                chan<- event.Event
	orchestratorMetadataOut chan<- senderOrchestratorMetadata
	orchestratorManifestOut chan<- senderOrchestratorManifest
	eventPlatformOut        chan<- senderEventPlatformEvent
	checkTags               []string
	service                 string
	noIndex                 bool
}

// senderItem knows how the aggregator should handle it
type senderItem interface {
	handle(agg *BufferedAggregator)
}

type senderMetricSample struct {
	id           checkid.ID
	metricSample *metrics.MetricSample
	commit       bool
}

func (s *senderMetricSample) handle(agg *BufferedAggregator) {
	agg.handleSenderSample(*s)
}

type senderHistogramBucket struct {
	id     checkid.ID
	bucket *metrics.HistogramBucket
}

func (s *senderHistogramBucket) handle(agg *BufferedAggregator) {
	agg.handleSenderBucket(*s)
}

type senderEventPlatformEvent struct {
	id        checkid.ID
	rawEvent  []byte
	eventType string
}

type senderOrchestratorMetadata struct {
	msgs        []types.ProcessMessageBody
	clusterID   string
	payloadType int
}

type senderOrchestratorManifest struct {
	msgs      []types.ProcessMessageBody
	clusterID string
}

type checkSenderPool struct {
	agg     *BufferedAggregator
	senders map[checkid.ID]sender.Sender
	m       sync.Mutex
}

func newCheckSender(
	id checkid.ID,
	defaultHostname string,
	itemsOut chan<- senderItem,
	serviceCheckOut chan<- servicecheck.ServiceCheck,
	eventOut chan<- event.Event,
	orchestratorMetadataOut chan<- senderOrchestratorMetadata,
	orchestratorManifestOut chan<- senderOrchestratorManifest,
	eventPlatformOut chan<- senderEventPlatformEvent,
) *checkSender {
	return &checkSender{
		id:                      id,
		defaultHostname:         defaultHostname,
		itemsOut:                itemsOut,
		serviceCheckOut:         serviceCheckOut,
		eventOut:                eventOut,
		metricStats:             stats.NewSenderStats(),
		priormetricStats:        stats.NewSenderStats(),
		orchestratorMetadataOut: orchestratorMetadataOut,
		orchestratorManifestOut: orchestratorManifestOut,
		eventPlatformOut:        eventPlatformOut,
	}
}

// DisableDefaultHostname allows check to override the default hostname that will be injected
// when no hostname is specified at submission (for metrics, events and service checks).
func (s *checkSender) DisableDefaultHostname(disable bool) {
	s.defaultHostnameDisabled = disable
}

// SetCheckCustomTags stores the tags set in the check configuration file.
// They will be appended to each send (metric, event and service)
func (s *checkSender) SetCheckCustomTags(tags []string) {
	s.checkTags = tags
}

// SetCheckService appends the service as a tag for metrics, events, and service checks
// This may be called any number of times, though the only the last call will have an effect
func (s *checkSender) SetCheckService(service string) {
	s.service = service
}

// FinalizeCheckServiceTag appends the service as a tag for metrics, events, and service checks
func (s *checkSender) FinalizeCheckServiceTag() {
	if s.service != "" {
		s.checkTags = append(s.checkTags, fmt.Sprintf("service:%s", s.service))
	}
}

func (s *checkSender) SetNoIndex(noIndex bool) {
	s.noIndex = noIndex
}

// Commit commits the metric samples & histogram buckets that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	// we use a metric sample to commit both for metrics & sketches
	s.itemsOut <- &senderMetricSample{s.id, &metrics.MetricSample{}, true}
	s.cyclemetricStats()
}

func (s *checkSender) GetSenderStats() (metricStats stats.SenderStats) {
	s.statsLock.RLock()
	defer s.statsLock.RUnlock()
	return s.priormetricStats.Copy()
}

func (s *checkSender) cyclemetricStats() {
	s.statsLock.Lock()
	defer s.statsLock.Unlock()
	s.priormetricStats = s.metricStats.Copy()
	s.metricStats = stats.NewSenderStats()
}

// SendRawMetricSample sends the raw sample
// Useful for testing - submitting precomputed samples.
func (s *checkSender) SendRawMetricSample(sample *metrics.MetricSample) {
	s.itemsOut <- &senderMetricSample{s.id, sample, false}
}

func (s *checkSender) sendMetricSample(
	metric string,
	value float64,
	hostname string,
	tags []string,
	mType metrics.MetricType,
	flushFirstValue bool,
	noIndex bool) {
	tags = append(tags, s.checkTags...)

	log.Trace(mType.String(), " sample: ", metric, ": ", value, " for hostname: ", hostname, " tags: ", tags)

	metricSample := &metrics.MetricSample{
		Name:            metric,
		Value:           value,
		Mtype:           mType,
		Tags:            tags,
		Host:            hostname,
		SampleRate:      1,
		Timestamp:       timeNowNano(),
		FlushFirstValue: flushFirstValue,
		NoIndex:         s.noIndex || noIndex,
	}

	if hostname == "" && !s.defaultHostnameDisabled {
		metricSample.Host = s.defaultHostname
	}

	s.itemsOut <- &senderMetricSample{s.id, metricSample, false}

	s.statsLock.Lock()
	s.metricStats.MetricSamples++
	s.statsLock.Unlock()
}

// Gauge should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeType, false, false)
}

// GaugeNoIndex should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
// This value is not indexed by the backend.
func (s *checkSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeType, false, true)
}

// Rate should be used to track the rate of a metric over each check run
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.RateType, false, false)
}

// Count should be used to count a number of events that occurred during the check run
func (s *checkSender) Count(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CountType, false, false)
}

// MonotonicCount should be used to track the increase of a monotonic raw counter
func (s *checkSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.MonotonicCountType, false, false)
}

// MonotonicCountWithFlushFirstValue should be used to track the increase of a monotonic raw counter,
// and allows specifying whether the aggregator should flush the first sampled value as-is.
func (s *checkSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.MonotonicCountType, flushFirstValue, false)
}

// Counter is DEPRECATED and only implemented to preserve backward compatibility with python checks. Prefer using either:
// * `Gauge` if you're counting states
// * `Count` if you're counting events
func (s *checkSender) Counter(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CounterType, false, false)
}

// Histogram should be used to track the statistical distribution of a set of values during a check run
// Should be called multiple times on the same (metric, hostname, tags) so that a distribution can be computed
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistogramType, false, false)
}

// HistogramBucket should be called to directly send raw buckets to be submitted as distribution metrics
func (s *checkSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	tags = append(tags, s.checkTags...)

	log.Tracef(
		"Histogram Bucket %s submitted: %v [%f-%f] monotonic: %v for host %s tags: %v",
		metric,
		value,
		lowerBound,
		upperBound,
		monotonic,
		hostname,
		tags,
	)

	histogramBucket := &metrics.HistogramBucket{
		Name:            metric,
		Value:           value,
		LowerBound:      lowerBound,
		UpperBound:      upperBound,
		Monotonic:       monotonic,
		Host:            hostname,
		Tags:            tags,
		Timestamp:       timeNowNano(),
		FlushFirstValue: flushFirstValue,
	}

	if hostname == "" && !s.defaultHostnameDisabled {
		histogramBucket.Host = s.defaultHostname
	}

	s.itemsOut <- &senderHistogramBucket{s.id, histogramBucket}

	s.statsLock.Lock()
	s.metricStats.HistogramBuckets++
	s.statsLock.Unlock()
}

// Historate should be used to create a histogram metric for "rate" like metrics.
// Warning this doesn't use the harmonic mean, beware of what it means when using it.
func (s *checkSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistorateType, false, false)
}

func (s *checkSender) Distribution(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.DistributionType, false, false)
}

// SendRawServiceCheck sends the raw service check
// Useful for testing - submitting precomputed service check.
func (s *checkSender) SendRawServiceCheck(sc *servicecheck.ServiceCheck) {
	s.serviceCheckOut <- *sc
}

// ServiceCheck submits a service check
func (s *checkSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	log.Trace("Service check submitted: ", checkName, ": ", status.String(), " for hostname: ", hostname, " tags: ", tags)
	serviceCheck := servicecheck.ServiceCheck{
		CheckName: checkName,
		Status:    status,
		Host:      hostname,
		Ts:        time.Now().Unix(),
		Tags:      append(tags, s.checkTags...),
		Message:   message,
	}

	if hostname == "" && !s.defaultHostnameDisabled {
		serviceCheck.Host = s.defaultHostname
	}

	s.serviceCheckOut <- serviceCheck

	s.statsLock.Lock()
	s.metricStats.ServiceChecks++
	s.statsLock.Unlock()
}

// Event submits an event
func (s *checkSender) Event(e event.Event) {
	e.Tags = append(e.Tags, s.checkTags...)

	log.Trace("Event submitted: ", e.Title, " for hostname: ", e.Host, " tags: ", e.Tags)

	if e.Host == "" && !s.defaultHostnameDisabled {
		e.Host = s.defaultHostname
	}

	s.eventOut <- e

	s.statsLock.Lock()
	s.metricStats.Events++
	s.statsLock.Unlock()
}

// Event submits an event
func (s *checkSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	s.eventPlatformOut <- senderEventPlatformEvent{
		id:        s.id,
		rawEvent:  rawEvent,
		eventType: eventType,
	}
	s.statsLock.Lock()
	defer s.statsLock.Unlock()
	s.metricStats.EventPlatformEvents[eventType] = s.metricStats.EventPlatformEvents[eventType] + 1
}

// OrchestratorMetadata submit orchestrator metadata messages
func (s *checkSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	om := senderOrchestratorMetadata{
		msgs:        msgs,
		clusterID:   clusterID,
		payloadType: nodeType,
	}
	s.orchestratorMetadataOut <- om
}

func (s *checkSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	om := senderOrchestratorManifest{
		msgs:      msgs,
		clusterID: clusterID,
	}
	s.orchestratorManifestOut <- om
}

func (sp *checkSenderPool) getSender(id checkid.ID) (sender.Sender, error) {
	sp.m.Lock()
	defer sp.m.Unlock()

	if sender, ok := sp.senders[id]; ok {
		return sender, nil
	}
	return nil, fmt.Errorf("Sender not found")
}

func (sp *checkSenderPool) mkSender(id checkid.ID) (sender.Sender, error) {
	sp.m.Lock()
	defer sp.m.Unlock()

	err := sp.agg.registerSender(id)
	sender := newCheckSender(
		id,
		sp.agg.hostname,
		sp.agg.checkItems,
		sp.agg.serviceCheckIn,
		sp.agg.eventIn,
		sp.agg.orchestratorMetadataIn,
		sp.agg.orchestratorManifestIn,
		sp.agg.eventPlatformIn,
	)
	sp.senders[id] = sender
	return sender, err
}

func (sp *checkSenderPool) setSender(sender sender.Sender, id checkid.ID) error {
	sp.m.Lock()
	defer sp.m.Unlock()

	if _, ok := sp.senders[id]; ok {
		sp.agg.deregisterSender(id)
	}
	err := sp.agg.registerSender(id)
	sp.senders[id] = sender

	return err
}

func (sp *checkSenderPool) removeSender(id checkid.ID) {
	sp.m.Lock()
	defer sp.m.Unlock()

	delete(sp.senders, id)
	sp.agg.deregisterSender(id)
}
