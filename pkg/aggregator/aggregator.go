// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package aggregator

import (
	"errors"
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/sort"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// DefaultFlushInterval aggregator default flush interval
	DefaultFlushInterval = 15 * time.Second // flush interval
	bucketSize           = 10               // fixed for now
	// MetricSamplePoolBatchSize is the batch size of the metric sample pool.
	MetricSamplePoolBatchSize = 32
)

// tagsetTlm handles telemetry for large tagsets.
var tagsetTlm *tagsetTelemetry

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

func (s *Stats) copy() *Stats {
	s.m.Lock()
	defer s.m.Unlock()

	return &Stats{
		Flushes:    s.Flushes,
		FlushIndex: s.FlushIndex,
		LastFlush:  s.LastFlush,
		Name:       s.Name,
	}
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
		res := make(map[string]*Stats, len(statsMap))
		for k, v := range statsMap {
			res[k] = v.copy()
		}
		return res
	}
}

func expMetricTags() interface{} {
	return tagsetTlm.exp()
}

func timeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second) // Unix time with nanosecond precision
}

var (
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
	aggregatorOrchestratorMetadata             = expvar.Int{}
	aggregatorOrchestratorMetadataErrors       = expvar.Int{}
	aggregatorOrchestratorManifests            = expvar.Int{}
	aggregatorOrchestratorManifestsErrors      = expvar.Int{}
	aggregatorDogstatsdContexts                = expvar.Int{}
	aggregatorDogstatsdContextsByMtype         = []expvar.Int{}
	aggregatorEventPlatformEvents              = expvar.Map{}
	aggregatorEventPlatformEventsErrors        = expvar.Map{}

	tlmFlush = telemetry.NewCounter("aggregator", "flush",
		[]string{"data_type", "state"}, "Number of metrics/service checks/events flushed")

	tlmChannelSize = telemetry.NewGauge("aggregator", "channel_size",
		[]string{"shard"}, "Size of the aggregator channel")
	tlmProcessed = telemetry.NewCounter("aggregator", "processed",
		[]string{"shard", "data_type"}, "Amount of metrics/services_checks/events processed by the aggregator")
	tlmDogstatsdTimeBuckets = telemetry.NewGauge("aggregator", "dogstatsd_time_buckets",
		[]string{"shard"}, "Number of time buckets in the dogstatsd sampler")
	tlmDogstatsdContexts = telemetry.NewGauge("aggregator", "dogstatsd_contexts",
		[]string{"shard"}, "Count the number of dogstatsd contexts in the aggregator")
	tlmDogstatsdContextsByMtype = telemetry.NewGauge("aggregator", "dogstatsd_contexts_by_mtype",
		[]string{"shard", "metric_type"}, "Count the number of dogstatsd contexts in the aggregator, by metric type")
	tlmDogstatsdContextsBytesByMtype = telemetry.NewGauge("aggregator", "dogstatsd_contexts_bytes_by_mtype",
		[]string{"shard", "metric_type", util.BytesKindTelemetryKey}, "Estimated count of bytes taken by contexts in the aggregator, by metric type")
	tlmChecksContexts = telemetry.NewGauge("aggregator", "checks_contexts",
		[]string{"shard"}, "Count the number of checks contexts in the check aggregator")
	tlmChecksContextsByMtype = telemetry.NewGauge("aggregator", "checks_contexts_by_mtype",
		[]string{"shard", "metric_type"}, "Count the number of checks contexts in the check aggregator, by metric type")
	tlmChecksContextsBytesByMtype = telemetry.NewGauge("aggregator", "checks_contexts_bytes_by_mtype",
		[]string{"shard", "metric_type", util.BytesKindTelemetryKey}, "Estimated count of bytes taken by contexts in the check aggregator, by metric type")

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
	newFlushTimeStats("ManifestsTime")
	aggregatorExpvars.Set("Flush", expvar.Func(expStatsMap(flushTimeStats)))

	newFlushCountStats("ServiceChecks")
	newFlushCountStats("Series")
	newFlushCountStats("Events")
	newFlushCountStats("Sketches")
	newFlushCountStats("Manifests")
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
	aggregatorExpvars.Set("OrchestratorMetadata", &aggregatorOrchestratorMetadata)
	aggregatorExpvars.Set("OrchestratorMetadataErrors", &aggregatorOrchestratorMetadataErrors)
	aggregatorExpvars.Set("OrchestratorManifests", &aggregatorOrchestratorManifests)
	aggregatorExpvars.Set("OrchestratorManifestsErrors", &aggregatorOrchestratorManifestsErrors)
	aggregatorExpvars.Set("DogstatsdContexts", &aggregatorDogstatsdContexts)
	aggregatorExpvars.Set("EventPlatformEvents", &aggregatorEventPlatformEvents)
	aggregatorExpvars.Set("EventPlatformEventsErrors", &aggregatorEventPlatformEventsErrors)

	contextsByMtypeMap := expvar.Map{}
	aggregatorDogstatsdContextsByMtype = make([]expvar.Int, int(metrics.NumMetricTypes))
	for i := 0; i < int(metrics.NumMetricTypes); i++ {
		mtype := metrics.MetricType(i).String()
		aggregatorDogstatsdContextsByMtype[i] = expvar.Int{}
		contextsByMtypeMap.Set(mtype, &aggregatorDogstatsdContextsByMtype[i])
	}
	aggregatorExpvars.Set("DogstatsdContextsByMtype", &contextsByMtypeMap)

	tagsetTlm = newTagsetTelemetry([]uint64{90, 100})

	aggregatorExpvars.Set("MetricTags", expvar.Func(expMetricTags))
}

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	bufferedServiceCheckIn chan []*servicecheck.ServiceCheck
	bufferedEventIn        chan []*event.Event

	eventIn        chan event.Event
	serviceCheckIn chan servicecheck.ServiceCheck

	checkItems             chan senderItem
	orchestratorMetadataIn chan senderOrchestratorMetadata
	orchestratorManifestIn chan senderOrchestratorManifest
	eventPlatformIn        chan senderEventPlatformEvent

	// metricSamplePool is a pool of slices of metric sample to avoid allocations.
	// Used by the Dogstatsd Batcher.
	MetricSamplePool *metrics.MetricSamplePool

	tagsStore              *tags.Store
	checkSamplers          map[checkid.ID]*CheckSampler
	serviceChecks          servicecheck.ServiceChecks
	events                 event.Events
	manifests              []*senderOrchestratorManifest
	flushInterval          time.Duration
	mu                     sync.Mutex // to protect the checkSamplers field
	flushMutex             sync.Mutex // to start multiple flushes in parallel
	serializer             serializer.MetricSerializer
	eventPlatformForwarder eventplatform.Component
	hostname               string
	hostnameUpdate         chan string
	hostnameUpdateDone     chan struct{} // signals that the hostname update is finished
	flushChan              chan flushTrigger

	stopChan  chan struct{}
	health    *health.Handle
	agentName string // Name of the agent for telemetry metrics

	tlmContainerTagsEnabled     bool                                         // Whether we should call the tagger to tag agent telemetry metrics
	agentTags                   func(types.TagCardinality) ([]string, error) // This function gets the agent tags from the tagger (defined as a struct field to ease testing)
	globalTags                  func(types.TagCardinality) ([]string, error) // This function gets global tags from the tagger when host tags are not available
	tagger                      tagger.Component
	flushAndSerializeInParallel FlushAndSerializeInParallel
}

// FlushAndSerializeInParallel contains options for flushing metrics and serializing in parallel.
type FlushAndSerializeInParallel struct {
	ChannelSize int
	BufferSize  int
}

// NewFlushAndSerializeInParallel creates a new instance of FlushAndSerializeInParallel.
func NewFlushAndSerializeInParallel(config model.Config) FlushAndSerializeInParallel {
	return FlushAndSerializeInParallel{
		BufferSize:  config.GetInt("aggregator_flush_metrics_and_serialize_in_parallel_buffer_size"),
		ChannelSize: config.GetInt("aggregator_flush_metrics_and_serialize_in_parallel_chan_size"),
	}
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(s serializer.MetricSerializer, eventPlatformForwarder eventplatform.Component, tagger tagger.Component, hostname string, flushInterval time.Duration) *BufferedAggregator {
	bufferSize := pkgconfigsetup.Datadog().GetInt("aggregator_buffer_size")

	agentName := flavor.GetFlavor()
	if agentName == flavor.IotAgent && !pkgconfigsetup.Datadog().GetBool("iot_host") {
		agentName = flavor.DefaultAgent
	} else if pkgconfigsetup.Datadog().GetBool("iot_host") {
		// Override the agentName if this Agent is configured to report as IotAgent
		agentName = flavor.IotAgent
	}
	if pkgconfigsetup.Datadog().GetBool("heroku_dyno") {
		// Override the agentName if this Agent is configured to report as Heroku Dyno
		agentName = flavor.HerokuAgent
	}

	if pkgconfigsetup.Datadog().GetBool("djm_config.enabled") {
		AddRecurrentSeries(&metrics.Serie{
			Name:   "datadog.djm.agent_host",
			Points: []metrics.Point{{Value: 1.0}},
			MType:  metrics.APIGaugeType,
		})
	}

	tagsStore := tags.NewStore(pkgconfigsetup.Datadog().GetBool("aggregator_use_tags_store"), "aggregator")

	aggregator := &BufferedAggregator{
		bufferedServiceCheckIn: make(chan []*servicecheck.ServiceCheck, bufferSize),
		bufferedEventIn:        make(chan []*event.Event, bufferSize),

		serviceCheckIn: make(chan servicecheck.ServiceCheck, bufferSize),
		eventIn:        make(chan event.Event, bufferSize),

		checkItems: make(chan senderItem, bufferSize),

		orchestratorMetadataIn: make(chan senderOrchestratorMetadata, bufferSize),
		orchestratorManifestIn: make(chan senderOrchestratorManifest, bufferSize),
		eventPlatformIn:        make(chan senderEventPlatformEvent, bufferSize),

		tagsStore:                   tagsStore,
		checkSamplers:               make(map[checkid.ID]*CheckSampler),
		flushInterval:               flushInterval,
		serializer:                  s,
		eventPlatformForwarder:      eventPlatformForwarder,
		hostname:                    hostname,
		hostnameUpdate:              make(chan string),
		hostnameUpdateDone:          make(chan struct{}),
		flushChan:                   make(chan flushTrigger),
		stopChan:                    make(chan struct{}),
		health:                      health.RegisterLiveness("aggregator"),
		agentName:                   agentName,
		tlmContainerTagsEnabled:     pkgconfigsetup.Datadog().GetBool("basic_telemetry_add_container_tags"),
		agentTags:                   tagger.AgentTags,
		globalTags:                  tagger.GlobalTags,
		tagger:                      tagger,
		flushAndSerializeInParallel: NewFlushAndSerializeInParallel(pkgconfigsetup.Datadog()),
	}

	return aggregator
}

func (agg *BufferedAggregator) addOrchestratorManifest(manifests *senderOrchestratorManifest) {
	agg.manifests = append(agg.manifests, manifests)
}

// getOrchestratorManifests grabs the manifests from the queue and clears it
func (agg *BufferedAggregator) getOrchestratorManifests() []*senderOrchestratorManifest {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	manifests := agg.manifests
	agg.manifests = nil
	return manifests
}

func (agg *BufferedAggregator) sendOrchestratorManifests(start time.Time, senderManifests []*senderOrchestratorManifest) {
	for _, senderManifest := range senderManifests {
		err := agg.serializer.SendOrchestratorManifests(
			senderManifest.msgs,
			agg.hostname,
			senderManifest.clusterID,
		)
		if err != nil {
			log.Warnf("Error flushing events: %v", err)
			aggregatorOrchestratorMetadataErrors.Add(1)
		}
		aggregatorOrchestratorManifests.Add(1)
		addFlushTime("ManifestsTime", int64(time.Since(start)))

	}
}

// flushOrchestratorManifests serializes and forwards events in a separate goroutine
func (agg *BufferedAggregator) flushOrchestratorManifests(start time.Time, waitForSerializer bool) {
	manifests := agg.getOrchestratorManifests()
	if len(manifests) == 0 {
		return
	}
	addFlushCount("Manifests", int64(len(manifests)))

	if waitForSerializer {
		agg.sendOrchestratorManifests(start, manifests)
	} else {
		go agg.sendOrchestratorManifests(start, manifests)
	}
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
	return len(agg.checkItems)+len(agg.serviceCheckIn)+len(agg.eventIn)+len(agg.eventPlatformIn) == 0
}

// GetBufferedChannels returns a channel which can be subsequently used to send Event or ServiceCheck.
func (agg *BufferedAggregator) GetBufferedChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck) {
	return agg.bufferedEventIn, agg.bufferedServiceCheckIn
}

// GetEventPlatformForwarder returns a event platform forwarder
func (agg *BufferedAggregator) GetEventPlatformForwarder() (eventplatform.Forwarder, error) {
	forwarder, found := agg.eventPlatformForwarder.Get()
	if !found {
		return nil, errors.New("event platform forwarder not initialized")
	}
	return forwarder, nil
}

func (agg *BufferedAggregator) registerSender(id checkid.ID) error {
	agg.checkItems <- &registerSampler{id}
	return nil
}

func (agg *BufferedAggregator) deregisterSender(id checkid.ID) {
	// Use the same channel as metrics, so that the drop happens only
	// after we handle all the metrics sent so far.
	agg.checkItems <- &deregisterSampler{id}
}

func (agg *BufferedAggregator) handleSenderSample(ss senderMetricSample) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	aggregatorChecksMetricSample.Add(1)
	tlmProcessed.Inc("", "metrics")

	if checkSampler, ok := agg.checkSamplers[ss.id]; ok {
		if ss.commit {
			checkSampler.commit(timeNowNano())
		} else {
			ss.metricSample.Tags = sort.UniqInPlace(ss.metricSample.Tags)
			checkSampler.addSample(ss.metricSample)
		}
	} else {
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle senderMetricSample", ss.id)
	}
}

func (agg *BufferedAggregator) handleSenderBucket(checkBucket senderHistogramBucket) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	aggregatorCheckHistogramBucketMetricSample.Add(1)
	tlmProcessed.Inc("", "histogram_bucket")

	if checkSampler, ok := agg.checkSamplers[checkBucket.id]; ok {
		checkBucket.bucket.Tags = sort.UniqInPlace(checkBucket.bucket.Tags)
		checkSampler.addBucket(checkBucket.bucket)
	} else {
		log.Debugf("CheckSampler with ID '%s' doesn't exist, can't handle histogram bucket", checkBucket.id)
	}
}

func (agg *BufferedAggregator) handleEventPlatformEvent(event senderEventPlatformEvent) error {
	forwarder, found := agg.eventPlatformForwarder.Get()
	if !found {
		return errors.New("event platform forwarder not initialized")
	}
	m := message.NewMessage(event.rawEvent, nil, "", 0)
	// eventPlatformForwarder is threadsafe so no locking needed here
	return forwarder.SendEventPlatformEvent(m, event.eventType)
}

// addServiceCheck adds the service check to the slice of current service checks
func (agg *BufferedAggregator) addServiceCheck(sc servicecheck.ServiceCheck) {
	if sc.Ts == 0 {
		sc.Ts = time.Now().Unix()
	}
	tb := tagset.NewHashlessTagsAccumulatorFromSlice(sc.Tags)
	agg.tagger.EnrichTags(tb, sc.OriginInfo)

	tb.SortUniq()
	sc.Tags = tb.Get()

	agg.serviceChecks = append(agg.serviceChecks, &sc)
}

// addEvent adds the event to the slice of current events
func (agg *BufferedAggregator) addEvent(e event.Event) {
	if e.Ts == 0 {
		e.Ts = time.Now().Unix()
	}
	tb := tagset.NewHashlessTagsAccumulatorFromSlice(e.Tags)
	agg.tagger.EnrichTags(tb, e.OriginInfo)

	tb.SortUniq()
	e.Tags = tb.Get()

	agg.events = append(agg.events, &e)
}

// GetSeriesAndSketches grabs all the series & sketches from the queue and clears the queue
// The parameter `before` is used as an end interval while retrieving series and sketches
// from the time sampler. Metrics and sketches before this timestamp should be returned.
func (agg *BufferedAggregator) GetSeriesAndSketches(before time.Time) (metrics.Series, metrics.SketchSeriesList) {
	var series metrics.Series
	var sketches metrics.SketchSeriesList
	agg.getSeriesAndSketches(before, &series, &sketches)
	return series, sketches
}

// getSeriesAndSketches grabs all the series & sketches from the queue and clears the queue
// The parameter `before` is used as an end interval while retrieving series and sketches
// from the time sampler. Metrics and sketches before this timestamp should be returned.
func (agg *BufferedAggregator) getSeriesAndSketches(
	_ time.Time,
	seriesSink metrics.SerieSink,
	sketchesSink metrics.SketchesSink,
) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	//nolint:revive // TODO(AML) Fix revive linter
	for checkId, checkSampler := range agg.checkSamplers {
		checkSeries, sketches := checkSampler.flush()
		for _, s := range checkSeries {
			seriesSink.Append(s)
		}

		for _, sk := range sketches {
			sketchesSink.Append(sk)
		}

		if checkSampler.deregistered {
			checkSampler.release()
			delete(agg.checkSamplers, checkId)
		}
	}
}

func updateSerieTelemetry(start time.Time, serieCount uint64, err error) {
	state := stateOk
	if err != nil {
		log.Warnf("Error flushing series: %v", err)
		aggregatorSeriesFlushErrors.Add(1)
		state = stateError
	}
	// NOTE(remy): that's historical but this one actually contains both metric series + dsd series
	addFlushTime("ChecksMetricSampleFlushTime", int64(time.Since(start)))
	aggregatorSeriesFlushed.Add(int64(serieCount))
	tlmFlush.Add(float64(serieCount), "series", state)
}

func updateSketchTelemetry(start time.Time, sketchesCount uint64, err error) {
	state := stateOk
	if err != nil {
		log.Warnf("Error flushing sketch: %v", err)
		aggregatorSketchesFlushErrors.Add(1)
		state = stateError
	}
	addFlushTime("MetricSketchFlushTime", int64(time.Since(start)))
	aggregatorSketchesFlushed.Add(int64(sketchesCount))
	tlmFlush.Add(float64(sketchesCount), "sketches", state)
}

func (agg *BufferedAggregator) appendDefaultSeries(start time.Time, series metrics.SerieSink) {
	recurrentSeriesLock.Lock()
	// Adding recurrentSeries to the flushed ones
	for _, extra := range recurrentSeries {
		if extra.Host == "" {
			extra.Host = agg.hostname
		}
		if extra.SourceTypeName == "" {
			extra.SourceTypeName = "System"
		}

		tags := tagset.CombineCompositeTagsAndSlice(extra.Tags, agg.tags(false))
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
		series.Append(newSerie)
	}
	recurrentSeriesLock.Unlock()

	// Send along a metric that showcases that this Agent is running (internally, in backend,
	// a `datadog.`-prefixed metric allows identifying this host as an Agent host, used for dogbone icon)
	series.Append(&metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", agg.agentName),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           tagset.CompositeTagsFromSlice(agg.tags(true)),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	})

	// Send along a metric that counts the number of times we dropped some payloads because we couldn't split them.
	series.Append(&metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", agg.agentName),
		Points:         []metrics.Point{{Value: float64(split.GetPayloadDrops()), Ts: float64(start.Unix())}},
		Tags:           tagset.CompositeTagsFromSlice(agg.tags(false)),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
		NoIndex:        true,
	})
}

func (agg *BufferedAggregator) flushSeriesAndSketches(trigger flushTrigger) {
	agg.getSeriesAndSketches(trigger.time, trigger.seriesSink, trigger.sketchesSink)
	agg.appendDefaultSeries(trigger.time, trigger.seriesSink)
}

// GetServiceChecks grabs all the service checks from the queue and clears the queue
func (agg *BufferedAggregator) GetServiceChecks() servicecheck.ServiceChecks {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	// Clear the current service check slice
	serviceChecks := agg.serviceChecks
	agg.serviceChecks = nil
	return serviceChecks
}

func (agg *BufferedAggregator) sendServiceChecks(start time.Time, serviceChecks servicecheck.ServiceChecks) {
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
	agg.addServiceCheck(servicecheck.ServiceCheck{
		CheckName: fmt.Sprintf("datadog.%s.up", agg.agentName),
		Status:    servicecheck.ServiceCheckOK,
		Tags:      agg.tags(false),
		Host:      agg.hostname,
	})

	serviceChecks := agg.GetServiceChecks()
	addFlushCount("ServiceChecks", int64(len(serviceChecks)))

	// For debug purposes print out all serviceCheck/tag combinations
	if pkgconfigsetup.Datadog().GetBool("log_payloads") {
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
func (agg *BufferedAggregator) GetEvents() event.Events {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	events := agg.events
	agg.events = nil
	return events
}

// GetEventPlatformEvents grabs the event platform events from the queue and clears them.
// Note that this works only if using the 'noop' event platform forwarder
func (agg *BufferedAggregator) GetEventPlatformEvents() map[string][]*message.Message {
	forwarder, found := agg.eventPlatformForwarder.Get()
	if !found {
		return nil
	}
	return forwarder.Purge()
}

func (agg *BufferedAggregator) sendEvents(start time.Time, events event.Events) {
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
	if pkgconfigsetup.Datadog().GetBool("log_payloads") {
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
func (agg *BufferedAggregator) Flush(trigger flushTrigger) {
	agg.flushMutex.Lock()
	defer agg.flushMutex.Unlock()
	agg.flushSeriesAndSketches(trigger)
	// notify the triggerer that we're done flushing the series and sketches
	if trigger.blockChan != nil {
		trigger.blockChan <- struct{}{}
	}
	agg.flushServiceChecks(trigger.time, trigger.waitForSerializer)
	agg.flushEvents(trigger.time, trigger.waitForSerializer)
	agg.flushOrchestratorManifests(trigger.time, trigger.waitForSerializer)
	agg.updateChecksTelemetry()
}

// Stop stops the aggregator.
func (agg *BufferedAggregator) Stop() {
	agg.stopChan <- struct{}{}
}

func (agg *BufferedAggregator) run() {
	// ensures event platform errors are logged at most once per flush
	aggregatorEventPlatformErrorLogged := false

	for {
		select {
		case <-agg.stopChan:
			log.Info("Stopping aggregator")
			return
		case trigger := <-agg.flushChan:
			agg.Flush(trigger)

			// Do this here, rather than in the Flush():
			// - make sure Shrink doesn't happen concurrently with sample processing.
			// - we don't need to Shrink() on stop
			agg.tagsStore.Shrink()

			aggregatorEventPlatformErrorLogged = false
		case <-agg.health.C:
		case checkItem := <-agg.checkItems:
			checkItem.handle(agg)
		case event := <-agg.eventIn:
			aggregatorEvent.Add(1)
			tlmProcessed.Inc("", "events")
			agg.addEvent(event)
		case serviceCheck := <-agg.serviceCheckIn:
			aggregatorServiceCheck.Add(1)
			tlmProcessed.Inc("", "service_checks")
			agg.addServiceCheck(serviceCheck)
		case serviceChecks := <-agg.bufferedServiceCheckIn:
			aggregatorServiceCheck.Add(int64(len(serviceChecks)))
			tlmProcessed.Add(float64(len(serviceChecks)), "", "service_checks")
			for _, serviceCheck := range serviceChecks {
				agg.addServiceCheck(*serviceCheck)
			}
		case events := <-agg.bufferedEventIn:
			aggregatorEvent.Add(int64(len(events)))
			tlmProcessed.Add(float64(len(events)), "", "events")
			for _, event := range events {
				agg.addEvent(*event)
			}
		case orchestratorMetadata := <-agg.orchestratorMetadataIn:
			aggregatorOrchestratorMetadata.Add(1)
			// each resource has its own payload so we cannot aggregate
			// use a routine to avoid blocking the aggregator
			go func(orchestratorMetadata senderOrchestratorMetadata) {
				err := agg.serializer.SendOrchestratorMetadata(
					orchestratorMetadata.msgs,
					agg.hostname,
					orchestratorMetadata.clusterID,
					orchestratorMetadata.payloadType,
				)
				if err != nil {
					aggregatorOrchestratorMetadataErrors.Add(1)
					log.Errorf("Error submitting orchestrator data: %s", err)
				}
			}(orchestratorMetadata)
		case orchestratorManifest := <-agg.orchestratorManifestIn:
			// each resource send manifests but as it's the same message
			// we can use the aggregator to buffer them
			agg.addOrchestratorManifest(&orchestratorManifest)
		case event := <-agg.eventPlatformIn:
			state := stateOk
			tlmProcessed.Inc("", event.eventType)
			aggregatorEventPlatformEvents.Add(event.eventType, 1)
			err := agg.handleEventPlatformEvent(event)
			if err != nil {
				state = stateError
				aggregatorEventPlatformEventsErrors.Add(event.eventType, 1)
				log.Debugf("error submitting event platform event: %s", err)
				if !aggregatorEventPlatformErrorLogged {
					log.Warnf("Failed to process some event platform events. error='%s' eventCounts=%s errorCounts=%s", err, aggregatorEventPlatformEvents.String(), aggregatorEventPlatformEventsErrors.String())
					aggregatorEventPlatformErrorLogged = true
				}
			}
			tlmFlush.Add(1, event.eventType, state)
		}
	}
}

// tags returns the list of tags that should be added to the agent telemetry metrics
// Container agent tags may be missing in the first seconds after agent startup
func (agg *BufferedAggregator) tags(withVersion bool) []string {
	var tags []string

	var err error
	tags, err = agg.globalTags(agg.tagger.ChecksCardinality())
	if err != nil {
		log.Debugf("Couldn't get Global tags: %v", err)
	}

	if agg.tlmContainerTagsEnabled {
		agentTags, err := agg.agentTags(agg.tagger.ChecksCardinality())
		if err == nil {
			if tags == nil {
				tags = agentTags
			} else {
				tags = append(tags, agentTags...)
			}
		} else {
			log.Debugf("Couldn't get Agent tags: %v", err)
		}
	}
	if withVersion {
		tags = append(tags, "version:"+version.AgentVersion)
		if version.AgentPackageVersion != "" {
			tags = append(tags, "package_version:"+version.AgentPackageVersion)
		}
	}
	// nil to empty string
	// This is expected by other components/tests
	if tags == nil {
		tags = []string{}
	}
	return tags
}

func (agg *BufferedAggregator) updateChecksTelemetry() {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	t := metrics.CheckMetricsTelemetryAccumulator{}
	for _, sampler := range agg.checkSamplers {
		t.VisitCheckMetrics(&sampler.metrics)
	}
	t.Flush()
}

// deregisterSampler is an item sent internally by the aggregator to
// signal that the sender will no longer will be used for a given
// checkid.ID.
//
// 1. A check is unscheduled while running.
//
// 2. Check sends metrics, sketches and eventually a commit, using the
// sender to the senderItems channel.
//
// 3. Check collector calls deregisterSampler() immediately as check's
// Run() completes.
//
// 4. Metrics are still in the channel, we can't drop the
// sampler. Instead we add another message to the channel (this type)
// to indicate that the sampler will no longer be in use once all the
// metrics are handled.
//
// 5. Aggregator handles metrics and the commit message. Sampler now
// contains metrics from the check.
//
// 6. Aggragator handles this message. We can't drop the sender now
// since it still contains metrics from the last run. Instead, we set
// a flag that the sampler can be dropped after the next flush.
//
// 7. Aggregator flushes metrics from the sampler and can now remove
// it: we know for sure that no more metrics will arrive.
type deregisterSampler struct {
	id checkid.ID
}

func (s *deregisterSampler) handle(agg *BufferedAggregator) {
	agg.handleDeregisterSampler(s.id)
}

func (agg *BufferedAggregator) handleDeregisterSampler(id checkid.ID) {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	if cs, ok := agg.checkSamplers[id]; ok {
		cs.deregistered = true
	}
}

// registerSampler is an item sent internally by the aggregator to
// register a new sampler or re-use an existing one.
//
// This handles a situation when a check is descheduled and then
// immediately re-scheduled again, within one Flush interval.
//
// We cannot immediately remove `deregistered` flag from the sampler,
// since the deregisterSampler message may still be in the queue when
// the check is re-scheduled. If registerSampler is called before
// the check runs, we will have a sampler for it one way or another.
type registerSampler struct {
	id checkid.ID
}

func (s *registerSampler) handle(agg *BufferedAggregator) {
	agg.handleRegisterSampler(s.id)
}

func (agg *BufferedAggregator) handleRegisterSampler(id checkid.ID) {
	agg.mu.Lock()
	defer agg.mu.Unlock()

	if cs, ok := agg.checkSamplers[id]; ok {
		cs.deregistered = false
		log.Debugf("Sampler with ID '%s' has already been registered, will use existing sampler", id)
		return
	}
	agg.checkSamplers[id] = newCheckSampler(
		pkgconfigsetup.Datadog().GetInt("check_sampler_bucket_commits_count_expiry"),
		pkgconfigsetup.Datadog().GetBool("check_sampler_expire_metrics"),
		pkgconfigsetup.Datadog().GetBool("check_sampler_context_metrics"),
		pkgconfigsetup.Datadog().GetDuration("check_sampler_stateful_metric_expiration_time"),
		agg.tagsStore,
		id,
		agg.tagger,
	)
}
