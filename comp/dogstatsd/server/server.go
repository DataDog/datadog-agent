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

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/sort"
	statutil "github.com/DataDog/datadog-agent/pkg/util/stat"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
	tagutil "github.com/DataDog/datadog-agent/pkg/util/tags"
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

	// while we try to add the origin tag in the tlmProcessed metric, we want to
	// avoid having it growing indefinitely, hence this safeguard to limit the
	// size of this cache for long-running agent or environment with a lot of
	// different container IDs.
	maxOriginCounters = 200

	defaultChannelBuckets = []float64{100, 250, 500, 1000, 10000}
	once                  sync.Once
)

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Demultiplexer aggregator.Demultiplexer

	Log       log.Component
	Config    configComponent.Component
	Debug     serverdebug.Component
	Replay    replay.Component
	PidMap    pidmap.Component
	Params    Params
	WMeta     option.Option[workloadmeta.Component]
	Telemetry telemetry.Component
	Hostname  hostnameinterface.Component
}

type provides struct {
	fx.Out

	Comp          Component
	StatsEndpoint api.AgentEndpointProvider
	RCListener    rctypes.ListenerProvider
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

type localBlocklistConfig struct {
	metricNames []string
	matchPrefix bool
}

// Server represent a Dogstatsd server
type server struct {
	log    log.Component
	config model.Reader
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
	sharedPacketPoolManager *packets.PoolManager[packets.Packet]
	sharedFloat64List       *float64ListPool
	Statistics              *statutil.Stats
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
	ServerlessMode bool
	udpLocalAddr   string

	// originTelemetry is true if we want to report telemetry per origin.
	originTelemetry bool

	enrichConfig
	localBlocklistConfig

	wmeta option.Option[workloadmeta.Component]

	// telemetry
	telemetry               telemetry.Component
	tlmProcessed            telemetry.Counter
	tlmProcessedOk          telemetry.SimpleCounter
	tlmProcessedError       telemetry.SimpleCounter
	tlmChannel              telemetry.Histogram
	listernersTelemetry     *listeners.TelemetryStore
	packetsTelemetry        *packets.TelemetryStore
	stringInternerTelemetry *stringInternerTelemetry
}

func initTelemetry() {
	dogstatsdExpvars.Set("ServiceCheckParseErrors", &dogstatsdServiceCheckParseErrors)
	dogstatsdExpvars.Set("ServiceCheckPackets", &dogstatsdServiceCheckPackets)
	dogstatsdExpvars.Set("EventParseErrors", &dogstatsdEventParseErrors)
	dogstatsdExpvars.Set("EventPackets", &dogstatsdEventPackets)
	dogstatsdExpvars.Set("MetricParseErrors", &dogstatsdMetricParseErrors)
	dogstatsdExpvars.Set("MetricPackets", &dogstatsdMetricPackets)
	dogstatsdExpvars.Set("UnterminatedMetricErrors", &dogstatsdUnterminatedMetricErrors)
}

// TODO: (components) - merge with newServerCompat once NewServerlessServer is removed
func newServer(deps dependencies) provides {
	s := newServerCompat(deps.Config, deps.Log, deps.Hostname, deps.Replay, deps.Debug, deps.Params.Serverless, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)

	if deps.Config.GetBool("use_dogstatsd") {
		deps.Lc.Append(fx.Hook{
			OnStart: s.startHook,
			OnStop:  s.stop,
		})
	}

	var rcListener rctypes.ListenerProvider
	rcListener.ListenerProvider = rctypes.RCListener{
		state.ProductMetricControl: s.onBlocklistUpdateCallback,
	}

	return provides{
		Comp:          s,
		StatsEndpoint: api.NewAgentEndpointProvider(s.writeStats, "/dogstatsd-stats", "GET"),
		RCListener:    rcListener,
	}
}

func newServerCompat(cfg model.Reader, log log.Component, hostname hostnameinterface.Component, capture replay.Component, debug serverdebug.Component, serverless bool, demux aggregator.Demultiplexer, wmeta option.Option[workloadmeta.Component], pidMap pidmap.Component, telemetrycomp telemetry.Component) *server {
	// This needs to be done after the configuration is loaded
	once.Do(func() { initTelemetry() })
	var stats *statutil.Stats
	if cfg.GetBool("dogstatsd_stats_enable") {
		buff := cfg.GetInt("dogstatsd_stats_buffer")
		s, err := statutil.NewStats(uint32(buff))
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

	defaultHostname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Errorf("Dogstatsd: unable to determine default hostname: %s", err.Error())
	}

	histToDist := cfg.GetBool("histogram_copy_to_distribution")
	histToDistPrefix := cfg.GetString("histogram_copy_to_distribution_prefix")

	extraTags := cfg.GetStringSlice("dogstatsd_tags")

	// if the server is running in a context where static tags are required, add those
	// to extraTags.
	if staticTags := tagutil.GetStaticTagsSlice(context.TODO(), cfg); staticTags != nil {
		extraTags = append(extraTags, staticTags...)
	}
	sort.UniqInPlace(extraTags)

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

	dogstatsdTelemetryCount := telemetrycomp.NewCounter("dogstatsd", "processed",
		[]string{"message_type", "state", "origin"}, "Count of service checks/events/metrics processed by dogstatsd")

	s := &server{
		log:                     log,
		config:                  cfg,
		Started:                 false,
		Statistics:              stats,
		packetsIn:               nil,
		captureChan:             nil,
		sharedPacketPool:        nil,
		sharedPacketPoolManager: nil,
		sharedFloat64List:       newFloat64ListPool(telemetrycomp),
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
		cachedOriginCounters: make(map[string]cachedOriginCounter),
		ServerlessMode:       serverless,
		enrichConfig: enrichConfig{
			metricPrefix:              metricPrefix,
			metricPrefixBlacklist:     metricPrefixBlacklist,
			entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
			defaultHostname:           defaultHostname,
			serverlessMode:            serverless,
		},
		wmeta:                   wmeta,
		telemetry:               telemetrycomp,
		tlmProcessed:            dogstatsdTelemetryCount,
		tlmProcessedOk:          dogstatsdTelemetryCount.WithValues("metrics", "ok", ""),
		tlmProcessedError:       dogstatsdTelemetryCount.WithValues("metrics", "error", ""),
		stringInternerTelemetry: newSiTelemetry(utils.IsTelemetryEnabled(cfg), telemetrycomp),
	}

	buckets := getBuckets(cfg, log, "telemetry.dogstatsd.aggregator_channel_latency_buckets")
	if buckets == nil {
		buckets = defaultChannelBuckets
	}

	s.tlmChannel = telemetrycomp.NewHistogram(
		"dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		buckets)

	s.listernersTelemetry = listeners.NewTelemetryStore(getBuckets(cfg, log, "telemetry.dogstatsd.listeners_latency_buckets"), telemetrycomp)
	s.packetsTelemetry = packets.NewTelemetryStore(getBuckets(cfg, log, "telemetry.dogstatsd.listeners_channel_latency_buckets"), telemetrycomp)

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
	sharedPacketPool := packets.NewPool(s.config.GetInt("dogstatsd_buffer_size"), s.packetsTelemetry)
	sharedPacketPoolManager := packets.NewPoolManager[packets.Packet](sharedPacketPool)

	socketPath := s.config.GetString("dogstatsd_socket")
	socketStreamPath := s.config.GetString("dogstatsd_stream_socket")
	originDetection := s.config.GetBool("dogstatsd_origin_detection")
	var sharedUDSOobPoolManager *packets.PoolManager[[]byte]
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
		unixListener, err := listeners.NewUDSDatagramListener(packetsChannel, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		if err != nil {
			s.log.Errorf("Can't init UDS listener on path %s: %s", socketPath, err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}

	if len(socketStreamPath) > 0 {
		s.log.Warnf("dogstatsd_stream_socket is not yet supported, run it at your own risk")
		unixListener, err := listeners.NewUDSStreamListener(packetsChannel, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		if err != nil {
			s.log.Errorf("Can't init listener: %s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}

	if s.config.GetString("dogstatsd_port") == listeners.RandomPortName || s.config.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetsChannel, sharedPacketPoolManager, s.config, s.tCapture, s.listernersTelemetry, s.packetsTelemetry)
		if err != nil {
			s.log.Errorf("%s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, udpListener)
			s.udpLocalAddr = udpListener.LocalAddr()
		}
	}

	pipeName := s.config.GetString("dogstatsd_pipe_name")
	if len(pipeName) > 0 {
		namedPipeListener, err := listeners.NewNamedPipeListener(pipeName, packetsChannel, sharedPacketPoolManager, s.config, s.tCapture, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		if err != nil {
			s.log.Errorf("named pipe error: %v", err.Error())
		} else {
			tmpListeners = append(tmpListeners, namedPipeListener)
		}
	}

	if len(tmpListeners) == 0 {
		return fmt.Errorf("listening on neither udp nor socket, please check your configuration")
	}

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

	s.health = health.RegisterLiveness("dogstatsd-main")

	// start the debug loop
	// ----------------------

	if s.config.GetBool("dogstatsd_metrics_stats_enable") {
		s.log.Info("Dogstatsd: metrics statistics will be stored.")
		s.Debug.SetMetricStatsEnabled(true)
	}

	// map some metric name
	// ----------------------

	cacheSize := s.config.GetInt("dogstatsd_mapper_cache_size")

	mappings, err := getDogstatsdMappingProfiles(s.config)
	if err != nil {
		s.log.Warn(err)
	} else if len(mappings) != 0 {
		mapperInstance, err := mapper.NewMetricMapper(mappings, cacheSize)
		if err != nil {
			s.log.Warnf("Could not create metric mapper: %v", err)
		} else {
			s.mapper = mapperInstance
		}
	}

	// start the workers processing the packets read on the socket
	// ----------------------

	s.handleMessages()
	s.Started = true
	return nil
}

func (s *server) stop(context.Context) error {
	if !s.IsRunning() {
		return nil
	}
	for _, l := range s.listeners {
		l.Stop()
	}
	close(s.stopChan)

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

// SetBlocklist updates the metric names blocklist on all running worker.
func (s *server) SetBlocklist(metricNames []string, matchPrefix bool) {
	s.log.Debugf("SetBlocklist with %d metrics", len(metricNames))

	// we will use two different blocklists:
	// - one with all the metrics names, with all values from `metricNames`
	// - one with only the metric names ending with histogram aggregates suffixes

	// only histogram metric names (including their aggregates suffixes)
	histoMetricNames := s.createHistogramsBlocklist(metricNames)

	// send the complete blocklist to all workers, the listening part of dogstatsd
	for _, worker := range s.workers {
		blocklist := utilstrings.NewBlocklist(metricNames, matchPrefix)
		worker.BlocklistUpdate <- blocklist
	}

	// send the histogram blocklist used right before flushing to the serializer
	histoBlocklist := utilstrings.NewBlocklist(histoMetricNames, matchPrefix)
	s.demultiplexer.SetTimeSamplersBlocklist(&histoBlocklist)
}

// create a list based on all `metricNames` but only containing metric names
// with histogram aggregates suffixes.
// TODO(remy): should we consider moving this in the metrics package instead?
func (s *server) createHistogramsBlocklist(metricNames []string) []string {
	aggrs := s.config.GetStringSlice("histogram_aggregates")

	percentiles := metrics.ParsePercentiles(s.config.GetStringSlice("histogram_percentiles"))
	percentileAggrs := make([]string, len(percentiles))
	for i, percentile := range percentiles {
		percentileAggrs[i] = fmt.Sprintf("%dpercentile", percentile)
	}

	histoMetricNames := []string{}
	for _, metricName := range metricNames {
		// metric names ending with a histogram aggregates
		for _, aggr := range aggrs {
			if strings.HasSuffix(metricName, "."+aggr) {
				histoMetricNames = append(histoMetricNames, metricName)
			}
		}
		// metric names ending with a percentile
		for _, percentileAggr := range percentileAggrs {
			if strings.HasSuffix(metricName, "."+percentileAggr) {
				histoMetricNames = append(histoMetricNames, metricName)
			}
		}
	}

	s.log.Debugf("SetBlocklist created a histograms subsets of %d metric names", len(histoMetricNames))
	return histoMetricNames
}

func (s *server) handleMessages() {
	if s.Statistics != nil {
		go s.Statistics.Process()
		go s.Statistics.Update(&dogstatsdPacketsLastSec)
	}

	// start the listeners

	for _, l := range s.listeners {
		l.Listen()
	}

	// create and start all the workers

	workersCount, _ := aggregator.GetDogStatsDWorkerAndPipelineCount()

	// undocumented configuration field to force the amount of dogstatsd workers
	// mainly used for benchmarks or some very specific use-case.
	if configWC := s.config.GetInt("dogstatsd_workers_count"); configWC != 0 {
		s.log.Debug("Forcing the amount of DogStatsD workers to:", configWC)
		workersCount = configWC
	}

	s.log.Debug("DogStatsD will run", workersCount, "workers")

	for i := 0; i < workersCount; i++ {
		worker := newWorker(s, i, s.wmeta, s.packetsTelemetry, s.stringInternerTelemetry)
		go worker.run()
		s.workers = append(s.workers, worker)
	}

	// init the metric names blocklist

	s.localBlocklistConfig = localBlocklistConfig{
		metricNames: s.config.GetStringSlice("statsd_metric_blocklist"),
		matchPrefix: s.config.GetBool("statsd_metric_blocklist_match_prefix"),
	}
	s.restoreBlocklistFromLocalConfig()
}

func (s *server) restoreBlocklistFromLocalConfig() {
	s.SetBlocklist(
		s.localBlocklistConfig.metricNames,
		s.localBlocklistConfig.matchPrefix,
	)
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

// scanLines is an almost identical reimplementation of bufio.scanLines, but also
// reports if the returned line is newline-terminated
func scanLines(data []byte, atEOF bool) (advance int, token []byte, eol bool, err error) {
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

	advance, message, eol, err := scanLines(*packet, true)
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
func (s *server) parsePackets(batcher dogstatsdBatcher, parser *parser, packets []*packets.Packet, samples metrics.MetricSampleBatch, blocklist *utilstrings.Blocklist) metrics.MetricSampleBatch {
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
				serviceCheck, err := s.parseServiceCheckMessage(parser, message, packet.Origin, packet.ProcessID)
				if err != nil {
					s.errLog("Dogstatsd: error parsing service check '%q': %s", message, err)
					continue
				}
				batcher.appendServiceCheck(serviceCheck)
			case eventType:
				event, err := s.parseEventMessage(parser, message, packet.Origin, packet.ProcessID)
				if err != nil {
					s.errLog("Dogstatsd: error parsing event '%q': %s", message, err)
					continue
				}
				batcher.appendEvent(event)
			case metricSampleType:
				var err error

				samples = samples[0:0]

				samples, err = s.parseMetricMessage(samples, parser, message, packet.Origin, packet.ProcessID, packet.ListenerID, s.originTelemetry, blocklist)
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
		okCnt:  s.tlmProcessed.WithTags(okMap),
		errCnt: s.tlmProcessed.WithTags(errorMap),
	}

	s.cachedOriginCounters[origin] = maps
	s.cachedOrder = append(s.cachedOrder, maps)

	if len(s.cachedOrder) > maxOriginCounters {
		// remove the oldest one from the cache
		pop := s.cachedOrder[0]
		delete(s.cachedOriginCounters, pop.origin)
		s.cachedOrder = s.cachedOrder[1:]
		// remove it from the telemetry metrics as well
		s.tlmProcessed.DeleteWithTags(pop.ok)
		s.tlmProcessed.DeleteWithTags(pop.err)
	}

	return maps.okCnt, maps.errCnt
}

// NOTE(remy): for performance purpose, we may need to revisit this method to deal with both a metricSamples slice and a lateMetricSamples
// slice, in order to not having to test multiple times if a metric sample is a late one using the Timestamp attribute,
// which will be slower when processing millions of samples. It could use a boolean returned by `parseMetricSample` which
// is the first part aware of processing a late metric. Also, it may help us having a telemetry of a "late_metrics" type here
// which we can't do today.
func (s *server) parseMetricMessage(metricSamples []metrics.MetricSample, parser *parser, message []byte, origin string,
	processID uint32, listenerID string, originTelemetry bool, blocklist *utilstrings.Blocklist) ([]metrics.MetricSample, error) {
	okCnt := s.tlmProcessedOk
	errorCnt := s.tlmProcessedError
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

	metricSamples = enrichMetricSample(metricSamples, sample, origin, processID, listenerID, s.enrichConfig, blocklist)

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

		// If we're receiving runtime metrics, we need to convert the default source to the runtime source
		if s.enrichConfig.serverlessMode && strings.HasPrefix(metricSamples[idx].Name, "runtime.") {
			metricSamples[idx].Source = serverlessSourceCustomToRuntime(metricSamples[idx].Source)
		}

		dogstatsdMetricPackets.Add(1)
		okCnt.Inc()
	}
	return metricSamples, nil
}

func (s *server) parseEventMessage(parser *parser, message []byte, origin string, processID uint32) (*event.Event, error) {
	sample, err := parser.parseEvent(message)
	if err != nil {
		dogstatsdEventParseErrors.Add(1)
		s.tlmProcessed.Inc("events", "error", "")
		return nil, err
	}
	event := enrichEvent(sample, origin, processID, s.enrichConfig)
	event.Tags = append(event.Tags, s.extraTags...)
	s.tlmProcessed.Inc("events", "ok", "")
	dogstatsdEventPackets.Add(1)
	return event, nil
}

func (s *server) parseServiceCheckMessage(parser *parser, message []byte, origin string, processID uint32) (*servicecheck.ServiceCheck, error) {
	sample, err := parser.parseServiceCheck(message)
	if err != nil {
		dogstatsdServiceCheckParseErrors.Add(1)
		s.tlmProcessed.Inc("service_checks", "error", "")
		return nil, err
	}
	serviceCheck := enrichServiceCheck(sample, origin, processID, s.enrichConfig)
	serviceCheck.Tags = append(serviceCheck.Tags, s.extraTags...)
	dogstatsdServiceCheckPackets.Add(1)
	s.tlmProcessed.Inc("service_checks", "ok", "")
	return serviceCheck, nil
}

func getBuckets(cfg model.Reader, logger log.Component, option string) []float64 {
	if !cfg.IsSet(option) {
		return nil
	}

	buckets := cfg.GetFloat64Slice(option)
	if len(buckets) == 0 {
		logger.Debugf("'%s' is empty, falling back to default values", option)
		return nil
	}
	return buckets
}

func getDogstatsdMappingProfiles(cfg model.Reader) ([]mapper.MappingProfileConfig, error) {
	var mappings []mapper.MappingProfileConfig
	if cfg.IsSet("dogstatsd_mapper_profiles") {
		err := structure.UnmarshalKey(cfg, "dogstatsd_mapper_profiles", &mappings)
		if err != nil {
			return []mapper.MappingProfileConfig{}, fmt.Errorf("Could not parse dogstatsd_mapper_profiles: %v", err)
		}
	}
	return mappings, nil
}
