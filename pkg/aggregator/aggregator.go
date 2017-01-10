package aggregator

import (
	"github.com/DataDog/datadog-agent/pb"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
)

const defaultFlushInterval = 15 // flush interval in seconds
const bucketSize = 10           // fixed for now

var aggregatorInstance *BufferedAggregator
var aggregatorInit sync.Once

// GetAggregator returns the Singleton instance
func GetAggregator(forwarder *forwarder.Forwarder) *BufferedAggregator {
	aggregatorInit.Do(func() {
		aggregatorInstance = newBufferedAggregator(forwarder)
	})

	return aggregatorInstance
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	dogstatsdIn           chan *MetricSample
	checkIn               chan senderSample
	sampler               Sampler
	checkSamplers         map[int64]*CheckSampler
	currentCheckSamplerID int64
	flushInterval         int64
	mu                    sync.Mutex // to protect the checkSamplers field
	forwarder             *forwarder.Forwarder
}

// Instantiate a BufferedAggregator and run it
func newBufferedAggregator(forwarder *forwarder.Forwarder) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:   make(chan *MetricSample, 100), // TODO make buffer size configurable
		checkIn:       make(chan senderSample, 100),  // TODO make buffer size configurable
		sampler:       *NewSampler(bucketSize),
		checkSamplers: make(map[int64]*CheckSampler),
		flushInterval: defaultFlushInterval,
		forwarder:     forwarder,
	}

	go aggregator.run()

	return aggregator
}

// GetChannel returns a channel which can be subsequently used to send MetricSamples
func (agg *BufferedAggregator) GetChannel() chan *MetricSample {
	return agg.dogstatsdIn
}

func (agg *BufferedAggregator) registerNewCheckSampler() int64 {
	agg.mu.Lock()
	agg.currentCheckSamplerID++
	agg.checkSamplers[agg.currentCheckSamplerID] = newCheckSampler(config.Datadog.GetString("hostname"))
	agg.mu.Unlock()

	return agg.currentCheckSamplerID
}

func (agg *BufferedAggregator) deregisterCheckSampler(checkSamplerID int64) {
	agg.mu.Lock()
	delete(agg.checkSamplers, checkSamplerID)
	agg.mu.Unlock()
}

func (agg *BufferedAggregator) run() {
	flushPeriod := time.Duration(agg.flushInterval) * time.Second
	flushTicker := time.NewTicker(flushPeriod)
	for {
		select {
		case <-flushTicker.C:
			now := time.Now().Unix()
			series := agg.sampler.flush(now)
			agg.mu.Lock()
			for _, checkSampler := range agg.checkSamplers {
				series = append(series, checkSampler.flush()...)
			}
			agg.mu.Unlock()
			go Report(series, config.Datadog.GetString("api_key"))

			p := make([]*pb.MetricsPayload_Sample, 1)
			tags := make([]string, 2)
			tags[0] = "BAR"
			tags[1] = "BUZ"
			p[0] = &pb.MetricsPayload_Sample{
				Metric: "foo",
				Tags:   tags,
			}

			agg.forwarder.SubmitSeries(&pb.MetricsPayload{
				Samples: p,
			})

		case sample := <-agg.dogstatsdIn:
			now := time.Now().Unix()
			agg.sampler.addSample(sample, now)
		case ss := <-agg.checkIn:
			if ss.commit {
				now := time.Now().Unix()
				agg.checkSamplers[ss.checkSamplerID].commit(now)
			} else {
				agg.checkSamplers[ss.checkSamplerID].addSample(ss.metricSample)
			}
		}
	}
}
