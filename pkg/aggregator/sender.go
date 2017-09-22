// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

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
	Histogram(metric string, value float64, hostname string, tags []string)
	Historate(metric string, value float64, hostname string, tags []string)
	ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string)
	Event(e metrics.Event)
	GetMetricStats() map[string]int64
}

type metricStats struct {
	Metrics       int64
	Events        int64
	ServiceChecks int64
	Lock          sync.RWMutex
}

// checkSender implements Sender
type checkSender struct {
	id               check.ID
	metricStats      metricStats
	priormetricStats metricStats
	smsOut           chan<- senderMetricSample
	serviceCheckOut  chan<- metrics.ServiceCheck
	eventOut         chan<- metrics.Event
}

type senderMetricSample struct {
	id           check.ID
	metricSample *metrics.MetricSample
	commit       bool
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

func newCheckSender(id check.ID, smsOut chan<- senderMetricSample, serviceCheckOut chan<- metrics.ServiceCheck, eventOut chan<- metrics.Event) *checkSender {
	return &checkSender{
		id:               id,
		smsOut:           smsOut,
		serviceCheckOut:  serviceCheckOut,
		eventOut:         eventOut,
		metricStats:      metricStats{},
		priormetricStats: metricStats{},
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
		var defaultCheckID check.ID // the default value is the zero value
		aggregatorInstance.registerSender(defaultCheckID)
		senderInstance = newCheckSender(defaultCheckID, aggregatorInstance.checkMetricIn, aggregatorInstance.serviceCheckIn, aggregatorInstance.eventIn)
	})

	return senderInstance, nil
}

// Commit commits the metric samples that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	s.smsOut <- senderMetricSample{s.id, &metrics.MetricSample{}, true}
	go s.cyclemetricStats()
}

func (s *checkSender) GetMetricStats() map[string]int64 {
	s.priormetricStats.Lock.RLock()
	defer s.priormetricStats.Lock.RUnlock()

	metricStats := make(map[string]int64)
	metricStats["Metrics"] = s.priormetricStats.Metrics
	metricStats["Events"] = s.priormetricStats.Events
	metricStats["ServiceChecks"] = s.priormetricStats.ServiceChecks

	return metricStats
}

func (s *checkSender) cyclemetricStats() {
	s.metricStats.Lock.Lock()
	s.priormetricStats.Lock.Lock()
	s.priormetricStats.Metrics = s.metricStats.Metrics
	s.priormetricStats.Events = s.metricStats.Events
	s.priormetricStats.ServiceChecks = s.metricStats.ServiceChecks
	s.metricStats.Metrics = 0
	s.metricStats.Events = 0
	s.metricStats.ServiceChecks = 0
	s.metricStats.Lock.Unlock()
	s.priormetricStats.Lock.Unlock()
}

func (s *checkSender) sendMetricSample(metric string, value float64, hostname string, tags []string, mType metrics.MetricType) {
	log.Debug(mType.String(), " sample: ", metric, ": ", value, " for hostname: ", hostname, " tags: ", tags)

	metricSample := &metrics.MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      mType,
		Tags:       tags,
		Host:       hostname,
		SampleRate: 1,
		Timestamp:  timeNowNano(),
	}

	s.smsOut <- senderMetricSample{s.id, metricSample, false}

	s.metricStats.Lock.Lock()
	s.metricStats.Metrics++
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

// Histogram should be used to track the statistical distribution of a set of values during a check run
// Should be called multiple times on the same (metric, hostname, tags) so that a distribution can be computed
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistogramType)
}

// Historate should be used to create a histogram metric for "rate" like metrics.
// Warning this doesn't use the harmonic mean, beware of what it means when using it.
func (s *checkSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistorateType)
}

// ServiceCheck submits a service check
func (s *checkSender) ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) {
	log.Debug("Service check submitted: ", checkName, ": ", status.String(), " for hostname: ", hostname, " tags: ", tags)
	serviceCheck := metrics.ServiceCheck{
		CheckName: checkName,
		Status:    status,
		Host:      hostname,
		Ts:        time.Now().Unix(),
		Tags:      tags,
		Message:   message,
	}

	s.serviceCheckOut <- serviceCheck

	s.metricStats.Lock.Lock()
	s.metricStats.ServiceChecks++
	s.metricStats.Lock.Unlock()
}

// Event submits an event
func (s *checkSender) Event(e metrics.Event) {
	log.Debug("Event submitted: ", e.Title, " for hostname: ", e.Host, " tags: ", e.Tags)

	s.eventOut <- e

	s.metricStats.Lock.Lock()
	s.metricStats.Events++
	s.metricStats.Lock.Unlock()
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
	sender := newCheckSender(id, aggregatorInstance.checkMetricIn, aggregatorInstance.serviceCheckIn, aggregatorInstance.eventIn)
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
