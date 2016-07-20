package aggregator

import (
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/op/go-logging"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

const defaultFlushInterval = 15 // flush interval in seconds

var log = logging.MustGetLogger("datadog-agent")

var _aggregator *BufferedAggregator

// Sender is the interface that allows sending metrics from checks
type Sender interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
}

// GetSender returns a Sender that aggregates according to the passed interval
func GetSender(interval int64) Sender {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator(defaultFlushInterval)
	}
	return &IntervalAggregator{_aggregator, interval}
}

// GetChannel returns a channel which can be subsequently used to send MetricSamples
func GetChannel() chan *dogstatsd.MetricSample {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator(defaultFlushInterval)
	}

	return _aggregator.in
}

// Gauge implements the Sender interface
func (agg *UnbufferedAggregator) Gauge(metric string, value float64, hostname string, tags []string) {
	if err := agg.client.Gauge(metric, value, tags, 1); err != nil {
		log.Errorf("Error posting gauge %s: %v", metric, err)
	}
}

// Histogram implements the Sender interface
func (agg *UnbufferedAggregator) Histogram(metric string, value float64, hostname string, tags []string) {
	if err := agg.client.Histogram(metric, value, tags, 1); err != nil {
		log.Errorf("Error histogram %s: %v", metric, err)
	}
}

// Gauge implements the Sender interface
func (ia *IntervalAggregator) Gauge(metric string, value float64, hostname string, tags []string) {
	metricSample := &dogstatsd.MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      dogstatsd.Gauge,
		Tags:       &tags,
		SampleRate: 1,
		Interval:   ia.checkInterval,
	}
	ia.aggregator.in <- metricSample
}

// Histogram implements the Sender interface
func (ia *IntervalAggregator) Histogram(metric string, value float64, hostname string, tags []string) {
	// TODO
}

// Implementation

// UnbufferedAggregator is special aggregator that doesn't aggregate anything,
// it just forward metrics to DogStatsD
type UnbufferedAggregator struct {
	client *statsd.Client
}

// BufferedAggregator aggregates metrics in buckets which intervals are determined
// by the checks' intervals
type BufferedAggregator struct {
	in            chan *dogstatsd.MetricSample
	sampler       Sampler
	flushInterval int64
}

// IntervalAggregator is a wrapper around BufferedAggregator, and implements the
// ChecksAggregator interface
type IntervalAggregator struct {
	aggregator    *BufferedAggregator
	checkInterval int64
}

// NewUnbufferedAggregator returns a newly initialized UnbufferedAggregator
func NewUnbufferedAggregator() *UnbufferedAggregator {
	c, err := statsd.New("127.0.0.1:8125")
	if err != nil {
		panic(err)
	}
	c.Namespace = "agent6."

	return &UnbufferedAggregator{c}
}

// Instantiate a BufferedAggregator and run it
func newBufferedAggregator(flushInterval int64) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		make(chan *dogstatsd.MetricSample, 100), // TODO make buffer size configurable
		*NewSampler(),
		flushInterval,
	}

	go aggregator.run()

	return aggregator
}

func (a *BufferedAggregator) run() {
	flushPeriod := time.Duration(a.flushInterval) * time.Second
	flushTicker := time.NewTicker(flushPeriod)
	for {
		select {
		case <-flushTicker.C:
			now := time.Now().Unix()
			go Report(a.sampler.flush(now))
		case sample := <-a.in:
			now := time.Now().Unix()
			a.sampler.addSample(sample, now)
		}
	}
}
