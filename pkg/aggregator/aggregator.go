package aggregator

import (
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
)

const defaultFlushInterval = 15 // flush interval in seconds
const bucketSize = 10           // fixed for now

var aggregatorInstance *BufferedAggregator
var aggregatorInit sync.Once

// InitAggregator returns the Singleton instance
func InitAggregator(f *forwarder.Forwarder) *BufferedAggregator {
	aggregatorInit.Do(func() {
		aggregatorInstance = newBufferedAggregator(f)
	})

	return aggregatorInstance
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	dogstatsdIn    chan *MetricSample
	checkMetricIn  chan senderMetricSample
	serviceCheckIn chan ServiceCheck
	sampler        Sampler
	checkSamplers  map[check.ID]*CheckSampler
	serviceChecks  []ServiceCheck
	flushInterval  int64
	mu             sync.Mutex // to protect the checkSamplers field
	forwarder      *forwarder.Forwarder
}

// Instantiate a BufferedAggregator and run it
func newBufferedAggregator(f *forwarder.Forwarder) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:    make(chan *MetricSample, 100),      // TODO make buffer size configurable
		checkMetricIn:  make(chan senderMetricSample, 100), // TODO make buffer size configurable
		serviceCheckIn: make(chan ServiceCheck, 100),       // TODO make buffer size configurable
		sampler:        *NewSampler(bucketSize),
		checkSamplers:  make(map[check.ID]*CheckSampler),
		flushInterval:  defaultFlushInterval,
		forwarder:      f,
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
		return fmt.Errorf("Sender with ID '%s' has already been registered, will use existing sampler", id)
	}
	agg.checkSamplers[id] = newCheckSampler(config.Datadog.GetString("hostname"))
	return nil
}

func (agg *BufferedAggregator) deregisterSender(id check.ID) {
	agg.mu.Lock()
	delete(agg.checkSamplers, id)
	agg.mu.Unlock()
}

func (agg *BufferedAggregator) handleSenderSample(ss senderMetricSample) {
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
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle senderMetricSample", ss.id)
	}
}

// addServiceCheck adds the service check to the slice of current service checks
// FIXME: the default hostname should be the one that's resolved by the Agent logic instead of the one pulled from the main config
func (agg *BufferedAggregator) addServiceCheck(sc ServiceCheck) {
	if sc.Host == "" {
		sc.Host = config.Datadog.GetString("hostname")
	}
	if sc.Ts == 0 {
		sc.Ts = time.Now().Unix()
	}

	agg.serviceChecks = append(agg.serviceChecks, sc)
}

func (agg *BufferedAggregator) flushSeries() {
	now := time.Now().Unix()
	series := agg.sampler.flush(now)
	agg.mu.Lock()
	for _, checkSampler := range agg.checkSamplers {
		series = append(series, checkSampler.flush()...)
	}
	agg.mu.Unlock()

	if len(series) == 0 {
		return
	}

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(series), " series to the forwarder")
		payload, err := MarshalJSONSeries(series)
		if err != nil {
			log.Error("could not serialize series, dropping it:", err)
			return
		}
		agg.forwarder.SubmitV1Series(config.Datadog.GetString("api_key"), &payload)
	}()
}

func (agg *BufferedAggregator) flushServiceChecks() {
	// Add a simple service check for the Agent status
	agg.addServiceCheck(ServiceCheck{
		CheckName: "datadog.agent.up",
		Status:    ServiceCheckOK,
	})

	// Clear the current service check slice
	serviceChecks := agg.serviceChecks
	agg.serviceChecks = nil

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(serviceChecks), " service checks to the forwarder")
		payload, err := MarshalJSONServiceChecks(serviceChecks)
		if err != nil {
			log.Error("could not serialize service checks, dropping them: ", err)
			return
		}
		agg.forwarder.SubmitV1CheckRuns(config.Datadog.GetString("api_key"), &payload)
	}()
}

func (agg *BufferedAggregator) run() {
	flushPeriod := time.Duration(agg.flushInterval) * time.Second
	flushTicker := time.NewTicker(flushPeriod)
	for {
		select {
		case <-flushTicker.C:
			agg.flushSeries()
			agg.flushServiceChecks()
		case sample := <-agg.dogstatsdIn:
			now := time.Now().Unix()
			agg.sampler.addSample(sample, now)
		case ss := <-agg.checkMetricIn:
			agg.handleSenderSample(ss)
		case sc := <-agg.serviceCheckIn:
			agg.addServiceCheck(sc)
		}
	}
}
