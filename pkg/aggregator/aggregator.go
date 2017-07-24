package aggregator

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const defaultFlushInterval = 15 // flush interval in seconds
const bucketSize = 10           // fixed for now

// Stats collect several flush duration allowing computation like median or percentiles
type Stats struct {
	FlushTime     [32]int64 // circular buffer of recent run durations
	FlushIndex    int       // last flush time position in circular buffer
	LastFlushTime int64     // most recent flush duration, provided for convenience
	Name          string
	m             sync.Mutex
}

func (s *Stats) add(duration int64) {
	s.m.Lock()
	defer s.m.Unlock()

	s.FlushIndex = (s.FlushIndex + 1) % 32
	s.FlushTime[s.FlushIndex] = duration
	s.LastFlushTime = duration
}

func newStats(name string) {
	flushStats[name] = &Stats{Name: name, FlushIndex: -1}
}

func addFlushTime(name string, value int64) {
	flushStats[name].add(value)
}

func expStats() interface{} {
	return flushStats
}

func timeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second) // Unix time with nanosecond precision
}

var (
	aggregatorInstance *BufferedAggregator
	aggregatorInit     sync.Once

	aggregatorExpvar = expvar.NewMap("aggregator")
	flushStats       = make(map[string]*Stats)
)

func init() {
	newStats("ChecksMetricSampleFlushTime")
	newStats("ServiceCheckFlushTime")
	newStats("EventFlushTime")
	newStats("MainFlushTime")
	newStats("MetricSketchFlushTime")
	aggregatorExpvar.Set("Flush", expvar.Func(expStats))
}

// InitAggregator returns the Singleton instance
func InitAggregator(f forwarder.Forwarder, hostname string) *BufferedAggregator {
	return InitAggregatorWithFlushInterval(f, hostname, defaultFlushInterval)
}

// InitAggregatorWithFlushInterval returns the Singleton instance with a configured flush interval
func InitAggregatorWithFlushInterval(f forwarder.Forwarder, hostname string, flushInterval int64) *BufferedAggregator {
	aggregatorInit.Do(func() {
		aggregatorInstance = NewBufferedAggregator(f, hostname, flushInterval)
		go aggregatorInstance.run()
	})

	return aggregatorInstance
}

// SetDefaultAggregator allows to force a custom Aggregator as the default one and run it.
// This is usefull for testing or benchmarking.
func SetDefaultAggregator(agg *BufferedAggregator) {
	aggregatorInstance = agg
	go aggregatorInstance.run()
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	dogstatsdIn        chan *metrics.MetricSample
	checkMetricIn      chan senderMetricSample
	serviceCheckIn     chan metrics.ServiceCheck
	eventIn            chan metrics.Event
	sampler            TimeSampler
	checkSamplers      map[check.ID]*CheckSampler
	distSampler        DistSampler
	serviceChecks      []metrics.ServiceCheck
	events             []metrics.Event
	flushInterval      int64
	mu                 sync.Mutex // to protect the checkSamplers field
	forwarder          forwarder.Forwarder
	hostname           string
	hostnameUpdate     chan string
	hostnameUpdateDone chan struct{}    // signals that the hostname update is finished
	TickerChan         <-chan time.Time // For test/benchmark purposes: it allows the flush to be controlled from the outside
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(f forwarder.Forwarder, hostname string, flushInterval int64) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:        make(chan *metrics.MetricSample, 100), // TODO make buffer size configurable
		checkMetricIn:      make(chan senderMetricSample, 100),    // TODO make buffer size configurable
		serviceCheckIn:     make(chan metrics.ServiceCheck, 100),  // TODO make buffer size configurable
		eventIn:            make(chan metrics.Event, 100),         // TODO make buffer size configurable
		sampler:            *NewTimeSampler(bucketSize, hostname),
		checkSamplers:      make(map[check.ID]*CheckSampler),
		distSampler:        *NewDistSampler(bucketSize, hostname),
		flushInterval:      defaultFlushInterval,
		forwarder:          f,
		hostname:           hostname,
		hostnameUpdate:     make(chan string),
		hostnameUpdateDone: make(chan struct{}),
	}

	return aggregator
}

func deduplicateTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	idx := 0
	for _, v := range tags {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = true
		tags[idx] = v
		idx++
	}
	return tags[:idx]
}

// IsInputQueueEmpty returns true if every input channel for the aggregator are
// empty. This is mainly usefull for tests and benchmark
func (agg *BufferedAggregator) IsInputQueueEmpty() bool {
	if len(agg.checkMetricIn)+len(agg.serviceCheckIn)+len(agg.eventIn) == 0 {
		return true
	}
	return false
}

// GetChannels returns a channel which can be subsequently used to send MetricSamples, Event or ServiceCheck
func (agg *BufferedAggregator) GetChannels() (chan *metrics.MetricSample, chan metrics.Event, chan metrics.ServiceCheck) {
	return agg.dogstatsdIn, agg.eventIn, agg.serviceCheckIn
}

// SetHostname sets the hostname that the aggregator uses by default on all the data it sends
// Blocks until the main aggregator goroutine has finished handling the update
func (agg *BufferedAggregator) SetHostname(hostname string) {
	agg.hostnameUpdate <- hostname
	<-agg.hostnameUpdateDone
}

// AddAgentStartupEvent adds the startup event to the events that'll be sent on the next flush
func (agg *BufferedAggregator) AddAgentStartupEvent(agentVersion string) {
	event := metrics.Event{
		Text:           fmt.Sprintf("Version %s", agentVersion),
		SourceTypeName: "System",
		EventType:      "Agent Startup",
	}
	agg.eventIn <- event
}

func (agg *BufferedAggregator) registerSender(id check.ID) error {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	if _, ok := agg.checkSamplers[id]; ok {
		return fmt.Errorf("Sender with ID '%s' has already been registered, will use existing sampler", id)
	}
	agg.checkSamplers[id] = newCheckSampler(agg.hostname)
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
			checkSampler.commit(timeNowNano())
		} else {
			ss.metricSample.Tags = deduplicateTags(ss.metricSample.Tags)
			checkSampler.addSample(ss.metricSample)
		}
	} else {
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle senderMetricSample", ss.id)
	}
}

// addServiceCheck adds the service check to the slice of current service checks
func (agg *BufferedAggregator) addServiceCheck(sc metrics.ServiceCheck) {
	if sc.Host == "" {
		sc.Host = agg.hostname
	}
	if sc.Ts == 0 {
		sc.Ts = time.Now().Unix()
	}
	sc.Tags = deduplicateTags(sc.Tags)

	agg.serviceChecks = append(agg.serviceChecks, sc)
}

// addEvent adds the event to the slice of current events
func (agg *BufferedAggregator) addEvent(e metrics.Event) {
	if e.Host == "" {
		e.Host = agg.hostname
	}
	if e.Ts == 0 {
		e.Ts = time.Now().Unix()
	}
	e.Tags = deduplicateTags(e.Tags)

	agg.events = append(agg.events, e)
}

// addSample adds the metric sample to either the sampler or distSampler
func (agg *BufferedAggregator) addSample(metricSample *metrics.MetricSample, timestamp float64) {
	metricSample.Tags = deduplicateTags(metricSample.Tags)
	if metricSample.Mtype == metrics.DistributionType {
		agg.distSampler.addSample(metricSample, timestamp)
	} else {
		agg.sampler.addSample(metricSample, timestamp)
	}
}

// GetSeries grabs all the series from the queue and clears the queue
func (agg *BufferedAggregator) GetSeries() []*Serie {
	series := agg.sampler.flush(timeNowNano())
	agg.mu.Lock()
	for _, checkSampler := range agg.checkSamplers {
		series = append(series, checkSampler.flush()...)
	}
	agg.mu.Unlock()
	return series
}

func (agg *BufferedAggregator) flushSeries() {
	start := time.Now()
	series := agg.GetSeries()

	if len(series) == 0 {
		return
	}

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(series), " series to the forwarder")
		payload, _, err := serializer.MarshalJSONSeries(series)
		if err != nil {
			log.Error("could not serialize series, dropping it:", err)
			return
		}
		agg.forwarder.SubmitV1Series(&payload)
		addFlushTime("ChecksMetricSampleFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("SeriesFlushed", int64(len(series)))
	}()
}

// GetServiceChecks grabs all the service checks from the queue and clears the queue
func (agg *BufferedAggregator) GetServiceChecks() []ServiceCheck {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	// Clear the current service check slice
	serviceChecks := agg.serviceChecks
	agg.serviceChecks = nil
	return serviceChecks
}

func (agg *BufferedAggregator) flushServiceChecks() {
	// Add a simple service check for the Agent status
	start := time.Now()
	agg.addServiceCheck(metrics.ServiceCheck{
		CheckName: "datadog.agent.up",
		Status:    metrics.ServiceCheckOK,
	})

	// Clear the current service check slice
	serviceChecks := agg.serviceChecks
	agg.serviceChecks = nil

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(serviceChecks), " service checks to the forwarder")
		payload, _, err := serializer.MarshalJSONServiceChecks(serviceChecks)
		if err != nil {
			log.Error("could not serialize service checks, dropping them: ", err)
			return
		}
		agg.forwarder.SubmitV1CheckRuns(&payload)
		addFlushTime("ServiceCheckFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("ServiceCheckFlushed", int64(len(serviceChecks)))
	}()
}

// GetSketches grabs all the sketches from the queue and clears the queue
func (agg *BufferedAggregator) GetSketches() []*percentile.SketchSeries {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	return agg.distSampler.flush(timeNowNano())
}

func (agg *BufferedAggregator) flushSketches() {
	// Serialize and forward in a separate goroutine
	start := time.Now()
	sketchSeries := agg.GetSketches()
	if len(sketchSeries) == 0 {
		return
	}
	go func() {
		log.Debug("Flushing ", len(sketchSeries), " sketches to the forwarder")
		// Serialize with Protocol Buffer and use v2 endpoint
		payload, _, err := serializer.MarshalSketchSeries(sketchSeries)
		if err != nil {
			log.Error("could not serialize sketches, dropping them:", err)
		}
		agg.forwarder.SubmitSketchSeries(&payload)
		addFlushTime("MetricSketchFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("SketchesFlushed", int64(len(sketchSeries)))
	}()
}

// GetEvents grabs the events from the queue and clears it
func (agg *BufferedAggregator) GetEvents() []Event {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	events := agg.events
	agg.events = nil
	return events
}

// flushEvents serializes and forwards events in a separate goroutine
func (agg *BufferedAggregator) flushEvents() {
	// Serialize and forward in a separate goroutine
	start := time.Now()
	events := agg.GetEvents()
	if len(events) == 0 {
		return
	}
	go func(hostname string) {
		log.Debug("Flushing ", len(events), " events to the forwarder")
		payload, _, err := serializer.MarshalJSONEvents(events, config.Datadog.GetString("api_key"), hostname)
		if err != nil {
			log.Error("could not serialize events, dropping them: ", err)
			return
		}
		// We use the agent 5 intake endpoint until the v2 events endpoint is ready
		agg.forwarder.SubmitV1Intake(&payload)
		addFlushTime("EventFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("EventsFlushed", int64(len(events)))
	}(agg.hostname)
}

func (agg *BufferedAggregator) flush() {
	agg.flushSeries()
	agg.flushSketches()
	agg.flushServiceChecks()
	agg.flushEvents()
}

func (agg *BufferedAggregator) run() {
	if agg.TickerChan == nil {
		flushPeriod := time.Duration(agg.flushInterval) * time.Second
		agg.TickerChan = time.NewTicker(flushPeriod).C
	}
	for {
		select {
		case <-agg.TickerChan:
			start := time.Now()
			agg.flush()
			addFlushTime("MainFlushTime", int64(time.Since(start)))
			aggregatorExpvar.Add("NumberOfFlush", 1)
		case sample := <-agg.dogstatsdIn:
			aggregatorExpvar.Add("DogstatsdMetricSample", 1)
			agg.addSample(sample, timeNowNano())
		case ss := <-agg.checkMetricIn:
			aggregatorExpvar.Add("ChecksMetricSample", 1)
			agg.handleSenderSample(ss)
		case sc := <-agg.serviceCheckIn:
			aggregatorExpvar.Add("ServiceCheck", 1)
			agg.addServiceCheck(sc)
		case e := <-agg.eventIn:
			aggregatorExpvar.Add("Event", 1)
			agg.addEvent(e)
		case h := <-agg.hostnameUpdate:
			aggregatorExpvar.Add("HostnameUpdate", 1)
			agg.hostname = h
			agg.mu.Lock()
			for _, checkSampler := range agg.checkSamplers {
				checkSampler.defaultHostname = h
			}
			agg.sampler.defaultHostname = h
			agg.mu.Unlock()
			agg.hostnameUpdateDone <- struct{}{}
		}
	}
}
