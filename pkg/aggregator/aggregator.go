package aggregator

import (
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/op/go-logging"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

const FLUSH_INTERVAL = 15 // flush interval in seconds

var log = logging.MustGetLogger("datadog-agent")

var _aggregator *BufferedAggregator

// Interface
type Sender interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
}

func GetSender(interval int64) Sender {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator(FLUSH_INTERVAL)
	}
	return &IntervalAggregator{_aggregator, interval}
}

func GetChannel() chan *dogstatsd.MetricSample {
	if _aggregator == nil {
		_aggregator = newBufferedAggregator(FLUSH_INTERVAL)
	}

	return _aggregator.in
}

func (agg *UnbufferedAggregator) Gauge(metric string, value float64, hostname string, tags []string) {
	if err := agg.client.Gauge(metric, value, tags, 1); err != nil {
		log.Errorf("Error posting gauge %s: %v", metric, err)
	}
}

func (agg *UnbufferedAggregator) Histogram(metric string, value float64, hostname string, tags []string) {
	if err := agg.client.Histogram(metric, value, tags, 1); err != nil {
		log.Errorf("Error histogram %s: %v", metric, err)
	}
}

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
