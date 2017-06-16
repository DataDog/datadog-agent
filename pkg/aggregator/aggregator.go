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
	aggregatorInit.Do(func() {
		aggregatorInstance = NewBufferedAggregator(f, hostname)
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
	dogstatsdIn        chan *MetricSample
	checkMetricIn      chan senderMetricSample
	serviceCheckIn     chan ServiceCheck
	eventIn            chan Event
	sampler            TimeSampler
	checkSamplers      map[check.ID]*CheckSampler
	distSampler        DistSampler
	serviceChecks      []ServiceCheck
	events             []Event
	flushInterval      int64
	mu                 sync.Mutex // to protect the checkSamplers field
	forwarder          forwarder.Forwarder
	hostname           string
	hostnameUpdate     chan string
	hostnameUpdateDone chan struct{}    // signals that the hostname update is finished
	TickerChan         <-chan time.Time // For test/benchmark purposes: it allows the flush to be controlled from the outside
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(f forwarder.Forwarder, hostname string) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:        make(chan *MetricSample, 100),      // TODO make buffer size configurable
		checkMetricIn:      make(chan senderMetricSample, 100), // TODO make buffer size configurable
		serviceCheckIn:     make(chan ServiceCheck, 100),       // TODO make buffer size configurable
		eventIn:            make(chan Event, 100),              // TODO make buffer size configurable
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

// IsInputQueueEmpty returns true if every input channel for the aggregator are
// empty. This is mainly usefull for tests and benchmark
func (agg *BufferedAggregator) IsInputQueueEmpty() bool {
	if len(agg.checkMetricIn)+len(agg.serviceCheckIn)+len(agg.eventIn) == 0 {
		return true
	}
	return false
}

// GetChannels returns a channel which can be subsequently used to send MetricSamples, Event or ServiceCheck
func (agg *BufferedAggregator) GetChannels() (chan *MetricSample, chan Event, chan ServiceCheck) {
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
	event := Event{
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
func (agg *BufferedAggregator) addServiceCheck(sc ServiceCheck) {
	if sc.Host == "" {
		sc.Host = agg.hostname
	}
	if sc.Ts == 0 {
		sc.Ts = time.Now().Unix()
	}

	agg.serviceChecks = append(agg.serviceChecks, sc)
}

// addEvent adds the event to the slice of current events
func (agg *BufferedAggregator) addEvent(e Event) {
	if e.Host == "" {
		e.Host = agg.hostname
	}
	if e.Ts == 0 {
		e.Ts = time.Now().Unix()
	}

	agg.events = append(agg.events, e)
}

// addSample adds the metric sample to either the sampler or distSampler
func (agg *BufferedAggregator) addSample(metricSample *MetricSample, timestamp int64) {
	log.Infof("Adding sample of type %s", metricSample.Mtype)
	if metricSample.Mtype == DistributionType {
		log.Infof("Adding sample to Distribution %v", metricSample.Value)
		agg.distSampler.addSample(metricSample, timestamp)
	} else {
		agg.sampler.addSample(metricSample, timestamp)
	}
}

func (agg *BufferedAggregator) flushSeries() {
	start := time.Now()
	series := agg.sampler.flush(start.Unix())
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
		addFlushTime("ChecksMetricSampleFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("SeriesFlushed", int64(len(series)))
	}()
}

func (agg *BufferedAggregator) flushServiceChecks() {
	// Add a simple service check for the Agent status
	start := time.Now()
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
		addFlushTime("ServiceCheckFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("ServiceCheckFlushed", int64(len(serviceChecks)))
	}()
}

func (agg *BufferedAggregator) flushSketches() {
	start := time.Now()
	sketchSeries := agg.distSampler.flush(start.Unix())

	if len(sketchSeries) == 0 {
		return
	}

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(sketchSeries), " sketches to the forwarder")
		payload, err := MarshalJSONSketchSeries(sketchSeries)
		if err != nil {
			log.Error("could not serialize sketches, dropping them:", err)
		}
		agg.forwarder.SubmitV1SketchSeries(config.Datadog.GetString("api_key"), &payload)
		log.Infof("Forwarding sketch %v", string(payload))
		addFlushTime("MetricSketchFlushTIme", int64(time.Since(start)))
		aggregatorExpvar.Add("SketchesFlushed", int64(len(sketchSeries)))
	}()
}

func (agg *BufferedAggregator) flushEvents() {
	// Clear the current event slice
	start := time.Now()
	events := agg.events
	agg.events = nil

	if len(events) == 0 {
		return
	}

	// Serialize and forward in a separate goroutine
	go func(hostname string) {
		log.Debug("Flushing ", len(events), " events to the forwarder")
		payload, err := MarshalJSONEvents(events, config.Datadog.GetString("api_key"), hostname)
		if err != nil {
			log.Error("could not serialize events, dropping them: ", err)
			return
		}
		// We use the agent 5 intake endpoint until the v2 events endpoint is ready
		agg.forwarder.SubmitV1Intake(config.Datadog.GetString("api_key"), &payload)
		addFlushTime("EventFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("EventsFlushed", int64(len(events)))
	}(agg.hostname)
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
			agg.flushSeries()
			agg.flushSketches()
			agg.flushServiceChecks()
			agg.flushEvents()
			addFlushTime("MainFlushTime", int64(time.Since(start)))
			aggregatorExpvar.Add("NumberOfFlush", 1)
		case sample := <-agg.dogstatsdIn:
			aggregatorExpvar.Add("DogstatsdMetricSample", 1)
			now := time.Now().Unix()
			agg.addSample(sample, now)
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
