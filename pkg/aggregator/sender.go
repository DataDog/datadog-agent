package aggregator

import "time"

var _sender *checkSender

// Sender allows sending metrics from checks/a check
type Sender interface {
	Commit()
	Destroy()
	Gauge(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
}

// checkSender implements Sender
type checkSender struct {
	checkSamplerID int64
	ssOut          chan<- senderSample
}

type senderSample struct {
	checkSamplerID int64
	metricSample   *MetricSample
	commit         bool
}

func newCheckSender(checkSamplerID int64, ssOut chan<- senderSample) *checkSender {
	return &checkSender{
		checkSamplerID: checkSamplerID,
		ssOut:          ssOut,
	}
}

// GetSender returns a new Sender, properly registered with the aggregator
func GetSender() Sender {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator()
	}

	sender := newCheckSender(_aggregator.registerNewCheckSampler(), _aggregator.checkIn)

	return sender
}

// GetDefaultSender returns the default sender
func GetDefaultSender() Sender {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator()
	}
	if _sender == nil {
		_sender = newCheckSender(_aggregator.registerNewCheckSampler(), _aggregator.checkIn)
	}

	return _sender
}

// Commit commits the metric samples that were added during a check run
// Should be called at the end of every check run
func (s *checkSender) Commit() {
	s.ssOut <- senderSample{s.checkSamplerID, &MetricSample{}, true}
}

// Destroy frees up the resources used by the sender (by deregistering it from the aggregator)
// Should be called when the sender is not used anymore
// The metrics of this sender that haven't been flushed yet will be lost
func (s *checkSender) Destroy() {
	_aggregator.deregisterCheckSampler(s.checkSamplerID)
}

// Gauge implements the Sender interface
func (s *checkSender) Gauge(metric string, value float64, hostname string, tags []string) {
	metricSample := &MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      GaugeType,
		Tags:       &tags,
		SampleRate: 1,
		Timestamp:  time.Now().Unix(),
	}

	s.ssOut <- senderSample{s.checkSamplerID, metricSample, false}
}

// Rate implements the Sender interface
func (s *checkSender) Rate(metric string, value float64, hostname string, tags []string) {
	metricSample := &MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      RateType,
		Tags:       &tags,
		SampleRate: 1,
		Timestamp:  time.Now().Unix(),
	}

	s.ssOut <- senderSample{s.checkSamplerID, metricSample, false}
}

// Histogram implements the Sender interface
func (s *checkSender) Histogram(metric string, value float64, hostname string, tags []string) {
	// TODO
}
