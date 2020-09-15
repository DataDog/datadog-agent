// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package aggregator

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var senderInstance *checkSender
var senderInit sync.Once
var senderPool *checkSenderPool

// Sender allows sending metrics from checks/a check
type Sender interface {
	Commit()
	Gauge(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	Count(metric string, value float64, hostname string, tags []string)
	MonotonicCount(metric string, value float64, hostname string, tags []string)
	Counter(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
	Historate(metric string, value float64, hostname string, tags []string)
	ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string)
	HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string)
	Event(e metrics.Event)
	GetMetricStats() map[string]int64
	DisableDefaultHostname(disable bool)
	SetCheckCustomTags(tags []string)
	SetCheckService(service string)
	FinalizeCheckServiceTag()
}

type metricStats struct {
	MetricSamples    int64
	Events           int64
	ServiceChecks    int64
	HistogramBuckets int64
	Lock             sync.RWMutex
}

// RawSender interface to submit samples to aggregator directly
type RawSender interface {
	SendRawMetricSample(sample *metrics.MetricSample)
	SendRawServiceCheck(sc *metrics.ServiceCheck)
	Event(e metrics.Event)
}

// checkSender implements Sender
type checkSender struct {
	id                      check.ID
	defaultHostname         string
	defaultHostnameDisabled bool
	metricStats             metricStats
	priormetricStats        metricStats
	smsOut                  chan<- senderMetricSample
	serviceCheckOut         chan<- metrics.ServiceCheck
	eventOut                chan<- metrics.Event
	histogramBucketOut      chan<- senderHistogramBucket
	checkTags               []string
	service                 string
}

type senderMetricSample struct {
	id           check.ID
	metricSample *metrics.MetricSample
	commit       bool
}

type senderHistogramBucket struct {
	id     check.ID
	bucket *metrics.HistogramBucket
}

type checkSenderPool struct {
	senders map[check.ID]Sender
	m       sync.Mutex
}

func init() {
	senderPool = &checkSenderPool{
		senders: make(map[check.ID]Sender),
	}
}

func newCheckSender(id check.ID, defaultHostname string, smsOut chan<- senderMetricSample, serviceCheckOut chan<- metrics.ServiceCheck, eventOut chan<- metrics.Event, bucketOut chan<- senderHistogramBucket) *checkSender {
	return &checkSender{
		id:                 id,
		defaultHostname:    defaultHostname,
		smsOut:             smsOut,
		serviceCheckOut:    serviceCheckOut,
		eventOut:           eventOut,
		metricStats:        metricStats{},
		priormetricStats:   metricStats{},
		histogramBucketOut: bucketOut,
	}
}

// GetSender returns a Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func GetSender(id check.ID) (Sender, error) {
	if aggregatorInstance == nil {
		return nil, errors.New("Aggregator was not initialized")
	}
	sender, err := senderPool.getSender(id)
	if err != nil {
		sender, err = senderPool.mkSender(id)
	}
	return sender, err
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func DestroySender(id check.ID) {
	senderPool.removeSender(id)
}

// SetSender returns the passed sender with the passed ID.
// This is largely for testing purposes
func SetSender(sender Sender, id check.ID) error {
	if aggregatorInstance == nil {
		return errors.New("Aggregator was not initialized")
	}
	return senderPool.setSender(sender, id)
}

// GetDefaultSender returns the default sender
func GetDefaultSender() (Sender, error) {
	if aggregatorInstance == nil {
		return nil, errors.New("Aggregator was not initialized")
	}

	senderInit.Do(func() {
		var defaultCheckID check.ID                       // the default value is the zero value
		aggregatorInstance.registerSender(defaultCheckID) //nolint:errcheck
		senderInstance = newCheckSender(defaultCheckID, aggregatorInstance.hostname, aggregatorInstance.checkMetricIn, aggregatorInstance.serviceCheckIn, aggregatorInstance.eventIn, aggregatorInstance.checkHistogramBucketIn)
	})

	return senderInstance, nil
}

// changeAllSendersDefaultHostname is to be called by the aggregator
// when its hostname changes. All existing senders will have their
// default hostname updated.
func changeAllSendersDefaultHostname(hostname string) {
	if senderPool != nil {
		senderPool.changeAllSendersDefaultHostname(hostname)
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

// Commit commits the metric samples & histogram buckets that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	// we use a metric sample to commit both for metrics & sketches
	s.smsOut <- senderMetricSample{s.id, &metrics.MetricSample{}, true}
	s.cyclemetricStats()
}

func (s *checkSender) GetMetricStats() map[string]int64 {
	s.priormetricStats.Lock.RLock()
	defer s.priormetricStats.Lock.RUnlock()

	metricStats := make(map[string]int64)
	metricStats["MetricSamples"] = s.priormetricStats.MetricSamples
	metricStats["Events"] = s.priormetricStats.Events
	metricStats["ServiceChecks"] = s.priormetricStats.ServiceChecks
	metricStats["HistogramBuckets"] = s.priormetricStats.HistogramBuckets

	return metricStats
}

func (s *checkSender) cyclemetricStats() {
	s.metricStats.Lock.Lock()
	s.priormetricStats.Lock.Lock()
	s.priormetricStats.MetricSamples = s.metricStats.MetricSamples
	s.priormetricStats.Events = s.metricStats.Events
	s.priormetricStats.ServiceChecks = s.metricStats.ServiceChecks
	s.priormetricStats.HistogramBuckets = s.metricStats.HistogramBuckets
	s.metricStats.MetricSamples = 0
	s.metricStats.Events = 0
	s.metricStats.ServiceChecks = 0
	s.metricStats.HistogramBuckets = 0
	s.metricStats.Lock.Unlock()
	s.priormetricStats.Lock.Unlock()
}

// SendRawMetricSample sends the raw sample
// Useful for testing - submitting precomputed samples.
func (s *checkSender) SendRawMetricSample(sample *metrics.MetricSample) {
	s.smsOut <- senderMetricSample{s.id, sample, false}
}

func (s *checkSender) sendMetricSample(metric string, value float64, hostname string, tags []string, mType metrics.MetricType) {
	tags = append(tags, s.checkTags...)

	log.Trace(mType.String(), " sample: ", metric, ": ", value, " for hostname: ", hostname, " tags: ", tags)

	metricSample := &metrics.MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      mType,
		Tags:       tags,
		Host:       hostname,
		SampleRate: 1,
		Timestamp:  timeNowNano(),
	}

	if hostname == "" && !s.defaultHostnameDisabled {
		metricSample.Host = s.defaultHostname
	}

	s.smsOut <- senderMetricSample{s.id, metricSample, false}

	s.metricStats.Lock.Lock()
	s.metricStats.MetricSamples++
	s.metricStats.Lock.Unlock()
}

// Gauge should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeType)
}

// Rate should be used to track the rate of a metric over each check run
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.RateType)
}

// Count should be used to count a number of events that occurred during the check run
func (s *checkSender) Count(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CountType)
}

// MonotonicCount should be used to track the increase of a monotonic raw counter
func (s *checkSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.MonotonicCountType)
}

// Counter is DEPRECATED and only implemented to preserve backward compatibility with python checks. Prefer using either:
// * `Gauge` if you're counting states
// * `Count` if you're counting events
func (s *checkSender) Counter(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CounterType)
}

// Histogram should be used to track the statistical distribution of a set of values during a check run
// Should be called multiple times on the same (metric, hostname, tags) so that a distribution can be computed
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistogramType)
}

// HistogramBucket should be called to directly send raw buckets to be submitted as distribution metrics
func (s *checkSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string) {
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
		Name:       metric,
		Value:      value,
		LowerBound: lowerBound,
		UpperBound: upperBound,
		Monotonic:  monotonic,
		Host:       hostname,
		Tags:       tags,
		Timestamp:  timeNowNano(),
	}

	if hostname == "" && !s.defaultHostnameDisabled {
		histogramBucket.Host = s.defaultHostname
	}

	s.histogramBucketOut <- senderHistogramBucket{s.id, histogramBucket}

	s.metricStats.Lock.Lock()
	s.metricStats.HistogramBuckets++
	s.metricStats.Lock.Unlock()
}

// Historate should be used to create a histogram metric for "rate" like metrics.
// Warning this doesn't use the harmonic mean, beware of what it means when using it.
func (s *checkSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistorateType)
}

// SendRawServiceCheck sends the raw service check
// Useful for testing - submitting precomputed service check.
func (s *checkSender) SendRawServiceCheck(sc *metrics.ServiceCheck) {
	s.serviceCheckOut <- *sc
}

// ServiceCheck submits a service check
func (s *checkSender) ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) {
	log.Trace("Service check submitted: ", checkName, ": ", status.String(), " for hostname: ", hostname, " tags: ", tags)
	serviceCheck := metrics.ServiceCheck{
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

	s.metricStats.Lock.Lock()
	s.metricStats.ServiceChecks++
	s.metricStats.Lock.Unlock()
}

// Event submits an event
func (s *checkSender) Event(e metrics.Event) {
	e.Tags = append(e.Tags, s.checkTags...)

	log.Trace("Event submitted: ", e.Title, " for hostname: ", e.Host, " tags: ", e.Tags)

	if e.Host == "" && !s.defaultHostnameDisabled {
		e.Host = s.defaultHostname
	}

	s.eventOut <- e

	s.metricStats.Lock.Lock()
	s.metricStats.Events++
	s.metricStats.Lock.Unlock()
}

// changeAllSendersDefaultHostname u
func (sp *checkSenderPool) changeAllSendersDefaultHostname(hostname string) {
	sp.m.Lock()
	defer sp.m.Unlock()
	for _, sender := range sp.senders {
		cs, ok := sender.(*checkSender)
		if !ok {
			continue
		}
		cs.defaultHostname = hostname
	}
}

func (sp *checkSenderPool) getSender(id check.ID) (Sender, error) {
	sp.m.Lock()
	defer sp.m.Unlock()

	if sender, ok := sp.senders[id]; ok {
		return sender, nil
	}
	return nil, fmt.Errorf("Sender not found")
}

func (sp *checkSenderPool) mkSender(id check.ID) (Sender, error) {
	sp.m.Lock()
	defer sp.m.Unlock()

	err := aggregatorInstance.registerSender(id)
	sender := newCheckSender(id, aggregatorInstance.hostname, aggregatorInstance.checkMetricIn, aggregatorInstance.serviceCheckIn, aggregatorInstance.eventIn, aggregatorInstance.checkHistogramBucketIn)
	sp.senders[id] = sender
	return sender, err
}

func (sp *checkSenderPool) setSender(sender Sender, id check.ID) error {
	sp.m.Lock()
	defer sp.m.Unlock()

	if _, ok := sp.senders[id]; ok {
		aggregatorInstance.deregisterSender(id)
	}
	err := aggregatorInstance.registerSender(id)
	sp.senders[id] = sender

	return err
}

func (sp *checkSenderPool) removeSender(id check.ID) {
	sp.m.Lock()
	defer sp.m.Unlock()

	delete(sp.senders, id)
	aggregatorInstance.deregisterSender(id)
}
