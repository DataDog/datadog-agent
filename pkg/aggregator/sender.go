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
}

// checkSender implements Sender
type checkSender struct {
	id    check.ID
	ssOut chan<- senderSample
}

type senderSample struct {
	id           check.ID
	metricSample *MetricSample
	commit       bool
}

func newCheckSender(id check.ID, ssOut chan<- senderSample) *checkSender {
	return &checkSender{
		id:    id,
		ssOut: ssOut,
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
	return newCheckSender(id, aggregatorInstance.checkIn), err
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
		senderInstance = newCheckSender(defaultCheckID, aggregatorInstance.checkIn)
	})

	return senderInstance, nil
}

// Commit commits the metric samples that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	s.ssOut <- senderSample{s.id, &MetricSample{}, true}
}

func (s *checkSender) sendSample(metric string, value float64, hostname string, tags []string, mType MetricType) {
	log.Debug(mType.String(), " sample: ", metric, ": ", value, " for hostname: ", hostname, " tags: ", tags)
	metricSample := &MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      mType,
		Tags:       &tags,
		SampleRate: 1,
		Timestamp:  time.Now().Unix(),
	}

	s.ssOut <- senderSample{s.id, metricSample, false}
}

// Gauge implements the Sender interface
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.sendSample(metric, value, hostname, tags, GaugeType)
}

// Rate implements the Sender interface
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.sendSample(metric, value, hostname, tags, RateType)
}

// Count implements the Sender interface
func (s *checkSender) Count(metric string, value float64, hostname string, tags []string) {
	s.sendSample(metric, value, hostname, tags, CountType)
}

// MonotonicCount implements the Sender interface
func (s *checkSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.sendSample(metric, value, hostname, tags, MonotonicCountType)
}

// Histogram implements the Sender interface
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendSample(metric, value, hostname, tags, HistogramType)
}
