package aggregator

import (
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const defaultFlushInterval = 15 // flush interval in seconds
const bucketSize = 10           // fixed for now

var aggregatorInstance *BufferedAggregator
var aggregatorInit sync.Once

// GetAggregator returns the Singleton instance
func GetAggregator() *BufferedAggregator {
	aggregatorInit.Do(func() {
		aggregatorInstance = newBufferedAggregator()
	})

	return aggregatorInstance
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	dogstatsdIn   chan *MetricSample
	checkIn       chan senderSample
	sampler       Sampler
	checkSamplers map[check.ID]*CheckSampler
	flushInterval int64
	mu            sync.Mutex // to protect the checkSamplers field
}

// Instantiate a BufferedAggregator and run it
func newBufferedAggregator() *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:   make(chan *MetricSample, 100), // TODO make buffer size configurable
		checkIn:       make(chan senderSample, 100),  // TODO make buffer size configurable
		sampler:       *NewSampler(bucketSize),
		checkSamplers: make(map[check.ID]*CheckSampler),
		flushInterval: defaultFlushInterval,
	}

	go aggregator.run()

	return aggregator
}

// GetChannel returns a channel which can be subsequently used to send MetricSamples
func (agg *BufferedAggregator) GetChannel() chan *MetricSample {
	return agg.dogstatsdIn
}

func (agg *BufferedAggregator) registerSender(id check.ID) error {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	if _, ok := agg.checkSamplers[id]; ok {
		return fmt.Errorf("Sender with check ID %d has already been registered", id)
	}
	agg.checkSamplers[id] = newCheckSampler(config.Datadog.GetString("hostname"))
	return nil
}

func (agg *BufferedAggregator) deregisterSender(id check.ID) {
	agg.mu.Lock()
	delete(agg.checkSamplers, id)
	agg.mu.Unlock()
}

func (agg *BufferedAggregator) handleSenderSample(ss senderSample) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	if checkSampler, ok := agg.checkSamplers[ss.id]; ok {
		if ss.commit {
			now := time.Now().Unix()
			checkSampler.commit(now)
		} else {
			checkSampler.addSample(ss.metricSample)
		}
	} else {
		log.Debugf("CheckSampler with ID %d doesn't exist, can't handle senderSample", ss.id)
	}
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
		case sample := <-agg.dogstatsdIn:
			now := time.Now().Unix()
			agg.sampler.addSample(sample, now)
		case ss := <-agg.checkIn:
			agg.handleSenderSample(ss)
		}
	}
}
