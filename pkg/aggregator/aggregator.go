// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package aggregator

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// DefaultFlushInterval aggregator default flush interval
const DefaultFlushInterval = 15 * time.Second // flush interval
const bucketSize = 10                         // fixed for now
// MetricSamplePoolBatchSize is the batch size of the metric sample pool.
const MetricSamplePoolBatchSize = 32

// Stats stores a statistic from several past flushes allowing computations like median or percentiles
type Stats struct {
	Flushes    [32]int64 // circular buffer of recent flushes stat
	FlushIndex int       // last flush position in circular buffer
	LastFlush  int64     // most recent flush stat, provided for convenience
	Name       string
	m          sync.Mutex
}

var (
	stateOk    = "ok"
	stateError = "error"
)

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

	aggregatorExpvars = expvar.NewMap("aggregator")
	flushTimeStats    = make(map[string]*Stats)
	flushCountStats   = make(map[string]*Stats)

	aggregatorSeriesFlushed                    = expvar.Int{}
	aggregatorSeriesFlushErrors                = expvar.Int{}
	aggregatorServiceCheckFlushErrors          = expvar.Int{}
	aggregatorServiceCheckFlushed              = expvar.Int{}
	aggregatorSketchesFlushErrors              = expvar.Int{}
	aggregatorSketchesFlushed                  = expvar.Int{}
	aggregatorEventsFlushErrors                = expvar.Int{}
	aggregatorEventsFlushed                    = expvar.Int{}
	aggregatorNumberOfFlush                    = expvar.Int{}
	aggregatorDogstatsdMetricSample            = expvar.Int{}
	aggregatorChecksMetricSample               = expvar.Int{}
	aggregatorCheckHistogramBucketMetricSample = expvar.Int{}
	aggregatorServiceCheck                     = expvar.Int{}
	aggregatorEvent                            = expvar.Int{}
	aggregatorHostnameUpdate                   = expvar.Int{}

	tlmFlush = telemetry.NewCounter("aggregator", "flush",
		[]string{"data_type", "state"}, "Number of metrics/service checks/events flushed")
	tlmProcessed = telemetry.NewCounter("aggregator", "processed",
		[]string{"data_type"}, "Amount of metrics/services_checks/events processed by the aggregator")
	tlmHostnameUpdate = telemetry.NewCounter("aggregator", "hostname_update",
		nil, "Count of hostname update")

	// Hold series to be added to aggregated series on each flush
	recurrentSeries     metrics.Series
	recurrentSeriesLock sync.Mutex
)

func init() {
	newFlushTimeStats("ChecksMetricSampleFlushTime")
	newFlushTimeStats("ServiceCheckFlushTime")
	newFlushTimeStats("EventFlushTime")
	newFlushTimeStats("MainFlushTime")
	newFlushTimeStats("MetricSketchFlushTime")
	aggregatorExpvars.Set("Flush", expvar.Func(expStatsMap(flushTimeStats)))

	newFlushCountStats("ServiceChecks")
	newFlushCountStats("Series")
	newFlushCountStats("Events")
	newFlushCountStats("Sketches")
	aggregatorExpvars.Set("FlushCount", expvar.Func(expStatsMap(flushCountStats)))

	aggregatorExpvars.Set("SeriesFlushed", &aggregatorSeriesFlushed)
	aggregatorExpvars.Set("SeriesFlushErrors", &aggregatorSeriesFlushErrors)
	aggregatorExpvars.Set("ServiceCheckFlushErrors", &aggregatorServiceCheckFlushErrors)
	aggregatorExpvars.Set("ServiceCheckFlushed", &aggregatorServiceCheckFlushed)
	aggregatorExpvars.Set("SketchesFlushErrors", &aggregatorSketchesFlushErrors)
	aggregatorExpvars.Set("SketchesFlushed", &aggregatorSketchesFlushed)
	aggregatorExpvars.Set("EventsFlushErrors", &aggregatorEventsFlushErrors)
	aggregatorExpvars.Set("EventsFlushed", &aggregatorEventsFlushed)
	aggregatorExpvars.Set("NumberOfFlush", &aggregatorNumberOfFlush)
	aggregatorExpvars.Set("DogstatsdMetricSample", &aggregatorDogstatsdMetricSample)
	aggregatorExpvars.Set("ChecksMetricSample", &aggregatorChecksMetricSample)
	aggregatorExpvars.Set("ChecksHistogramBucketMetricSample", &aggregatorCheckHistogramBucketMetricSample)
	aggregatorExpvars.Set("ServiceCheck", &aggregatorServiceCheck)
	aggregatorExpvars.Set("Event", &aggregatorEvent)
	aggregatorExpvars.Set("HostnameUpdate", &aggregatorHostnameUpdate)
}

// InitAggregator returns the Singleton instance
func InitAggregator(s serializer.MetricSerializer, hostname string) *BufferedAggregator {
	return InitAggregatorWithFlushInterval(s, hostname, DefaultFlushInterval)
}

// InitAggregatorWithFlushInterval returns the Singleton instance with a configured flush interval
func InitAggregatorWithFlushInterval(s serializer.MetricSerializer, hostname string, flushInterval time.Duration) *BufferedAggregator {
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

// StopDefaultAggregator stops the default aggregator. Based on 'flushData'
// waiting metrics (from checks or closed dogstatsd buckets) will be sent to
// the serializer before stopping.
func StopDefaultAggregator() {
	if aggregatorInstance != nil {
		aggregatorInstance.Stop()
	}
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	bufferedMetricIn       chan []metrics.MetricSample
	bufferedServiceCheckIn chan []*metrics.ServiceCheck
	bufferedEventIn        chan []*metrics.Event

	metricIn       chan *metrics.MetricSample
	eventIn        chan metrics.Event
	serviceCheckIn chan metrics.ServiceCheck

	checkMetricIn          chan senderMetricSample
	checkHistogramBucketIn chan senderHistogramBucket

	// metricSamplePool is a pool of slices of metric sample to avoid allocations.
	// Used by the Dogstatsd Batcher.
	MetricSamplePool *metrics.MetricSamplePool

	statsdSampler      TimeSampler
	checkSamplers      map[check.ID]*CheckSampler
	serviceChecks      metrics.ServiceChecks
	events             metrics.Events
	flushInterval      time.Duration
	mu                 sync.Mutex // to protect the checkSamplers field
	flushMutex         sync.Mutex // to start multiple flushes in parallel
	serializer         serializer.MetricSerializer
	hostname           string
	hostnameUpdate     chan string
	hostnameUpdateDone chan struct{}    // signals that the hostname update is finished
	TickerChan         <-chan time.Time // For test/benchmark purposes: it allows the flush to be controlled from the outside
	stopChan           chan struct{}
	health             *health.Handle
	agentName          string // Name of the agent for telemetry metrics

	tlmContainerTagsEnabled bool                                              // Whether we should call the tagger to tag agent telemetry metrics
	agentTags               func(collectors.TagCardinality) ([]string, error) // This function gets the agent tags from the tagger (defined as a struct field to ease testing)
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(s serializer.MetricSerializer, hostname string, flushInterval time.Duration) *BufferedAggregator {
	bufferSize := config.Datadog.GetInt("aggregator_buffer_size")

	agentName := flavor.GetFlavor()
	if config.Datadog.GetBool("iot_host") {
		// Override the agentName if this Agent is configured to report as IotAgent
		agentName = flavor.IotAgent
	}
	if config.Datadog.GetBool("heroku_dyno") {
		// Override the agentName if this Agent is configured to report as Heroku Dyno
		agentName = flavor.HerokuAgent
	}

	aggregator := &BufferedAggregator{
		bufferedMetricIn:       make(chan []metrics.MetricSample, bufferSize),
		bufferedServiceCheckIn: make(chan []*metrics.ServiceCheck, bufferSize),
		bufferedEventIn:        make(chan []*metrics.Event, bufferSize),

		metricIn:       make(chan *metrics.MetricSample, bufferSize),
		serviceCheckIn: make(chan metrics.ServiceCheck, bufferSize),
		eventIn:        make(chan metrics.Event, bufferSize),

		checkMetricIn:          make(chan senderMetricSample, bufferSize),
		checkHistogramBucketIn: make(chan senderHistogramBucket, bufferSize),

		MetricSamplePool: metrics.NewMetricSamplePool(MetricSamplePoolBatchSize),

		statsdSampler:           *NewTimeSampler(bucketSize),
		checkSamplers:           make(map[check.ID]*CheckSampler),
		flushInterval:           flushInterval,
		serializer:              s,
		hostname:                hostname,
		hostnameUpdate:          make(chan string),
		hostnameUpdateDone:      make(chan struct{}),
		stopChan:                make(chan struct{}),
		health:                  health.RegisterLiveness("aggregator"),
		agentName:               agentName,
		tlmContainerTagsEnabled: config.Datadog.GetBool("basic_telemetry_add_container_tags"),
		agentTags:               tagger.AgentTags,
	}

	return aggregator
}

// AddRecurrentSeries adds a serie to the series that are sent at every flush
func AddRecurrentSeries(newSerie *metrics.Serie) {
	recurrentSeriesLock.Lock()
	defer recurrentSeriesLock.Unlock()
	recurrentSeries = append(recurrentSeries, newSerie)
}

// IsInputQueueEmpty returns true if every input channel for the aggregator are
// empty. This is mainly useful for tests and benchmark
func (agg *BufferedAggregator) IsInputQueueEmpty() bool {
	if len(agg.checkMetricIn)+len(agg.serviceCheckIn)+len(agg.eventIn)+len(agg.checkHistogramBucketIn) == 0 {
		return true
	}
	return false
}

// GetChannels returns a channel which can be subsequently used to send MetricSamples, Event or ServiceCheck
func (agg *BufferedAggregator) GetChannels() (chan *metrics.MetricSample, chan metrics.Event, chan metrics.ServiceCheck) {
	return agg.metricIn, agg.eventIn, agg.serviceCheckIn
}

// GetBufferedChannels returns a channel which can be subsequently used to send MetricSamples, Event or ServiceCheck
func (agg *BufferedAggregator) GetBufferedChannels() (chan []metrics.MetricSample, chan []*metrics.Event, chan []*metrics.ServiceCheck) {
	return agg.bufferedMetricIn, agg.bufferedEventIn, agg.bufferedServiceCheckIn
}

// SetHostname sets the hostname that the aggregator uses by default on all the data it sends
// Blocks until the main aggregator goroutine has finished handling the update
func (agg *BufferedAggregator) SetHostname(hostname string) {
	agg.hostnameUpdate <- hostname
	<-agg.hostnameUpdateDone
}

// AddAgentStartupTelemetry adds a startup event and count to be sent on the next flush
func (agg *BufferedAggregator) AddAgentStartupTelemetry(agentVersion string) {

	metric := &metrics.MetricSample{
		Name:       fmt.Sprintf("datadog.%s.started", agg.agentName),
		Value:      1,
		Tags:       agg.tags(true),
		Host:       agg.hostname,
		Mtype:      metrics.CountType,
		SampleRate: 1,
		Timestamp:  0,
	}
	agg.metricIn <- metric
	if agg.hostname != "" {
		// Send startup event only when we have a valid hostname
		agg.eventIn <- metrics.Event{
			Text:           fmt.Sprintf("Version %s", agentVersion),
			SourceTypeName: "System",
			Host:           agg.hostname,
			EventType:      "Agent Startup",
		}
	}
}

func (agg *BufferedAggregator) registerSender(id check.ID) error {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	if _, ok := agg.checkSamplers[id]; ok {
		return fmt.Errorf("Sender with ID '%s' has already been registered, will use existing sampler", id)
	}
	agg.checkSamplers[id] = newCheckSampler()
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
			ss.metricSample.Tags = util.SortUniqInPlace(ss.metricSample.Tags)
			checkSampler.addSample(ss.metricSample)
		}
	} else {
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle senderMetricSample", ss.id)
	}
}

func (agg *BufferedAggregator) handleSenderBucket(checkBucket senderHistogramBucket) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	if checkSampler, ok := agg.checkSamplers[checkBucket.id]; ok {
		checkBucket.bucket.Tags = util.SortUniqInPlace(checkBucket.bucket.Tags)
		checkSampler.addBucket(checkBucket.bucket)
	} else {
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle histogram bucket", checkBucket.id)
	}
}

// addServiceCheck adds the service check to the slice of current service checks
func (agg *BufferedAggregator) addServiceCheck(sc metrics.ServiceCheck) {
	if sc.Ts == 0 {
		sc.Ts = time.Now().Unix()
	}
	sc.Tags = metrics.EnrichTags(sc.Tags, sc.OriginID, sc.K8sOriginID)

	agg.serviceChecks = append(agg.serviceChecks, &sc)
}

// addEvent adds the event to the slice of current events
func (agg *BufferedAggregator) addEvent(e metrics.Event) {
	if e.Ts == 0 {
		e.Ts = time.Now().Unix()
	}
	e.Tags = metrics.EnrichTags(e.Tags, e.OriginID, e.K8sOriginID)

	agg.events = append(agg.events, &e)
}

// addSample adds the metric sample
func (agg *BufferedAggregator) addSample(metricSample *metrics.MetricSample, timestamp float64) {
	agg.statsdSampler.addSample(metricSample, timestamp)
}

// GetSeriesAndSketches grabs all the series & sketches from the queue and clears the queue
// The parameter `before` is used as an end interval while retrieving series and sketches
// from the time sampler. Metrics and sketches before this timestamp should be returned.
func (agg *BufferedAggregator) GetSeriesAndSketches(before time.Time) (metrics.Series, metrics.SketchSeriesList) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	series, sketches := agg.statsdSampler.flush(float64(before.UnixNano()) / float64(time.Second))
	for _, checkSampler := range agg.checkSamplers {
		s, sk := checkSampler.flush()
		series = append(series, s...)
		sketches = append(sketches, sk...)
	}
	return series, sketches
}

func (agg *BufferedAggregator) pushSketches(start time.Time, sketches metrics.SketchSeriesList) {
	log.Debugf("Flushing %d sketches to the forwarder", len(sketches))
	err := agg.serializer.SendSketch(sketches)
	state := stateOk
	if err != nil {
		log.Warnf("Error flushing sketch: %v", err)
		aggregatorSketchesFlushErrors.Add(1)
		state = stateError
	}
	addFlushTime("MetricSketchFlushTime", int64(time.Since(start)))
	aggregatorSketchesFlushed.Add(int64(len(sketches)))
	tlmFlush.Add(float64(len(sketches)), "sketches", state)
}

func (agg *BufferedAggregator) pushSeries(start time.Time, series metrics.Series) {
	log.Debugf("Flushing %d series to the forwarder", len(series))
	err := agg.serializer.SendSeries(series)
	state := stateOk
	if err != nil {
		log.Warnf("Error flushing series: %v", err)
		aggregatorSeriesFlushErrors.Add(1)
		state = stateError
	}
	addFlushTime("ChecksMetricSampleFlushTime", int64(time.Since(start)))
	aggregatorSeriesFlushed.Add(int64(len(series)))
	tlmFlush.Add(float64(len(series)), "series", state)
}

func (agg *BufferedAggregator) sendSeries(start time.Time, series metrics.Series, waitForSerializer bool) {
	recurrentSeriesLock.Lock()
	// Adding recurrentSeries to the flushed ones
	for _, extra := range recurrentSeries {
		if extra.Host == "" {
			extra.Host = agg.hostname
		}
		if extra.SourceTypeName == "" {
			extra.SourceTypeName = "System"
		}

		tags := append(extra.Tags, agg.tags(false)...)
		newSerie := &metrics.Serie{
			Name:           extra.Name,
			Tags:           tags,
			Host:           extra.Host,
			MType:          extra.MType,
			SourceTypeName: extra.SourceTypeName,
		}

		// Updating Ts for every points
		updatedPoints := []metrics.Point{}
		for _, point := range extra.Points {
			updatedPoints = append(updatedPoints,
				metrics.Point{
					Value: point.Value,
					Ts:    float64(start.Unix()),
				})
		}
		newSerie.Points = updatedPoints
		series = append(series, newSerie)
	}
	recurrentSeriesLock.Unlock()

	// Send along a metric that showcases that this Agent is running (internally, in backend,
	// a `datadog.`-prefixed metric allows identifying this host as an Agent host, used for dogbone icon)
	series = append(series, &metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", agg.agentName),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           agg.tags(true),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})

	// Send along a metric that counts the number of times we dropped some payloads because we couldn't split them.
	series = append(series, &metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", agg.agentName),
		Points:         []metrics.Point{{Value: float64(split.GetPayloadDrops()), Ts: float64(start.Unix())}},
		Tags:           agg.tags(false),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})

	addFlushCount("Series", int64(len(series)))

	// For debug purposes print out all metrics/tag combinations
	if config.Datadog.GetBool("log_payloads") {
		log.Debug("Flushing the following metrics:")
		for _, serie := range series {
			log.Debugf("%s", serie)
		}
	}

	if waitForSerializer {
		agg.pushSeries(start, series)
	} else {
		go agg.pushSeries(start, series)
	}
}

func (agg *BufferedAggregator) sendSketches(start time.Time, sketches metrics.SketchSeriesList, waitForSerializer bool) {
	// Serialize and forward sketches in a separate goroutine
	addFlushCount("Sketches", int64(len(sketches)))
	if len(sketches) != 0 {
		if waitForSerializer {
			agg.pushSketches(start, sketches)
		} else {
			go agg.pushSketches(start, sketches)
		}
	}
}

func (agg *BufferedAggregator) flushSeriesAndSketches(start time.Time, waitForSerializer bool) {
	series, sketches := agg.GetSeriesAndSketches(start)

	agg.sendSketches(start, sketches, waitForSerializer)
	agg.sendSeries(start, series, waitForSerializer)
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

func (agg *BufferedAggregator) sendServiceChecks(start time.Time, serviceChecks metrics.ServiceChecks) {
	log.Debugf("Flushing %d service checks to the forwarder", len(serviceChecks))
	state := stateOk
	if err := agg.serializer.SendServiceChecks(serviceChecks); err != nil {
		log.Warnf("Error flushing service checks: %v", err)
		aggregatorServiceCheckFlushErrors.Add(1)
		state = stateError
	}
	addFlushTime("ServiceCheckFlushTime", int64(time.Since(start)))
	aggregatorServiceCheckFlushed.Add(int64(len(serviceChecks)))
	tlmFlush.Add(float64(len(serviceChecks)), "service_checks", state)
}

func (agg *BufferedAggregator) flushServiceChecks(start time.Time, waitForSerializer bool) {
	// Add a simple service check for the Agent status
	agg.addServiceCheck(metrics.ServiceCheck{
		CheckName: "datadog.agent.up",
		Status:    metrics.ServiceCheckOK,
		Tags:      agg.tags(false),
		Host:      agg.hostname,
	})

	serviceChecks := agg.GetServiceChecks()
	addFlushCount("ServiceChecks", int64(len(serviceChecks)))

	// For debug purposes print out all serviceCheck/tag combinations
	if config.Datadog.GetBool("log_payloads") {
		log.Debug("Flushing the following Service Checks:")
		for _, sc := range serviceChecks {
			log.Debugf("%s", sc)
		}
	}

	if waitForSerializer {
		agg.sendServiceChecks(start, serviceChecks)
	} else {
		go agg.sendServiceChecks(start, serviceChecks)
	}
}

// GetEvents grabs the events from the queue and clears it
func (agg *BufferedAggregator) GetEvents() metrics.Events {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	events := agg.events
	agg.events = nil
	return events
}

func (agg *BufferedAggregator) sendEvents(start time.Time, events metrics.Events) {
	log.Debugf("Flushing %d events to the forwarder", len(events))
	err := agg.serializer.SendEvents(events)
	state := stateOk
	if err != nil {
		log.Warnf("Error flushing events: %v", err)
		aggregatorEventsFlushErrors.Add(1)
		state = stateError
	}
	addFlushTime("EventFlushTime", int64(time.Since(start)))
	aggregatorEventsFlushed.Add(int64(len(events)))
	tlmFlush.Add(float64(len(events)), "events", state)
}

// flushEvents serializes and forwards events in a separate goroutine
func (agg *BufferedAggregator) flushEvents(start time.Time, waitForSerializer bool) {
	// Serialize and forward in a separate goroutine
	events := agg.GetEvents()
	if len(events) == 0 {
		return
	}
	addFlushCount("Events", int64(len(events)))

	// For debug purposes print out all Event/tag combinations
	if config.Datadog.GetBool("log_payloads") {
		log.Debug("Flushing the following Events:")
		for _, event := range events {
			log.Debugf("%s", event)
		}
	}

	if waitForSerializer {
		agg.sendEvents(start, events)
	} else {
		go agg.sendEvents(start, events)
	}
}

// Flush flushes the data contained in the BufferedAggregator into the Forwarder.
// This method can be called from multiple routines.
func (agg *BufferedAggregator) Flush(start time.Time, waitForSerializer bool) {
	agg.flushMutex.Lock()
	defer agg.flushMutex.Unlock()
	agg.flushSeriesAndSketches(start, waitForSerializer)
	agg.flushServiceChecks(start, waitForSerializer)
	agg.flushEvents(start, waitForSerializer)
}

// Stop stops the aggregator. Based on 'flushData' waiting metrics (from checks
// or closed dogstatsd buckets) will be sent to the serializer before stopping.
func (agg *BufferedAggregator) Stop() {
	agg.stopChan <- struct{}{}

	timeout := config.Datadog.GetDuration("aggregator_stop_timeout") * time.Second
	if timeout > 0 {
		done := make(chan struct{})
		go func() {
			agg.Flush(time.Now(), true)
			done <- struct{}{}
		}()

		select {
		case <-done:
		case <-time.After(timeout):
			log.Errorf("flushing data after stop timed out")
		}
	}

}

func (agg *BufferedAggregator) run() {
	if agg.TickerChan == nil {
		if agg.flushInterval != 0 {
			agg.TickerChan = time.NewTicker(agg.flushInterval).C
		} else {
			log.Debugf("aggregator flushInterval set to 0: aggregator won't flush data")
		}
	}

	for {
		select {
		case <-agg.stopChan:
			log.Info("Stopping aggregator")
			return
		case <-agg.health.C:
		case <-agg.TickerChan:
			start := time.Now()
			agg.Flush(start, false)
			addFlushTime("MainFlushTime", int64(time.Since(start)))
			aggregatorNumberOfFlush.Add(1)
		case checkMetric := <-agg.checkMetricIn:
			aggregatorChecksMetricSample.Add(1)
			tlmProcessed.Inc("metrics")
			agg.handleSenderSample(checkMetric)
		case checkHistogramBucket := <-agg.checkHistogramBucketIn:
			aggregatorCheckHistogramBucketMetricSample.Add(1)
			tlmProcessed.Inc("histogram_bucket")
			agg.handleSenderBucket(checkHistogramBucket)
		case metric := <-agg.metricIn:
			aggregatorDogstatsdMetricSample.Add(1)
			tlmProcessed.Inc("dogstatsd_metrics")
			agg.addSample(metric, timeNowNano())
		case event := <-agg.eventIn:
			aggregatorEvent.Add(1)
			tlmProcessed.Inc("events")
			agg.addEvent(event)
		case serviceCheck := <-agg.serviceCheckIn:
			aggregatorServiceCheck.Add(1)
			tlmProcessed.Inc("service_checks")
			agg.addServiceCheck(serviceCheck)
		case ms := <-agg.bufferedMetricIn:
			aggregatorDogstatsdMetricSample.Add(int64(len(ms)))
			tlmProcessed.Add(float64(len(ms)), "dogstatsd_metrics")
			for i := 0; i < len(ms); i++ {
				agg.addSample(&ms[i], timeNowNano())
			}
			agg.MetricSamplePool.PutBatch(ms)
		case serviceChecks := <-agg.bufferedServiceCheckIn:
			aggregatorServiceCheck.Add(int64(len(serviceChecks)))
			tlmProcessed.Add(float64(len(serviceChecks)), "service_checks")
			for _, serviceCheck := range serviceChecks {
				agg.addServiceCheck(*serviceCheck)
			}
		case events := <-agg.bufferedEventIn:
			aggregatorEvent.Add(int64(len(events)))
			tlmProcessed.Add(float64(len(events)), "events")
			for _, event := range events {
				agg.addEvent(*event)
			}
		case h := <-agg.hostnameUpdate:
			aggregatorHostnameUpdate.Add(1)
			tlmHostnameUpdate.Inc()
			agg.hostname = h
			changeAllSendersDefaultHostname(h)
			agg.hostnameUpdateDone <- struct{}{}
		}
	}
}

// tags returns the list of tags that should be added to the agent telemetry metrics
// Container agent tags may be missing in the first seconds after agent startup
func (agg *BufferedAggregator) tags(withVersion bool) []string {
	tags := []string{}
	if agg.tlmContainerTagsEnabled {
		var err error
		tags, err = agg.agentTags(tagger.ChecksCardinality)
		if err != nil {
			log.Debugf("Couldn't get Agent tags: %v", err)
		}
	}
	if withVersion {
		return append(tags, "version:"+version.AgentVersion)
	}
	return tags
}
