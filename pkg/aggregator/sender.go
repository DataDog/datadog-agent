package aggregator

import (
	"errors"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

var senderInstance *checkSender
var senderInit sync.Once

// Sender allows sending metrics from checks/a check
type Sender interface {
	Commit()
	Gauge(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	Count(metric string, value float64, hostname string, tags []string)
	MonotonicCount(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
	Historate(metric string, value float64, hostname string, tags []string)
	ServiceCheck(checkName string, status ServiceCheckStatus, hostname string, tags []string, message string)
	Event(e Event)
}

// checkSender implements Sender
type checkSender struct {
	id              check.ID
	smsOut          chan<- senderMetricSample
	serviceCheckOut chan<- ServiceCheck
	eventOut        chan<- Event
}

type senderMetricSample struct {
	id           check.ID
	metricSample *MetricSample
	commit       bool
}

func newCheckSender(id check.ID, smsOut chan<- senderMetricSample, serviceCheckOut chan<- ServiceCheck, eventOut chan<- Event) *checkSender {
	return &checkSender{
		id:              id,
		smsOut:          smsOut,
		serviceCheckOut: serviceCheckOut,
		eventOut:        eventOut,
	}
}

// GetSender returns a Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func GetSender(id check.ID) (Sender, error) {
	if aggregatorInstance == nil {
		return nil, errors.New("Aggregator was not initialized")
	}

	err := aggregatorInstance.registerSender(id)
	return newCheckSender(id, aggregatorInstance.checkMetricIn, aggregatorInstance.serviceCheckIn, aggregatorInstance.eventIn), err
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func DestroySender(id check.ID) {
	aggregatorInstance.deregisterSender(id)
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
	s.smsOut <- senderMetricSample{s.id, &MetricSample{}, true}
}

func (s *checkSender) sendMetricSample(metric string, value float64, hostname string, tags []string, mType MetricType) {
	log.Debug(mType.String(), " sample: ", metric, ": ", value, " for hostname: ", hostname, " tags: ", tags)
	metricSample := &MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      mType,
		Tags:       &tags,
		Host:       hostname,
		SampleRate: 1,
		Timestamp:  time.Now().Unix(),
	}

	s.smsOut <- senderMetricSample{s.id, metricSample, false}
}

// Gauge should be used to send a simple gauge value to the aggregator. Only the last value sampled is kept at commit time.
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, GaugeType)
}

// Rate should be used to track the rate of a metric over each check run
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, RateType)
}

// Count should be used to count a number of events that occurred during the check run
func (s *checkSender) Count(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, CountType)
}

// MonotonicCount should be used to track the increase of a monotonic raw counter
func (s *checkSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, MonotonicCountType)
}

// Histogram should be used to track the statistical distribution of a set of values during a check run
// Should be called multiple times on the same (metric, hostname, tags) so that a distribution can be computed
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, HistogramType)
}

// Historate should be used to create a histogram metric for "rate" like metrics.
// Warning this doesn't use the harmonic mean, beware of what it means when using it.
func (s *checkSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, HistorateType)
}

// ServiceCheck submits a service check
func (s *checkSender) ServiceCheck(checkName string, status ServiceCheckStatus, hostname string, tags []string, message string) {
	log.Debug("Service check submitted: ", checkName, ": ", status.String(), " for hostname: ", hostname, " tags: ", tags)
	serviceCheck := ServiceCheck{
		CheckName: checkName,
		Status:    status,
		Host:      hostname,
		Ts:        time.Now().Unix(),
		Tags:      tags,
		Message:   message,
	}

	s.serviceCheckOut <- serviceCheck
}

// Event submits an event
func (s *checkSender) Event(e Event) {
	log.Debug("Event submitted: ", e.Title, " for hostname: ", e.Host, " tags: ", e.Tags)

	s.eventOut <- e
}
