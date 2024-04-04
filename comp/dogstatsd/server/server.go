// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"context"
	"expvar"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

var (
	dogstatsdExpvars                  = expvar.NewMap("dogstatsd")
	dogstatsdServiceCheckParseErrors  = expvar.Int{}
	dogstatsdServiceCheckPackets      = expvar.Int{}
	dogstatsdEventParseErrors         = expvar.Int{}
	dogstatsdEventPackets             = expvar.Int{}
	dogstatsdMetricParseErrors        = expvar.Int{}
	dogstatsdMetricPackets            = expvar.Int{}
	dogstatsdPacketsLastSec           = expvar.Int{}
	dogstatsdUnterminatedMetricErrors = expvar.Int{}

	tlmProcessed = telemetry.NewCounter("dogstatsd", "processed",
		[]string{"message_type", "state", "origin"}, "Count of service checks/events/metrics processed by dogstatsd")
	tlmProcessedOk    = tlmProcessed.WithValues("metrics", "ok", "")
	tlmProcessedError = tlmProcessed.WithValues("metrics", "error", "")

	// while we try to add the origin tag in the tlmProcessed metric, we want to
	// avoid having it growing indefinitely, hence this safeguard to limit the
	// size of this cache for long-running agent or environment with a lot of
	// different container IDs.
	maxOriginCounters = 200

	tlmChannel            = telemetry.NewHistogramNoOp()
	defaultChannelBuckets = []float64{100, 250, 500, 1000, 10000}
	once                  sync.Once
)

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Demultiplexer aggregator.Demultiplexer

	Log    logComponent.Component
	Config configComponent.Component
	Debug  serverdebug.Component
	Replay replay.Component
	PidMap pidmap.Component
	Params Params
	WMeta  optional.Option[workloadmeta.Component]
}

// When the internal telemetry is enabled, used to tag the origin
// on the processed metric.
type cachedOriginCounter struct {
	origin string
	ok     map[string]string
	err    map[string]string
	okCnt  telemetry.SimpleCounter
	errCnt telemetry.SimpleCounter
}

// Server represent a Dogstatsd server
type server struct {
	log    logComponent.Component
	config config.Reader
	// listeners are the instantiated socket listener (UDS or UDP or both)
	listeners []listeners.StatsdListener

	// demultiplexer will receive the metrics processed by the DogStatsD server,
	// will take care of processing them concurrently if possible, and will
	// also take care of forwarding the metrics to the intake.
	demultiplexer aggregator.Demultiplexer

	// running in their own routine, workers are responsible of parsing the packets
	// and pushing them to the aggregator
	workers []*worker

	packetsIn               chan packets.Packets
	captureChan             chan packets.Packets
	serverlessFlushChan     chan bool
	sharedPacketPool        *packets.Pool
	sharedPacketPoolManager *packets.PoolManager
	sharedFloat64List       *float64ListPool
	Statistics              *util.Stats
	Started                 bool
	stopChan                chan bool
	health                  *health.Handle
	histToDist              bool
	histToDistPrefix        string
	extraTags               []string
	Debug                   serverdebug.Component

	tCapture                replay.Component
	pidMap                  pidmap.Component
	mapper                  *mapper.MetricMapper
	eolTerminationUDP       bool
	eolTerminationUDS       bool
	eolTerminationNamedPipe bool
	// disableVerboseLogs is a feature flag to disable the logs capable
	// of flooding the logger output (e.g. parsing messages error).
	// NOTE(remy): this should probably be dropped and use a throttler logger, see
	// package (pkg/trace/log/throttled.go) for a possible throttler implementation.
	disableVerboseLogs bool

	// cachedTlmLock must be held when accessing cachedOriginCounters and cachedOrder
	cachedTlmLock sync.Mutex
	// cachedOriginCounters caches telemetry counter per origin
	// (when dogstatsd origin telemetry is enabled)
	// TODO: use lru.Cache and track listenerId too
	cachedOriginCounters map[string]cachedOriginCounter
	cachedOrder          []cachedOriginCounter // for cache eviction

	// ServerlessMode is set to true if we're running in a serverless environment.
	ServerlessMode     bool
	udsListenerRunning bool
	udpLocalAddr       string

	// originTelemetry is true if we want to report telemetry per origin.
	originTelemetry bool

	enrichConfig enrichConfig

	wmeta optional.Option[workloadmeta.Component]
}

func initTelemetry(cfg config.Reader, logger logComponent.Component) {
	dogstatsdExpvars.Set("ServiceCheckParseErrors", &dogstatsdServiceCheckParseErrors)
	dogstatsdExpvars.Set("ServiceCheckPackets", &dogstatsdServiceCheckPackets)
	dogstatsdExpvars.Set("EventParseErrors", &dogstatsdEventParseErrors)
	dogstatsdExpvars.Set("EventPackets", &dogstatsdEventPackets)
	dogstatsdExpvars.Set("MetricParseErrors", &dogstatsdMetricParseErrors)
	dogstatsdExpvars.Set("MetricPackets", &dogstatsdMetricPackets)
	dogstatsdExpvars.Set("UnterminatedMetricErrors", &dogstatsdUnterminatedMetricErrors)

	get := func(option string) []float64 {
		if !cfg.IsSet(option) {
			return nil
		}

		buckets, err := cfg.GetFloat64SliceE(option)
		if err != nil {
			logger.Errorf("%s, falling back to default values", err)
			return nil
		}
		if len(buckets) == 0 {
			logger.Debugf("'%s' is empty, falling back to default values", option)
			return nil
		}
		return buckets
	}

	buckets := get("telemetry.dogstatsd.aggregator_channel_latency_buckets")
	if buckets == nil {
		buckets = defaultChannelBuckets
	}

	tlmChannel = telemetry.NewHistogram(
		"dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		buckets)

	listeners.InitTelemetry(get("telemetry.dogstatsd.listeners_latency_buckets"))
	packets.InitTelemetry(get("telemetry.dogstatsd.listeners_channel_latency_buckets"))
}

// TODO: (components) - merge with newServerCompat once NewServerlessServer is removed
func newServer(deps dependencies) Component {
	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, deps.Params.Serverless, deps.Demultiplexer, deps.WMeta, deps.PidMap)

	if config.Datadog.GetBool("use_dogstatsd") {
		deps.Lc.Append(fx.Hook{
			OnStart: s.startHook,
			OnStop:  s.stop,
		})
	}

	return s

}

func newServerCompat(cfg config.Reader, log logComponent.Component, capture replay.Component, debug serverdebug.Component, serverless bool, demux aggregator.Demultiplexer, wmeta optional.Option[workloadmeta.Component], pidMap pidmap.Component) *server {
	// This needs to be done after the configuration is loaded
	once.Do(func() { initTelemetry(cfg, log) })

	var stats *util.Stats
	if cfg.GetBool("dogstatsd_stats_enable") {
		buff := cfg.GetInt("dogstatsd_stats_buffer")
		s, err := util.NewStats(uint32(buff))
		if err != nil {
			log.Errorf("Dogstatsd: unable to start statistics facilities")
		}
		stats = s
		dogstatsdExpvars.Set("PacketsLastSecond", &dogstatsdPacketsLastSec)
	}

	// check configuration for custom namespace
	metricPrefix := cfg.GetString("statsd_metric_namespace")
	if metricPrefix != "" && !strings.HasSuffix(metricPrefix, ".") {
		metricPrefix = metricPrefix + "."
	}

	metricPrefixBlacklist := cfg.GetStringSlice("statsd_metric_namespace_blacklist")
	metricBlocklist := newBlocklist(
		cfg.GetStringSlice("statsd_metric_blocklist"),
		cfg.GetBool("statsd_metric_blocklist_match_prefix"),
	)

	defaultHostname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Errorf("Dogstatsd: unable to determine default hostname: %s", err.Error())
	}

	histToDist := cfg.GetBool("histogram_copy_to_distribution")
	histToDistPrefix := cfg.GetString("histogram_copy_to_distribution_prefix")

	extraTags := cfg.GetStringSlice("dogstatsd_tags")

	// if the server is running in a context where static tags are required, add those
	// to extraTags.
	if staticTags := util.GetStaticTagsSlice(context.TODO()); staticTags != nil {
		extraTags = append(extraTags, staticTags...)
	}
	util.SortUniqInPlace(extraTags)

	entityIDPrecedenceEnabled := cfg.GetBool("dogstatsd_entity_id_precedence")

	eolTerminationUDP := false
	eolTerminationUDS := false
	eolTerminationNamedPipe := false

	for _, v := range cfg.GetStringSlice("dogstatsd_eol_required") {
		switch v {
		case "udp":
			eolTerminationUDP = true
		case "uds":
			eolTerminationUDS = true
		case "named_pipe":
			eolTerminationNamedPipe = true
		default:
			log.Errorf("Invalid dogstatsd_eol_required value: %s", v)
		}
	}

	s := &server{
		log:                     log,
		config:                  cfg,
		Started:                 false,
		Statistics:              stats,
		packetsIn:               nil,
		captureChan:             nil,
		sharedPacketPool:        nil,
		sharedPacketPoolManager: nil,
		sharedFloat64List:       newFloat64ListPool(),
		demultiplexer:           demux,
		listeners:               nil,
		stopChan:                make(chan bool),
		serverlessFlushChan:     make(chan bool),
		health:                  nil,
		histToDist:              histToDist,
		histToDistPrefix:        histToDistPrefix,
		extraTags:               extraTags,
		eolTerminationUDP:       eolTerminationUDP,
		eolTerminationUDS:       eolTerminationUDS,
		eolTerminationNamedPipe: eolTerminationNamedPipe,
		disableVerboseLogs:      cfg.GetBool("dogstatsd_disable_verbose_logs"),
		Debug:                   debug,
		originTelemetry: cfg.GetBool("telemetry.enabled") &&
			cfg.GetBool("telemetry.dogstatsd_origin"),
		tCapture:             capture,
		pidMap:               pidMap,
		udsListenerRunning:   false,
		cachedOriginCounters: make(map[string]cachedOriginCounter),
		ServerlessMode:       serverless,
		enrichConfig: enrichConfig{
			metricPrefix:              metricPrefix,
			metricPrefixBlacklist:     metricPrefixBlacklist,
			metricBlocklist:           metricBlocklist,
			entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
			defaultHostname:           defaultHostname,
			serverlessMode:            serverless,
		},
		wmeta: wmeta,
	}

	return s
}

func (s *server) startHook(context context.Context) error {

	err := s.start(context)
	if err != nil {
		s.log.Errorf("Could not start dogstatsd: %s", err)
	} else {
		s.log.Debug("dogstatsd started")
	}
	return nil
}

func (s *server) start(context.Context) error {

	packetsChannel := make(chan packets.Packets, s.config.GetInt("dogstatsd_queue_size"))
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	if err := s.tCapture.GetStartUpError(); err != nil {
		return err
	}

	// sharedPacketPool is used by the packet assembler to retrieve already allocated
	// buffer in order to avoid allocation. The packets are pushed back by the server.
	sharedPacketPool := packets.NewPool(s.config.GetInt("dogstatsd_buffer_size"))
	sharedPacketPoolManager := packets.NewPoolManager(sharedPacketPool)

	udsListenerRunning := false

	socketPath := s.config.GetString("dogstatsd_socket")
	socketStreamPath := s.config.GetString("dogstatsd_stream_socket")
	originDetection := s.config.GetBool("dogstatsd_origin_detection")
	var sharedUDSOobPoolManager *packets.PoolManager
	if originDetection {
		sharedUDSOobPoolManager = listeners.NewUDSOobPoolManager()
	}

	if s.tCapture != nil {
		err := s.tCapture.RegisterSharedPoolManager(sharedPacketPoolManager)
		if err != nil {
			s.log.Errorf("Can't register shared pool manager: %s", err.Error())
		}
		err = s.tCapture.RegisterOOBPoolManager(sharedUDSOobPoolManager)
		if err != nil {
			s.log.Errorf("Can't register OOB pool manager: %s", err.Error())
		}
	}

	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUDSDatagramListener(packetsChannel, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap)
		if err != nil {
			s.log.Errorf("Can't init listener: %s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
			udsListenerRunning = true
		}
	}

	if len(socketStreamPath) > 0 {
		s.log.Warnf("dogstatsd_stream_socket is not yet supported, run it at your own risk")
		unixListener, err := listeners.NewUDSStreamListener(packetsChannel, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap)
		if err != nil {
			s.log.Errorf("Can't init listener: %s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}

	if s.config.GetString("dogstatsd_port") == listeners.RandomPortName || s.config.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetsChannel, sharedPacketPoolManager, s.config, s.tCapture)
		if err != nil {
			s.log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, udpListener)
			s.udpLocalAddr = udpListener.LocalAddr()
		}
	}

	pipeName := s.config.GetString("dogstatsd_pipe_name")
	if len(pipeName) > 0 {
		namedPipeListener, err := listeners.NewNamedPipeListener(pipeName, packetsChannel, sharedPacketPoolManager, s.config, s.tCapture)
		if err != nil {
			s.log.Errorf("named pipe error: %v", err.Error())
		} else {
			tmpListeners = append(tmpListeners, namedPipeListener)
		}
	}

	if len(tmpListeners) == 0 {
		return fmt.Errorf("listening on neither udp nor socket, please check your configuration")
	}

	s.udsListenerRunning = udsListenerRunning
	s.packetsIn = packetsChannel
	s.captureChan = packetsChannel
	s.sharedPacketPool = sharedPacketPool
	s.sharedPacketPoolManager = sharedPacketPoolManager
	s.listeners = tmpListeners

	// packets forwarding
	// ----------------------

	forwardHost := s.config.GetString("statsd_forward_host")
	forwardPort := s.config.GetInt("statsd_forward_port")
	if forwardHost != "" && forwardPort != 0 {
		forwardAddress := fmt.Sprintf("%s:%d", forwardHost, forwardPort)
		con, err := net.Dial("udp", forwardAddress)
		if err != nil {
			s.log.Warnf("Could not connect to statsd forward host : %s", err)
		} else {
			s.packetsIn = make(chan packets.Packets, s.config.GetInt("dogstatsd_queue_size"))
			go s.forwarder(con)
		}
	}

	// start the workers processing the packets read on the socket
	// ----------------------

	s.health = health.RegisterLiveness("dogstatsd-main")
	s.handleMessages()
	s.Started = true

	// start the debug loop
	// ----------------------

	if s.config.GetBool("dogstatsd_metrics_stats_enable") {
		s.log.Info("Dogstatsd: metrics statistics will be stored.")
		s.Debug.SetMetricStatsEnabled(true)
	}

	// map some metric name
	// ----------------------

	cacheSize := s.config.GetInt("dogstatsd_mapper_cache_size")

	mappings, err := config.GetDogstatsdMappingProfiles()
	if err != nil {
		s.log.Warnf("Could not parse mapping profiles: %v", err)
	} else if len(mappings) != 0 {
		mapperInstance, err := mapper.NewMetricMapper(mappings, cacheSize)
		if err != nil {
			s.log.Warnf("Could not create metric mapper: %v", err)
		} else {
			s.mapper = mapperInstance
		}
	}
	return nil
}

func (s *server) stop(context.Context) error {
	if !s.IsRunning() {
		return nil
	}
	close(s.stopChan)
	for _, l := range s.listeners {
		l.Stop()
	}
	if s.Statistics != nil {
		s.Statistics.Stop()
	}
	if s.tCapture != nil {
		s.tCapture.StopCapture()
	}
	s.health.Deregister() //nolint:errcheck
	s.Started = false

	return nil
}

func (s *server) IsRunning() bool {
	return s.Started
}

// SetExtraTags sets extra tags. All metrics sent to the DogstatsD will be tagged with them.
func (s *server) SetExtraTags(tags []string) {
	s.extraTags = tags
}

func (s *server) handleMessages() {
	if s.Statistics != nil {
		go s.Statistics.Process()
		go s.Statistics.Update(&dogstatsdPacketsLastSec)
	}

	for _, l := range s.listeners {
		l.Listen()
	}

	workersCount, _ := aggregator.GetDogStatsDWorkerAndPipelineCount()

	// undocumented configuration field to force the amount of dogstatsd workers
	// mainly used for benchmarks or some very specific use-case.
	if configWC := s.config.GetInt("dogstatsd_workers_count"); configWC != 0 {
		s.log.Debug("Forcing the amount of DogStatsD workers to:", configWC)
		workersCount = configWC
	}

	s.log.Debug("DogStatsD will run", workersCount, "workers")

	for i := 0; i < workersCount; i++ {
		worker := newWorker(s, i, s.wmeta)
		go worker.run()
		s.workers = append(s.workers, worker)
	}
}

func (s *server) UDPLocalAddr() string {
	return s.udpLocalAddr
}

func (s *server) forwarder(fcon net.Conn) {
	for {
		select {
		case <-s.stopChan:
			return
		case packets := <-s.captureChan:
			for _, packet := range packets {
				_, err := fcon.Write(packet.Contents)

				if err != nil {
					s.log.Warnf("Forwarding packet failed : %s", err)
				}
			}
			s.packetsIn <- packets
		}
	}
}

// ServerlessFlush flushes all the data to the aggregator to them send it to the Datadog intake.
func (s *server) ServerlessFlush(sketchesBucketDelay time.Duration) {
	s.log.Debug("Received a Flush trigger")

	// make all workers flush their aggregated data (in the batchers) into the time samplers
	s.serverlessFlushChan <- true

	start := time.Now()
	// flush the aggregator to have the serializer/forwarder send data to the backend.
	// We add 10 seconds to the interval to ensure that we're getting the whole sketches bucket
	s.demultiplexer.ForceFlushToSerializer(start.Add(sketchesBucketDelay), true)
}

// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}

// ScanLines is an almost identical reimplementation of bufio.ScanLines, but also
// reports if the returned line is newline-terminated
func ScanLines(data []byte, atEOF bool) (advance int, token []byte, eol bool, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, false, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, dropCR(data[0:i]), true, nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), dropCR(data), false, nil
	}
	// Request more data.
	return 0, nil, false, nil
}

func nextMessage(packet *[]byte, eolTermination bool) (message []byte) {
	if len(*packet) == 0 {
		return nil
	}

	advance, message, eol, err := ScanLines(*packet, true)
	if err != nil {
		return nil
	}

	if eolTermination && !eol {
		dogstatsdUnterminatedMetricErrors.Add(1)
		return nil
	}

	*packet = (*packet)[advance:]
	return message
}

func (s *server) UdsListenerRunning() bool {
	return s.udsListenerRunning
}

func (s *server) eolEnabled(sourceType packets.SourceType) bool {
	switch sourceType {
	case packets.UDS:
		return s.eolTerminationUDS
	case packets.UDP:
		return s.eolTerminationUDP
	case packets.NamedPipe:
		return s.eolTerminationNamedPipe
	}
	return false
}

func (s *server) errLog(format string, params ...interface{}) {
	if s.disableVerboseLogs {
		s.log.Debugf(format, params...)
	} else {
		s.log.Errorf(format, params...)
	}
}

// workers are running this function in their goroutine
func (s *server) parsePackets(batcher *batcher, parser *parser, packets []*packets.Packet, samples metrics.MetricSampleBatch) metrics.MetricSampleBatch {
	for _, packet := range packets {
		s.log.Tracef("Dogstatsd receive: %q", packet.Contents)
		for {
			message := nextMessage(&packet.Contents, s.eolEnabled(packet.Source))
			if message == nil {
				break
			}
			if len(message) == 0 {
				continue
			}
			if s.Statistics != nil {
				s.Statistics.StatEvent(1)
			}
			messageType := findMessageType(message)

			switch messageType {
			case serviceCheckType:
				serviceCheck, err := s.parseServiceCheckMessage(parser, message, packet.Origin)
				if err != nil {
					s.errLog("Dogstatsd: error parsing service check '%q': %s", message, err)
					continue
				}
				batcher.appendServiceCheck(serviceCheck)
			case eventType:
				event, err := s.parseEventMessage(parser, message, packet.Origin)
				if err != nil {
					s.errLog("Dogstatsd: error parsing event '%q': %s", message, err)
					continue
				}
				batcher.appendEvent(event)
			case metricSampleType:
				var err error

				samples = samples[0:0]

				samples, err = s.parseMetricMessage(samples, parser, message, packet.Origin, packet.ListenerID, s.originTelemetry)
				if err != nil {
					s.errLog("Dogstatsd: error parsing metric message '%q': %s", message, err)
					continue
				}

				for idx := range samples {
					s.Debug.StoreMetricStats(samples[idx])

					if samples[idx].Timestamp > 0.0 {
						batcher.appendLateSample(samples[idx])
					} else {
						batcher.appendSample(samples[idx])
					}

					if s.histToDist && samples[idx].Mtype == metrics.HistogramType {
						distSample := samples[idx].Copy()
						distSample.Name = s.histToDistPrefix + distSample.Name
						distSample.Mtype = metrics.DistributionType
						batcher.appendSample(*distSample)
					}
				}
			}
		}
		s.sharedPacketPoolManager.Put(packet)
	}
	batcher.flush()
	return samples
}

// getOriginCounter returns a telemetry counter for processed metrics using the given origin as a tag.
// They are stored in cache to avoid heap escape.
// Only `maxOriginCounters` are stored to avoid an infinite expansion.
// Counters returned by `getOriginCounter` are thread safe.
func (s *server) getOriginCounter(origin string) (okCnt telemetry.SimpleCounter, errorCnt telemetry.SimpleCounter) {
	s.cachedTlmLock.Lock()
	defer s.cachedTlmLock.Unlock()

	if maps, ok := s.cachedOriginCounters[origin]; ok {
		return maps.okCnt, maps.errCnt
	}

	okMap := map[string]string{"message_type": "metrics", "state": "ok"}
	errorMap := map[string]string{"message_type": "metrics", "state": "error"}
	okMap["origin"] = origin
	errorMap["origin"] = origin
	maps := cachedOriginCounter{
		origin: origin,
		ok:     okMap,
		err:    errorMap,
		okCnt:  tlmProcessed.WithTags(okMap),
		errCnt: tlmProcessed.WithTags(errorMap),
	}

	s.cachedOriginCounters[origin] = maps
	s.cachedOrder = append(s.cachedOrder, maps)

	if len(s.cachedOrder) > maxOriginCounters {
		// remove the oldest one from the cache
		pop := s.cachedOrder[0]
		delete(s.cachedOriginCounters, pop.origin)
		s.cachedOrder = s.cachedOrder[1:]
		// remove it from the telemetry metrics as well
		tlmProcessed.DeleteWithTags(pop.ok)
		tlmProcessed.DeleteWithTags(pop.err)
	}

	return maps.okCnt, maps.errCnt
}

// NOTE(remy): for performance purpose, we may need to revisit this method to deal with both a metricSamples slice and a lateMetricSamples
// slice, in order to not having to test multiple times if a metric sample is a late one using the Timestamp attribute,
// which will be slower when processing millions of samples. It could use a boolean returned by `parseMetricSample` which
// is the first part aware of processing a late metric. Also, it may help us having a telemetry of a "late_metrics" type here
// which we can't do today.
func (s *server) parseMetricMessage(metricSamples []metrics.MetricSample, parser *parser, message []byte, origin string, listenerID string, originTelemetry bool) ([]metrics.MetricSample, error) {
	okCnt := tlmProcessedOk
	errorCnt := tlmProcessedError
	if origin != "" && originTelemetry {
		okCnt, errorCnt = s.getOriginCounter(origin)
	}

	sample, err := parser.parseMetricSample(message)
	if err != nil {
		dogstatsdMetricParseErrors.Add(1)
		errorCnt.Inc()
		return metricSamples, err
	}

	if s.mapper != nil {
		mapResult := s.mapper.Map(sample.name)
		if mapResult != nil {
			s.log.Tracef("Dogstatsd mapper: metric mapped from %q to %q with tags %v", sample.name, mapResult.Name, mapResult.Tags)
			sample.name = mapResult.Name
			sample.tags = append(sample.tags, mapResult.Tags...)
		}
	}

	metricSamples = enrichMetricSample(metricSamples, sample, origin, listenerID, s.enrichConfig)

	if len(sample.values) > 0 {
		s.sharedFloat64List.put(sample.values)
	}

	for idx := range metricSamples {
		// All metricSamples already share the same Tags slice. We can
		// extends the first one and reuse it for the rest.
		if idx == 0 {
			metricSamples[idx].Tags = append(metricSamples[idx].Tags, s.extraTags...)
		} else {
			metricSamples[idx].Tags = metricSamples[0].Tags
		}
		dogstatsdMetricPackets.Add(1)
		okCnt.Inc()
	}
	return metricSamples, nil
}

func (s *server) parseEventMessage(parser *parser, message []byte, origin string) (*event.Event, error) {
	sample, err := parser.parseEvent(message)
	if err != nil {
		dogstatsdEventParseErrors.Add(1)
		tlmProcessed.Inc("events", "error", "")
		return nil, err
	}
	event := enrichEvent(sample, origin, s.enrichConfig)
	event.Tags = append(event.Tags, s.extraTags...)
	tlmProcessed.Inc("events", "ok", "")
	dogstatsdEventPackets.Add(1)
	return event, nil
}

func (s *server) parseServiceCheckMessage(parser *parser, message []byte, origin string) (*servicecheck.ServiceCheck, error) {
	sample, err := parser.parseServiceCheck(message)
	if err != nil {
		dogstatsdServiceCheckParseErrors.Add(1)
		tlmProcessed.Inc("service_checks", "error", "")
		return nil, err
	}
	serviceCheck := enrichServiceCheck(sample, origin, s.enrichConfig)
	serviceCheck.Tags = append(serviceCheck.Tags, s.extraTags...)
	dogstatsdServiceCheckPackets.Add(1)
	tlmProcessed.Inc("service_checks", "ok", "")
	return serviceCheck, nil
}
