// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// DefaultFlushInterval aggregator default flush interval
const DefaultFlushInterval = 15 * time.Second // flush interval
const bucketSize = 10                         // fixed for now

// Stats stores a statistic from several past flushes allowing computations like median or percentiles
type Stats struct {
	Flushes    [32]int64 // circular buffer of recent flushes stat
	FlushIndex int       // last flush position in circular buffer
	LastFlush  int64     // most recent flush stat, provided for convenience
	Name       string
	m          sync.Mutex
}

func (s *Stats) add(stat int64) {
	s.m.Lock()
	defer s.m.Unlock()

	s.FlushIndex = (s.FlushIndex + 1) % 32
	s.Flushes[s.FlushIndex] = stat
	s.LastFlush = stat
}

func newFlushTimeStats(name string) {
	flushTimeStats[name] = &Stats{Name: name, FlushIndex: -1}
}

func addFlushTime(name string, value int64) {
	flushTimeStats[name].add(value)
}

func newFlushCountStats(name string) {
	flushCountStats[name] = &Stats{Name: name, FlushIndex: -1}
}

func addFlushCount(name string, value int64) {
	flushCountStats[name].add(value)
}

func expStatsMap(statsMap map[string]*Stats) func() interface{} {
	return func() interface{} {
		return statsMap
	}
}

func timeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second) // Unix time with nanosecond precision
}

var (
	aggregatorInstance *BufferedAggregator
	aggregatorInit     sync.Once

	aggregatorExpvar = expvar.NewMap("aggregator")
	flushTimeStats   = make(map[string]*Stats)
	flushCountStats  = make(map[string]*Stats)
)

func init() {
	newFlushTimeStats("ChecksMetricSampleFlushTime")
	newFlushTimeStats("ServiceCheckFlushTime")
	newFlushTimeStats("EventFlushTime")
	newFlushTimeStats("MainFlushTime")
	newFlushTimeStats("MetricSketchFlushTime")
	aggregatorExpvar.Set("Flush", expvar.Func(expStatsMap(flushTimeStats)))

	newFlushCountStats("ServiceChecks")
	newFlushCountStats("Series")
	newFlushCountStats("Events")
	newFlushCountStats("Sketches")
	aggregatorExpvar.Set("FlushCount", expvar.Func(expStatsMap(flushCountStats)))
}

// InitAggregator returns the Singleton instance
func InitAggregator(s *serializer.Serializer, hostname string) *BufferedAggregator {
	return InitAggregatorWithFlushInterval(s, hostname, DefaultFlushInterval)
}

// InitAggregatorWithFlushInterval returns the Singleton instance with a configured flush interval
func InitAggregatorWithFlushInterval(s *serializer.Serializer, hostname string, flushInterval time.Duration) *BufferedAggregator {
	aggregatorInit.Do(func() {
		aggregatorInstance = NewBufferedAggregator(s, hostname, flushInterval)
		go aggregatorInstance.run()
	})

	return aggregatorInstance
}

// SetDefaultAggregator allows to force a custom Aggregator as the default one and run it.
// This is useful for testing or benchmarking.
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
	serviceChecks      metrics.ServiceChecks
	events             metrics.Events
	flushInterval      time.Duration
	mu                 sync.Mutex // to protect the checkSamplers field
	serializer         *serializer.Serializer
	hostname           string
	hostnameUpdate     chan string
	hostnameUpdateDone chan struct{}    // signals that the hostname update is finished
	TickerChan         <-chan time.Time // For test/benchmark purposes: it allows the flush to be controlled from the outside
	healthTicker       *time.Ticker
	healthToken        health.ID
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(s *serializer.Serializer, hostname string, flushInterval time.Duration) *BufferedAggregator {
	aggregator := &BufferedAggregator{
		dogstatsdIn:        make(chan *metrics.MetricSample, 100), // TODO make buffer size configurable
		checkMetricIn:      make(chan senderMetricSample, 100),    // TODO make buffer size configurable
		serviceCheckIn:     make(chan metrics.ServiceCheck, 100),  // TODO make buffer size configurable
		eventIn:            make(chan metrics.Event, 100),         // TODO make buffer size configurable
		sampler:            *NewTimeSampler(bucketSize, hostname),
		checkSamplers:      make(map[check.ID]*CheckSampler),
		distSampler:        *NewDistSampler(bucketSize, hostname),
		flushInterval:      flushInterval,
		serializer:         s,
		hostname:           hostname,
		hostnameUpdate:     make(chan string),
		hostnameUpdateDone: make(chan struct{}),
		healthTicker:       time.NewTicker(15 * time.Second),
		healthToken:        health.Register("aggregator"),
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
// empty. This is mainly useful for tests and benchmark
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

	agg.serviceChecks = append(agg.serviceChecks, &sc)
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

	agg.events = append(agg.events, &e)
}

// addSample adds the metric sample to either the sampler or distSampler
func (agg *BufferedAggregator) addSample(metricSample *metrics.MetricSample, timestamp float64) {
	metricSample.Tags = deduplicateTags(metricSample.Tags)
	if _, ok := metrics.DistributionMetricTypes[metricSample.Mtype]; ok {
		agg.distSampler.addSample(metricSample, timestamp)
	} else {
		agg.sampler.addSample(metricSample, timestamp)
	}
}

// GetSeries grabs all the series from the queue and clears the queue
func (agg *BufferedAggregator) GetSeries() metrics.Series {
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
	addFlushCount("Series", int64(len(series)))

	if len(series) == 0 {
		return
	}

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(series), " series to the forwarder")
		err := agg.serializer.SendSeries(series)
		if err != nil {
			log.Warnf("Error flushing series: %v", err)
			aggregatorExpvar.Add("SeriesFlushErrors", 1)
		}
		addFlushTime("ChecksMetricSampleFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("SeriesFlushed", int64(len(series)))
	}()
}

// GetServiceChecks grabs all the service checks from the queue and clears the queue
func (agg *BufferedAggregator) GetServiceChecks() metrics.ServiceChecks {
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

	serviceChecks := agg.GetServiceChecks()
	addFlushCount("ServiceChecks", int64(len(serviceChecks)))

	// Serialize and forward in a separate goroutine
	go func() {
		log.Debug("Flushing ", len(serviceChecks), " service checks to the forwarder")
		err := agg.serializer.SendServiceChecks(serviceChecks)
		if err != nil {
			log.Warnf("Error flushing service checks: %v", err)
			aggregatorExpvar.Add("ServiceCheckFlushErrors", 1)
		}
		addFlushTime("ServiceCheckFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("ServiceCheckFlushed", int64(len(serviceChecks)))
	}()
}

// GetSketches grabs all the sketches from the queue and clears the queue
func (agg *BufferedAggregator) GetSketches() percentile.SketchSeriesList {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	return agg.distSampler.flush(timeNowNano())
}

func (agg *BufferedAggregator) flushSketches() {
	// Serialize and forward in a separate goroutine
	start := time.Now()
	sketchSeries := agg.GetSketches()
	addFlushCount("Sketches", int64(len(sketchSeries)))
	if len(sketchSeries) == 0 {
		return
	}

	go func() {
		log.Debug("Flushing ", len(sketchSeries), " sketches to the forwarder")
		err := agg.serializer.SendSketch(sketchSeries)
		if err != nil {
			log.Warnf("Error flushing sketch: %v", err)
			aggregatorExpvar.Add("SketchesFlushErrors", 1)
		}
		addFlushTime("MetricSketchFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("SketchesFlushed", int64(len(sketchSeries)))
	}()
}

// GetEvents grabs the events from the queue and clears it
func (agg *BufferedAggregator) GetEvents() metrics.Events {
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
	addFlushCount("Events", int64(len(events)))

	go func() {
		log.Debug("Flushing ", len(events), " events to the forwarder")
		err := agg.serializer.SendEvents(events)
		if err != nil {
			log.Warnf("Error flushing events: %v", err)
			aggregatorExpvar.Add("EventsFlushErrors", 1)
		}
		addFlushTime("EventFlushTime", int64(time.Since(start)))
		aggregatorExpvar.Add("EventsFlushed", int64(len(events)))
	}()
}

func (agg *BufferedAggregator) flush() {
	agg.flushSeries()
	agg.flushSketches()
	agg.flushServiceChecks()
	agg.flushEvents()
}

func (agg *BufferedAggregator) run() {
	if agg.TickerChan == nil {
		flushPeriod := agg.flushInterval
		agg.TickerChan = time.NewTicker(flushPeriod).C
	}
	for {
		select {
		case <-agg.healthTicker.C:
			health.Ping(agg.healthToken)
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
