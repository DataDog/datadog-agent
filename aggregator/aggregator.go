package aggregator

import (
	"github.com/DataDog/datadog-go/statsd"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("datadog-agent")

type Aggregator interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
	Flush() string
}

// UnbufferedAggregator is special aggregator that doesn't aggregate anything,
// it just forward metrics to DogStatsD
type UnbufferedAggregator struct {
	client *statsd.Client
}

func NewUnbufferedAggregator() *UnbufferedAggregator {
	c, err := statsd.New("127.0.0.1:8125")
	if err != nil {
		panic(err)
	}
	c.Namespace = "agent6."

	return &UnbufferedAggregator{c}
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

func (agg UnbufferedAggregator) Flush() string {
	return "" // noop
}
