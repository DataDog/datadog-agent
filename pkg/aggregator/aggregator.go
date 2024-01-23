// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package aggregator

import (
	"expvar"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func expMetricTags() interface{} {
	panic("not called")
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
	eventPlatformForwarder epforwarder.EventPlatformForwarder
	hostname               string
	hostnameUpdate         chan string
	hostnameUpdateDone     chan struct{} // signals that the hostname update is finished
	flushChan              chan flushTrigger

	stopChan  chan struct{}
	health    *health.Handle
	agentName string // Name of the agent for telemetry metrics

	tlmContainerTagsEnabled bool                                              // Whether we should call the tagger to tag agent telemetry metrics
	agentTags               func(collectors.TagCardinality) ([]string, error) // This function gets the agent tags from the tagger (defined as a struct field to ease testing)
	globalTags              func(collectors.TagCardinality) ([]string, error) // This function gets global tags from the tagger when host tags are not available

	flushAndSerializeInParallel FlushAndSerializeInParallel
}

// FlushAndSerializeInParallel contains options for flushing metrics and serializing in parallel.
type FlushAndSerializeInParallel struct {
	ChannelSize int
	BufferSize  int
}

// NewFlushAndSerializeInParallel creates a new instance of FlushAndSerializeInParallel.
func NewFlushAndSerializeInParallel(config config.Config) FlushAndSerializeInParallel {
	return FlushAndSerializeInParallel{
		BufferSize:  config.GetInt("aggregator_flush_metrics_and_serialize_in_parallel_buffer_size"),
		ChannelSize: config.GetInt("aggregator_flush_metrics_and_serialize_in_parallel_chan_size"),
	}
}

// NewBufferedAggregator instantiates a BufferedAggregator
func NewBufferedAggregator(s serializer.MetricSerializer, eventPlatformForwarder epforwarder.EventPlatformForwarder, hostname string, flushInterval time.Duration) *BufferedAggregator {
	panic("not called")
}

func (agg *BufferedAggregator) addOrchestratorManifest(manifests *senderOrchestratorManifest) {
	panic("not called")
}

// getOrchestratorManifests grabs the manifests from the queue and clears it
func (agg *BufferedAggregator) getOrchestratorManifests() []*senderOrchestratorManifest {
	panic("not called")
}

func (agg *BufferedAggregator) sendOrchestratorManifests(start time.Time, senderManifests []*senderOrchestratorManifest) {
	panic("not called")
}

// flushOrchestratorManifests serializes and forwards events in a separate goroutine
func (agg *BufferedAggregator) flushOrchestratorManifests(start time.Time, waitForSerializer bool) {
	panic("not called")
}

// AddRecurrentSeries adds a serie to the series that are sent at every flush
func AddRecurrentSeries(newSerie *metrics.Serie) {
	panic("not called")
}

// IsInputQueueEmpty returns true if every input channel for the aggregator are
// empty. This is mainly useful for tests and benchmark
func (agg *BufferedAggregator) IsInputQueueEmpty() bool {
	panic("not called")
}

// GetBufferedChannels returns a channel which can be subsequently used to send Event or ServiceCheck.
func (agg *BufferedAggregator) GetBufferedChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck) {
	panic("not called")
}

// GetEventPlatformForwarder returns a event platform forwarder
func (agg *BufferedAggregator) GetEventPlatformForwarder() (epforwarder.EventPlatformForwarder, error) {
	panic("not called")
}

func (agg *BufferedAggregator) registerSender(id checkid.ID) error {
	panic("not called")
}

func (agg *BufferedAggregator) deregisterSender(id checkid.ID) {
	panic("not called")
}

func (agg *BufferedAggregator) handleSenderSample(ss senderMetricSample) {
	panic("not called")
}

func (agg *BufferedAggregator) handleSenderBucket(checkBucket senderHistogramBucket) {
	panic("not called")
}

func (agg *BufferedAggregator) handleEventPlatformEvent(event senderEventPlatformEvent) error {
	panic("not called")
}

// addServiceCheck adds the service check to the slice of current service checks
func (agg *BufferedAggregator) addServiceCheck(sc servicecheck.ServiceCheck) {
	panic("not called")
}

// addEvent adds the event to the slice of current events
func (agg *BufferedAggregator) addEvent(e event.Event) {
	panic("not called")
}

// GetSeriesAndSketches grabs all the series & sketches from the queue and clears the queue
// The parameter `before` is used as an end interval while retrieving series and sketches
// from the time sampler. Metrics and sketches before this timestamp should be returned.
func (agg *BufferedAggregator) GetSeriesAndSketches(before time.Time) (metrics.Series, metrics.SketchSeriesList) {
	panic("not called")
}

// getSeriesAndSketches grabs all the series & sketches from the queue and clears the queue
// The parameter `before` is used as an end interval while retrieving series and sketches
// from the time sampler. Metrics and sketches before this timestamp should be returned.
func (agg *BufferedAggregator) getSeriesAndSketches(
	before time.Time,
	seriesSink metrics.SerieSink,
	sketchesSink metrics.SketchesSink,
) {
	panic("not called")
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
	panic("not called")
}

func (agg *BufferedAggregator) appendDefaultSeries(start time.Time, series metrics.SerieSink) {
	panic("not called")
}

func (agg *BufferedAggregator) flushSeriesAndSketches(trigger flushTrigger) {
	panic("not called")
}

// GetServiceChecks grabs all the service checks from the queue and clears the queue
func (agg *BufferedAggregator) GetServiceChecks() servicecheck.ServiceChecks {
	panic("not called")
}

func (agg *BufferedAggregator) sendServiceChecks(start time.Time, serviceChecks servicecheck.ServiceChecks) {
	panic("not called")
}

func (agg *BufferedAggregator) flushServiceChecks(start time.Time, waitForSerializer bool) {
	panic("not called")
}

// GetEvents grabs the events from the queue and clears it
func (agg *BufferedAggregator) GetEvents() event.Events {
	panic("not called")
}

// GetEventPlatformEvents grabs the event platform events from the queue and clears them.
// Note that this works only if using the 'noop' event platform forwarder
func (agg *BufferedAggregator) GetEventPlatformEvents() map[string][]*message.Message {
	panic("not called")
}

func (agg *BufferedAggregator) sendEvents(start time.Time, events event.Events) {
	panic("not called")
}

// flushEvents serializes and forwards events in a separate goroutine
func (agg *BufferedAggregator) flushEvents(start time.Time, waitForSerializer bool) {
	panic("not called")
}

// Flush flushes the data contained in the BufferedAggregator into the Forwarder.
// This method can be called from multiple routines.
func (agg *BufferedAggregator) Flush(trigger flushTrigger) {
	panic("not called")
}

// Stop stops the aggregator.
func (agg *BufferedAggregator) Stop() {
	panic("not called")
}

func (agg *BufferedAggregator) run() {
	panic("not called")
}

// tags returns the list of tags that should be added to the agent telemetry metrics
// Container agent tags may be missing in the first seconds after agent startup
func (agg *BufferedAggregator) tags(withVersion bool) []string {
	panic("not called")
}

func (agg *BufferedAggregator) updateChecksTelemetry() {
	panic("not called")
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
	panic("not called")
}

func (agg *BufferedAggregator) handleDeregisterSampler(id checkid.ID) {
	panic("not called")
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
	panic("not called")
}

func (agg *BufferedAggregator) handleRegisterSampler(id checkid.ID) {
	panic("not called")
}
