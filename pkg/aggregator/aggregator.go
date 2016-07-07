package aggregator

import (
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/op/go-logging"
)

const defaultFlushInterval = 15 // flush interval in seconds
const bucketSize = 10           // fixed for now

var log = logging.MustGetLogger("datadog-agent")

var _aggregator *BufferedAggregator

// Sender is the interface that allows sending metrics from checks
type Sender interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
}

// GetSender returns a Sender
func GetSender() Sender {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator()
	}
	return _aggregator
}

// GetChannel returns a channel which can be subsequently used to send MetricSamples
func GetChannel() chan *MetricSample {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator()
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
func (agg *BufferedAggregator) Gauge(metric string, value float64, hostname string, tags []string) {
	metricSample := &MetricSample{
		Name:       metric,
		Value:      value,
		Mtype:      GaugeType,
		Tags:       &tags,
		SampleRate: 1,
		Timestamp:  time.Now().Unix(),
	}
	agg.in <- metricSample
}

// Histogram implements the Sender interface
func (agg *BufferedAggregator) Histogram(metric string, value float64, hostname string, tags []string) {
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
	in            chan *MetricSample
	sampler       Sampler
	flushInterval int64
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
func newBufferedAggregator() *BufferedAggregator {
	aggregator := &BufferedAggregator{
		make(chan *MetricSample, 100), // TODO make buffer size configurable
		*NewSampler(bucketSize),
		defaultFlushInterval,
	}

	go aggregator.run()

	return aggregator
}

func (agg *BufferedAggregator) run() {
	flushPeriod := time.Duration(agg.flushInterval) * time.Second
	flushTicker := time.NewTicker(flushPeriod)
	for {
		select {
		case <-flushTicker.C:
			now := time.Now().Unix()
			go Report(agg.sampler.flush(now))
		case sample := <-agg.in:
			now := time.Now().Unix()
			agg.sampler.addSample(sample, now)
		}
	}
}
