package aggregator

import (

	// stdlib
	"bytes"
	"sort"

	"github.com/op/go-logging"
)

type Aggregator interface {
	Gauge(metric string, value float64, hostname string, tags *[]string)
	Rate(metric string, value float64, hostname string, tags *[]string)
	Flush() string
}

type Point struct {
	Timestamp int64
	Value     float32
}

type Serie struct {
	MetricName string
	Tags       *[]string
	Points     *[]Point
}

type DefaultAggregator struct {
	Series *map[string]Serie
}

var log = logging.MustGetLogger("datadog-agent")

func genKey(metric string, hostname string, tags *[]string) string {

	sort.Strings(*tags)

	var buffer bytes.Buffer
	buffer.WriteString(metric)
	buffer.WriteString(hostname)
	for _, tag := range *tags {
		buffer.WriteString(tag)
	}

	return buffer.String()

}

func (agg DefaultAggregator) Gauge(metric string, value float64, hostname string, tags *[]string) {
	key := genKey(metric, hostname, tags)
	log.Infof("Submitted GAUGE: %v = %v", key, value)

}

func (agg DefaultAggregator) Rate(metric string, value float64, hostname string, tags *[]string) {
	key := genKey(metric, hostname, tags)
	log.Infof("Submitted RATE: %v = %v", key, value)

}

func (agg DefaultAggregator) Flush() string {
	return "flushed!"

}
