// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"bytes"
	"context"
	"errors"
	"expvar"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dsdconfig "github.com/DataDog/datadog-agent/comp/dogstatsd/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	server "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	offlinereporter "github.com/DataDog/datadog-agent/comp/offlinereporter/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
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
	compdef.In

	Lc compdef.Lifecycle

	Demultiplexer aggregator.Demultiplexer

	Log             log.Component
	Config          configComponent.Component
	Debug           serverdebug.Component
	Replay          replay.Component
	PidMap          pidmap.Component
	Params          server.Params
	WMeta           option.Option[workloadmeta.Component]
	Telemetry       telemetry.Component
	Hostname        hostnameinterface.Component
	FilterList      filterlist.Component
	OfflineReporter offlinereporter.Component
}

// Provides defines the output of the dogstatsd server component.
type Provides struct {
	compdef.Out

	Comp          server.Component
	StatsEndpoint api.AgentEndpointProvider
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
type dsdServer struct {
	log    log.Component
	config model.ReaderWriter
	// listeners are the instantiated socket listener (UDS or UDP or both)
	listeners []listeners.StatsdListener

	// demultiplexer will receive the metrics processed by the DogStatsD server,
	// will take care of processing them concurrently if possible, and will
	// also take care of forwarding the metrics to the intake.
	demultiplexer aggregator.Demultiplexer

	// running in their own routine, workers are responsible of parsing the packets
	// and pushing them to the aggregator
	workers []*worker

	packetsIn                chan packets.Packets
	captureChan              chan packets.Packets
	ingressLogShards         *packetIngressLogShards
	rawIngressShards         *packets.RawIngressShards
	compactRawIngressShards  *packets.CompactRawIngressShards
	rawIngressBatchDrain     bool
	rawIngressBatchDrainSize int
	workersCount             int
	serverlessFlushChan      chan bool
	sharedPacketPool         *packets.Pool
	sharedPacketPoolManager  *packets.PoolManager[packets.Packet]
	sharedFloat64List        *float64ListPool
	Statistics               *statutil.Stats
	Started                  bool
	startedMtx               sync.RWMutex
	stopChan                 chan bool
	health                   *health.Handle
	histToDist               bool
	histToDistPrefix         string
	extraTags                []string
	columnarV3DirectParse    bool
	Debug                    serverdebug.Component
	filterList               filterlist.Component

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

	wmeta           option.Option[workloadmeta.Component]
	offlineReporter offlinereporter.Component

	// telemetry
	telemetry               telemetry.Component
	tlmProcessed            telemetry.Counter
	tlmProcessedOk          telemetry.SimpleCounter
	tlmProcessedError       telemetry.SimpleCounter
	tlmChannel              telemetry.Histogram
	listernersTelemetry     *listeners.TelemetryStore
	packetsTelemetry        *packets.TelemetryStore
	stringInternerTelemetry *stringInternerTelemetry
	// Counter for absolute metric types
	tlmMetricTypes            telemetry.Counter
	tlmMetricTypeGauge        telemetry.SimpleCounter
	tlmMetricTypeCounter      telemetry.SimpleCounter
	tlmMetricTypeDistribution telemetry.SimpleCounter
	tlmMetricTypeHistogram    telemetry.SimpleCounter
	tlmMetricTypeSet          telemetry.SimpleCounter
	tlmMetricTypeTiming       telemetry.SimpleCounter
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
// NewComponent creates a new dogstatsd server component.
func NewComponent(deps dependencies) Provides {
	s := newServerCompat(deps.Config, deps.Log, deps.Hostname, deps.Replay, deps.Debug, deps.Params.Serverless, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry, deps.FilterList)
	s.offlineReporter = deps.OfflineReporter

	dsdConfig := dsdconfig.NewConfig(s.config)
	if dsdConfig.EnabledInternal() {
		deps.Lc.Append(compdef.Hook{
			OnStart: s.startHook,
			OnStop:  s.stop,
		})
	}

	return Provides{
		Comp:          s,
		StatsEndpoint: api.NewAgentEndpointProvider(s.writeStats, "/dogstatsd-stats", "GET"),
	}
}

func newServerCompat(cfg model.ReaderWriter, log log.Component, hostname hostnameinterface.Component, capture replay.Component, debug serverdebug.Component, serverless bool, demux aggregator.Demultiplexer, wmeta option.Option[workloadmeta.Component], pidMap pidmap.Component, telemetrycomp telemetry.Component, filterList filterlist.Component) *dsdServer {
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

	s := &dsdServer{
		log:                     log,
		config:                  cfg,
		Started:                 false,
		Statistics:              stats,
		packetsIn:               nil,
		captureChan:             nil,
		sharedPacketPool:        nil,
		sharedPacketPoolManager: nil,
		sharedFloat64List:       newFloat64ListPool(cfg, telemetrycomp),
		demultiplexer:           demux,
		listeners:               nil,
		stopChan:                make(chan bool),
		serverlessFlushChan:     make(chan bool),
		health:                  nil,
		histToDist:              histToDist,
		histToDistPrefix:        histToDistPrefix,
		extraTags:               extraTags,
		columnarV3DirectParse:   columnarV3DirectParseEnabled(),
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
		filterList:              filterList,
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

	// Initialize the metric type counters. These metrics are not
	// per-context but absolute.
	s.tlmMetricTypes = telemetrycomp.NewCounter("dogstatsd", "metric_type_count",
		[]string{"metric_type"}, "Count of metrics processed by dogstatsd by type")
	// Cache counter/tag instances to avoid interior hashmap lookups. If
	// SimpleCounter ever uses relaxed atomics this will also be improved
	// over a simple Inc("tag") on ARM, as Inc implies a mutex cycle.
	s.tlmMetricTypeGauge = s.tlmMetricTypes.WithValues("gauge")
	s.tlmMetricTypeCounter = s.tlmMetricTypes.WithValues("counter")
	s.tlmMetricTypeDistribution = s.tlmMetricTypes.WithValues("distribution")
	s.tlmMetricTypeHistogram = s.tlmMetricTypes.WithValues("histogram")
	s.tlmMetricTypeSet = s.tlmMetricTypes.WithValues("set")
	s.tlmMetricTypeTiming = s.tlmMetricTypes.WithValues("timing")

	s.listernersTelemetry = listeners.NewTelemetryStore(getBuckets(cfg, log, "telemetry.dogstatsd.listeners_latency_buckets"), telemetrycomp)
	s.packetsTelemetry = packets.NewTelemetryStore(getBuckets(cfg, log, "telemetry.dogstatsd.listeners_channel_latency_buckets"), telemetrycomp)

	return s
}

func (s *dsdServer) startHook(context context.Context) error {
	err := s.start(context)
	if err != nil {
		s.log.Errorf("Could not start dogstatsd: %s", err)
	} else {
		s.log.Debug("dogstatsd started")
	}
	return nil
}

func (s *dsdServer) start(context.Context) error {
	statsdForwardEnabled := s.config.GetString("statsd_forward_host") != "" && s.config.GetInt("statsd_forward_port") != 0
	pipeName := s.config.GetString("dogstatsd_pipe_name")
	socketPath := s.config.GetString("dogstatsd_socket")
	socketStreamPath := s.config.GetString("dogstatsd_stream_socket")
	originDetection := s.config.GetBool("dogstatsd_origin_detection")
	udpEnabled := s.config.GetString("dogstatsd_port") == listeners.RandomPortName || s.config.GetInt("dogstatsd_port") > 0
	rawIngressEligible := !statsdForwardEnabled && pipeName == "" && len(socketPath) > 0 && socketStreamPath == "" && !originDetection && !udpEnabled
	directCompactRawUDSIngressRingEnabled := experimentalDirectCompactRawUDSIngressRingEnabled() && rawIngressEligible
	compactRawUDSIngressRingEnabled := experimentalCompactRawUDSIngressRingEnabled() && rawIngressEligible && !directCompactRawUDSIngressRingEnabled
	rawUDSIngressRingEnabled := experimentalRawUDSIngressRingEnabled() && rawIngressEligible && !compactRawUDSIngressRingEnabled && !directCompactRawUDSIngressRingEnabled
	rawIngressEnabled := rawUDSIngressRingEnabled || compactRawUDSIngressRingEnabled || directCompactRawUDSIngressRingEnabled
	s.rawIngressBatchDrain = rawIngressEnabled && experimentalRawUDSIngressBatchDrainEnabled()
	s.rawIngressBatchDrainSize = experimentalRawUDSIngressBatchDrainSize()
	shardedIngressLogEnabled := experimentalShardedIngressLogEnabled() && !statsdForwardEnabled && pipeName == "" && !rawIngressEnabled
	ingressLogEnabled := experimentalIngressLogEnabled() && !statsdForwardEnabled && !shardedIngressLogEnabled && !rawIngressEnabled
	packetsChannelSize := s.config.GetInt("dogstatsd_queue_size")
	if ingressLogEnabled {
		// The experimental ingress log replaces the large packetsIn channel as
		// the overload absorber. Keep the listener-to-log channel tiny so
		// backpressure is controlled by the byte-bounded log, not by a second
		// implicit packet reservoir.
		packetsChannelSize = 1
	}
	packetsChannel := make(chan packets.Packets, packetsChannelSize)
	packetWriter := packets.NewChannelBatchWriter(packetsChannel)
	var rawPacketWriter packets.RawPacketWriter
	if directCompactRawUDSIngressRingEnabled {
		packetsChannel = nil
		s.workersCount = s.getDogStatsDWorkersCount()
		s.compactRawIngressShards = packets.NewDirectCompactRawIngressShards(s.workersCount, experimentalIngressLogMaxBytes(), s.config.GetInt("dogstatsd_buffer_size"), s.telemetry)
		rawPacketWriter = s.compactRawIngressShards
		s.log.Infof("DogStatsD experimental direct compact raw UDS ingress ring enabled with max_bytes=%d shards=%d", experimentalIngressLogMaxBytes(), s.workersCount)
	} else if compactRawUDSIngressRingEnabled {
		packetsChannel = nil
		s.workersCount = s.getDogStatsDWorkersCount()
		s.compactRawIngressShards = packets.NewCompactRawIngressShards(s.workersCount, experimentalIngressLogMaxBytes(), s.config.GetInt("dogstatsd_buffer_size"), s.telemetry)
		rawPacketWriter = s.compactRawIngressShards
		s.log.Infof("DogStatsD experimental compact raw UDS ingress ring enabled with max_bytes=%d shards=%d", experimentalIngressLogMaxBytes(), s.workersCount)
	} else if rawUDSIngressRingEnabled {
		packetsChannel = nil
		s.workersCount = s.getDogStatsDWorkersCount()
		s.rawIngressShards = packets.NewRawIngressShards(s.workersCount, experimentalIngressLogMaxBytes(), s.config.GetInt("dogstatsd_buffer_size"), s.telemetry)
		rawPacketWriter = s.rawIngressShards
		s.log.Infof("DogStatsD experimental raw UDS ingress ring enabled with max_bytes=%d shards=%d", experimentalIngressLogMaxBytes(), s.workersCount)
	} else if shardedIngressLogEnabled {
		packetsChannel = nil
		s.workersCount = s.getDogStatsDWorkersCount()
		s.ingressLogShards = newPacketIngressLogShards(s.workersCount, experimentalIngressLogMaxBytes(), s.telemetry)
		packetWriter = s.ingressLogShards
		s.log.Infof("DogStatsD experimental sharded ingress log enabled with max_bytes=%d shards=%d", experimentalIngressLogMaxBytes(), s.workersCount)
	} else if experimentalShardedIngressLogEnabled() && statsdForwardEnabled {
		s.log.Warn("DogStatsD experimental sharded ingress log disabled because statsd packet forwarding is enabled")
	} else if experimentalShardedIngressLogEnabled() && pipeName != "" {
		s.log.Warn("DogStatsD experimental sharded ingress log disabled because named-pipe intake is enabled")
	}
	if experimentalRawUDSIngressRingEnabled() && !rawUDSIngressRingEnabled && !compactRawUDSIngressRingEnabled && !directCompactRawUDSIngressRingEnabled {
		s.log.Warn("DogStatsD experimental raw UDS ingress ring disabled because it currently requires UDS datagram only, no origin detection, no forwarding, no UDP, no stream socket, and no named pipe")
	}
	if experimentalCompactRawUDSIngressRingEnabled() && !compactRawUDSIngressRingEnabled && !directCompactRawUDSIngressRingEnabled {
		s.log.Warn("DogStatsD experimental compact raw UDS ingress ring disabled because it currently requires UDS datagram only, no origin detection, no forwarding, no UDP, no stream socket, and no named pipe")
	}
	if experimentalDirectCompactRawUDSIngressRingEnabled() && !directCompactRawUDSIngressRingEnabled {
		s.log.Warn("DogStatsD experimental direct compact raw UDS ingress ring disabled because it currently requires UDS datagram only, no origin detection, no forwarding, no UDP, no stream socket, and no named pipe")
	}
	if experimentalRawUDSIngressBatchDrainEnabled() && !rawIngressEnabled {
		s.log.Warn("DogStatsD experimental raw UDS ingress batch drain disabled because raw UDS ingress ring is not enabled")
	}
	if s.rawIngressBatchDrain {
		s.log.Infof("DogStatsD experimental raw UDS ingress batch drain enabled with batch_size=%d", s.rawIngressBatchDrainSize)
	}
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	if err := s.tCapture.GetStartUpError(); err != nil {
		return err
	}

	// sharedPacketPool is used by the packet assembler to retrieve already allocated
	// buffer in order to avoid allocation. The packets are pushed back by the server.
	sharedPacketPool := packets.NewPool(s.config, s.config.GetInt("dogstatsd_buffer_size"), s.packetsTelemetry)
	sharedPacketPoolManager := packets.NewPoolManager[packets.Packet](sharedPacketPool)

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
		var unixListener *listeners.UDSDatagramListener
		var err error
		if rawIngressEnabled {
			unixListener, err = listeners.NewUDSDatagramListenerWithRawPacketWriter(rawPacketWriter, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		} else {
			unixListener, err = listeners.NewUDSDatagramListenerWithWriter(packetWriter, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		}
		if err != nil {
			s.log.Errorf("Can't init UDS listener on path %s: %s", socketPath, err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}

	if len(socketStreamPath) > 0 {
		s.log.Warnf("dogstatsd_stream_socket is not yet supported, run it at your own risk")
		unixListener, err := listeners.NewUDSStreamListenerWithWriter(packetWriter, sharedPacketPoolManager, sharedUDSOobPoolManager, s.config, s.tCapture, s.wmeta, s.pidMap, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		if err != nil {
			s.log.Errorf("Can't init listener: %s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}

	if udpEnabled {
		udpListener, err := listeners.NewUDPListenerWithWriter(packetWriter, sharedPacketPoolManager, s.config, s.tCapture, s.listernersTelemetry, s.packetsTelemetry)
		if err != nil {
			s.log.Errorf("%s", err.Error())
		} else {
			tmpListeners = append(tmpListeners, udpListener)
			s.udpLocalAddr = udpListener.LocalAddr()
		}
	}

	if len(pipeName) > 0 {
		namedPipeListener, err := listeners.NewNamedPipeListener(pipeName, packetsChannel, sharedPacketPoolManager, s.config, s.tCapture, s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
		if err != nil {
			s.log.Errorf("named pipe error: %v", err.Error())
		} else {
			tmpListeners = append(tmpListeners, namedPipeListener)
		}
	}

	if len(tmpListeners) == 0 {
		return errors.New("listening on neither udp nor socket, please check your configuration")
	}

	workerPacketsChannel := packetsChannel
	if rawIngressEnabled {
		workerPacketsChannel = nil
	} else if shardedIngressLogEnabled {
		workerPacketsChannel = nil
	} else if ingressLogEnabled {
		workerPacketsChannel = make(chan packets.Packets)
		ingressLog := newPacketIngressLog(experimentalIngressLogMaxBytes(), s.telemetry)
		go ingressLog.run(packetsChannel, workerPacketsChannel, s.stopChan)
		s.log.Infof("DogStatsD experimental ingress log enabled with max_bytes=%d", experimentalIngressLogMaxBytes())
	} else if experimentalIngressLogEnabled() && statsdForwardEnabled {
		s.log.Warn("DogStatsD experimental ingress log disabled because statsd packet forwarding is enabled")
	}

	s.packetsIn = workerPacketsChannel
	s.captureChan = packetsChannel
	s.sharedPacketPool = sharedPacketPool
	s.sharedPacketPoolManager = sharedPacketPoolManager
	s.listeners = tmpListeners

	// packets forwarding
	// ----------------------

	forwardHost := s.config.GetString("statsd_forward_host")
	forwardPort := s.config.GetInt("statsd_forward_port")
	if forwardHost != "" && forwardPort != 0 {
		forwardAddress := net.JoinHostPort(forwardHost, strconv.Itoa(forwardPort))
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

func (s *dsdServer) stop(context.Context) error {
	s.startedMtx.Lock()
	defer s.startedMtx.Unlock()

	if !s.IsRunning() {
		return nil
	}

	if s.ingressLogShards != nil {
		s.ingressLogShards.stop()
	}
	if s.rawIngressShards != nil {
		s.rawIngressShards.Stop()
	}
	if s.compactRawIngressShards != nil {
		s.compactRawIngressShards.Stop()
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

func (s *dsdServer) IsRunning() bool {
	return s.Started
}

func (s *dsdServer) onFilterListUpdate(filterList utilstrings.Matcher, _ utilstrings.Matcher) {
	s.startedMtx.RLock()
	defer s.startedMtx.RUnlock()

	if !s.IsRunning() {
		// The server is not running, so workers can't receive updates.
		return
	}

	// send the complete filterlist to all workers, the listening part of dogstatsd
	for _, worker := range s.workers {
		worker.FilterListUpdate <- filterList
	}
}

func (s *dsdServer) getDogStatsDWorkersCount() int {
	workersCount, _ := aggregator.GetDogStatsDWorkerAndPipelineCount()

	// undocumented configuration field to force the amount of dogstatsd workers
	// mainly used for benchmarks or some very specific use-case.
	if configWC := s.config.GetInt("dogstatsd_workers_count"); configWC != 0 {
		s.log.Debug("Forcing the amount of DogStatsD workers to:", configWC)
		workersCount = configWC
	}
	return workersCount
}

func (s *dsdServer) handleMessages() {
	if s.Statistics != nil {
		go s.Statistics.Process()
		go s.Statistics.Update(&dogstatsdPacketsLastSec)
	}

	// start the listeners

	for _, l := range s.listeners {
		l.Listen()
	}

	if s.offlineReporter != nil {
		s.offlineReporter.SendOfflineDuration("datadog.agent.dogstatsd.offline_duration_seconds", nil)
	}

	// create and start all the workers

	workersCount := s.workersCount
	if workersCount == 0 {
		workersCount = s.getDogStatsDWorkersCount()
	}

	s.log.Debug("DogStatsD will run", workersCount, "workers")

	for i := 0; i < workersCount; i++ {
		worker := newWorker(s, i, s.wmeta, s.packetsTelemetry, s.stringInternerTelemetry, s.filterList.GetMetricFilterList())
		if s.ingressLogShards != nil {
			worker.packetLog = s.ingressLogShards.shard(i)
		}
		if s.rawIngressShards != nil {
			worker.rawIngress = s.rawIngressShards.Shard(i)
		}
		if s.compactRawIngressShards != nil {
			worker.rawIngress = s.compactRawIngressShards.Shard(i)
		}
		go worker.run()
		s.workers = append(s.workers, worker)
	}

	// It is important to set this up after the workers are running so they receive
	// any updates.
	s.filterList.OnUpdateMetricFilterList(s.onFilterListUpdate)
}

func (s *dsdServer) UDPLocalAddr() string {
	return s.udpLocalAddr
}

func (s *dsdServer) forwarder(fcon net.Conn) {
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
func (s *dsdServer) ServerlessFlush(sketchesBucketDelay time.Duration) {
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

func (s *dsdServer) eolEnabled(sourceType packets.SourceType) bool {
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

func (s *dsdServer) errLog(format string, params ...interface{}) {
	if s.disableVerboseLogs {
		s.log.Debugf(format, params...)
	} else {
		s.log.Errorf(format, params...)
	}
}

type precomputedDebugStatsStore interface {
	StoreMetricStatsWithDebugViewKey(sample metrics.MetricSample, debugViewKey identity.DebugViewKey)
}

func columnarV3DirectParseEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_PARSE"))
	return err == nil && enabled
}

// workers are running this function in their goroutine
func (s *dsdServer) parsePackets(batcher dogstatsdBatcher, parser *parser, identityBuilder *identity.Builder, packets []*packets.Packet, samples metrics.MetricSampleBatch, filterList *utilstrings.Matcher) metrics.MetricSampleBatch {
	if identityBuilder == nil {
		identityBuilder = identity.NewBuilder()
	}

	var columnarV3 aggregator.DogStatsDColumnarV3Inserter
	columnarV3Enabled := false
	if inserter, ok := s.demultiplexer.(aggregator.DogStatsDColumnarV3Inserter); ok && inserter.DogStatsDColumnarV3Enabled() {
		columnarV3 = inserter
		columnarV3Enabled = true
	}

	for _, packet := range packets {
		samples = s.parsePacket(batcher, parser, identityBuilder, packet, samples, filterList, columnarV3, columnarV3Enabled)
		s.sharedPacketPoolManager.Put(packet)
	}

	batcher.flush()
	return samples
}

func (s *dsdServer) parsePacket(batcher dogstatsdBatcher, parser *parser, identityBuilder *identity.Builder, packet *packets.Packet, samples metrics.MetricSampleBatch, filterList *utilstrings.Matcher, columnarV3 aggregator.DogStatsDColumnarV3Inserter, columnarV3Enabled bool) metrics.MetricSampleBatch {
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
			var handled bool

			samples = samples[0:0]

			if columnarV3Enabled && s.columnarV3DirectParse {
				samples, handled, err = s.parseMetricMessageColumnarV3Direct(batcher, parser, identityBuilder, message, packet.Origin, packet.ProcessID, packet.ListenerID, s.originTelemetry, filterList, samples, columnarV3)
				if err != nil {
					s.errLog("Dogstatsd: error parsing metric message '%q': %s", message, err)
					continue
				}
				if handled {
					continue
				}
			}

			samples, err = s.parseMetricMessage(samples, parser, message, packet.Origin, packet.ProcessID, packet.ListenerID, s.originTelemetry, filterList)
			if err != nil {
				s.errLog("Dogstatsd: error parsing metric message '%q': %s", message, err)
				continue
			}
			s.appendMetricSamples(batcher, identityBuilder, samples, columnarV3, columnarV3Enabled)
		}
	}

	return samples
}

func (s *dsdServer) appendMetricSamples(batcher dogstatsdBatcher, identityBuilder *identity.Builder, samples []metrics.MetricSample, columnarV3 aggregator.DogStatsDColumnarV3Inserter, columnarV3Enabled bool) {
	batcherNeedsContext := batcher.needsSampleContext()
	for idx := range samples {
		debugEnabled := s.Debug.IsDebugEnabled()
		needsShardContext := batcherNeedsContext || columnarV3Enabled
		var sampleContext identity.HotPathContext
		if debugEnabled {
			sampleContext = identityBuilder.ResolveHotPath(samples[idx])
		} else if needsShardContext {
			sampleContext = identityBuilder.ResolveShardHotPath(samples[idx])
		}

		if debugEnabled {
			s.storeMetricStats(samples[idx], sampleContext)
		} else {
			// Preserve the legacy runtime-setting race behavior: if debug is
			// enabled after the cheap IsDebugEnabled check, StoreMetricStats
			// can still record this sample using its legacy local key path.
			s.Debug.StoreMetricStats(samples[idx])
		}

		if columnarV3Enabled && columnarV3.AcceptDogStatsDColumnarV3Sample(samples[idx]) {
			// The experimental v3 columnar table is now the authoritative
			// aggregation state for this supported sample. Unsupported
			// samples return false and continue through the legacy batcher.
			batcher.appendColumnarV3SampleWithContext(samples[idx], sampleContext)
		} else if samples[idx].Timestamp > 0.0 {
			if needsShardContext {
				batcher.appendLateSampleWithContext(samples[idx], sampleContext)
			} else {
				batcher.appendLateSample(samples[idx])
			}
		} else if needsShardContext {
			batcher.appendSampleWithContext(samples[idx], sampleContext)
		} else {
			batcher.appendSample(samples[idx])
		}

		if s.histToDist && samples[idx].Mtype == metrics.HistogramType {
			distSample := samples[idx].Copy()
			distSample.Name = s.histToDistPrefix + distSample.Name
			distSample.Mtype = metrics.DistributionType
			if batcherNeedsContext {
				distContext := identityBuilder.ResolveHotPath(*distSample)
				batcher.appendSampleWithContext(*distSample, distContext)
			} else {
				batcher.appendSample(*distSample)
			}
		}
	}
}

func (s *dsdServer) storeMetricStats(sample metrics.MetricSample, sampleContext identity.HotPathContext) {
	if debugStore, ok := s.Debug.(precomputedDebugStatsStore); ok {
		debugStore.StoreMetricStatsWithDebugViewKey(sample, sampleContext.DebugView)
		return
	}
	s.Debug.StoreMetricStats(sample)
}

// getOriginCounter returns a telemetry counter for processed metrics using the given origin as a tag.
// They are stored in cache to avoid heap escape.
// Only `maxOriginCounters` are stored to avoid an infinite expansion.
// Counters returned by `getOriginCounter` are thread safe.
func (s *dsdServer) getOriginCounter(origin string) (okCnt telemetry.SimpleCounter, errorCnt telemetry.SimpleCounter) {
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
func (s *dsdServer) parseMetricMessage(metricSamples []metrics.MetricSample, parser *parser, message []byte, origin string,
	processID uint32, listenerID string, originTelemetry bool, filterList *utilstrings.Matcher) ([]metrics.MetricSample, error) {
	okCnt, errorCnt := s.metricTelemetryCounters(origin, originTelemetry)

	sample, err := parser.parseMetricSample(message)
	if err != nil {
		dogstatsdMetricParseErrors.Add(1)
		errorCnt.Inc()
		return metricSamples, err
	}
	s.recordMetricTypeTelemetry(sample.metricType)
	return s.finishParsedMetricMessage(metricSamples, sample, origin, processID, listenerID, filterList, okCnt), nil
}

func (s *dsdServer) metricTelemetryCounters(origin string, originTelemetry bool) (telemetry.SimpleCounter, telemetry.SimpleCounter) {
	okCnt := s.tlmProcessedOk
	errorCnt := s.tlmProcessedError
	if origin != "" && originTelemetry {
		okCnt, errorCnt = s.getOriginCounter(origin)
	}
	return okCnt, errorCnt
}

func (s *dsdServer) recordMetricTypeTelemetry(metricType metricType) {
	switch metricType {
	case gaugeType:
		s.tlmMetricTypeGauge.Inc()
	case countType:
		s.tlmMetricTypeCounter.Inc()
	case distributionType:
		s.tlmMetricTypeDistribution.Inc()
	case histogramType:
		s.tlmMetricTypeHistogram.Inc()
	case setType:
		s.tlmMetricTypeSet.Inc()
	case timingType:
		s.tlmMetricTypeTiming.Inc()
	}
}

func columnarV3DirectMetricTypeSupported(mtype metrics.MetricType) bool {
	switch mtype {
	case metrics.GaugeType, metrics.CounterType, metrics.CountType, metrics.SetType:
		return true
	default:
		return false
	}
}

func (s *dsdServer) parseMetricMessageColumnarV3Direct(batcher dogstatsdBatcher, parser *parser, identityBuilder *identity.Builder, message []byte, origin string,
	processID uint32, listenerID string, originTelemetry bool, filterList *utilstrings.Matcher, metricSamples []metrics.MetricSample, columnarV3 aggregator.DogStatsDColumnarV3Inserter) ([]metrics.MetricSample, bool, error) {
	// Keep this vertical slice intentionally narrow. If compatibility features that
	// rewrite identities are active, the normal MetricSample path remains the
	// source of truth.
	if s.mapper != nil || len(s.extraTags) > 0 || s.histToDist || s.Debug.IsDebugEnabled() {
		return metricSamples, false, nil
	}

	okCnt, errorCnt := s.metricTelemetryCounters(origin, originTelemetry)
	sample, err := parser.parseMetricSample(message)
	if err != nil {
		dogstatsdMetricParseErrors.Add(1)
		errorCnt.Inc()
		return metricSamples, true, err
	}
	s.recordMetricTypeTelemetry(sample.metricType)

	metricName := sample.name
	if !isExcluded(metricName, s.enrichConfig.metricPrefix, s.enrichConfig.metricPrefixBlacklist) {
		metricName = s.enrichConfig.metricPrefix + metricName
	}
	if filterList != nil && filterList.Test(metricName) {
		tlmFilteredPoints.Inc()
		if len(sample.values) > 0 {
			s.sharedFloat64List.put(sample.values)
		}
		return metricSamples, true, nil
	}

	tags, hostnameFromTags, extractedOrigin, metricSource := extractTagsMetadata(sample.tags, origin, processID, sample.localData, sample.externalData, sample.cardinality, s.enrichConfig)
	if s.enrichConfig.serverlessMode {
		hostnameFromTags = ""
		if strings.HasPrefix(metricName, "runtime.") {
			metricSource = serverlessSourceCustomToRuntime(metricSource)
		}
	}

	mtype := enrichMetricType(sample.metricType)
	unit := unitFromMetricType(sample.metricType)
	timestamp := tsToFloatForSamples(sample.ts)
	template := metrics.MetricSample{
		Host:              hostnameFromTags,
		Name:              metricName,
		Tags:              tags,
		Mtype:             mtype,
		SampleRate:        sample.sampleRate,
		RawValue:          sample.setValue,
		Timestamp:         timestamp,
		OriginInfo:        extractedOrigin,
		ListenerID:        listenerID,
		Source:            metricSource,
		Unit:              unit,
		DogStatsDTagsetID: sample.tagsetID,
	}

	acceptedDirect := func(value float64) bool {
		return timestamp == 0 && !math.IsInf(value, 0) && !math.IsNaN(value) && columnarV3DirectMetricTypeSupported(mtype)
	}
	appendDirectAccepted := func(value float64) {
		template.Value = value
		context := identityBuilder.ResolveShardHotPath(template)
		batcher.appendColumnarV3SampleWithContext(template, context)
		dogstatsdMetricPackets.Add(1)
		okCnt.Inc()
	}

	finishLegacy := func() []metrics.MetricSample {
		metricSamples = s.finishParsedMetricMessage(metricSamples, sample, origin, processID, listenerID, filterList, okCnt)
		s.appendMetricSamples(batcher, identityBuilder, metricSamples, columnarV3, true)
		return metricSamples
	}

	if len(sample.values) > 0 {
		for _, value := range sample.values {
			if !acceptedDirect(value) {
				return finishLegacy(), true, nil
			}
		}
		for _, value := range sample.values {
			appendDirectAccepted(value)
		}
		s.sharedFloat64List.put(sample.values)
		return metricSamples, true, nil
	}

	if acceptedDirect(sample.value) {
		appendDirectAccepted(sample.value)
	} else {
		return finishLegacy(), true, nil
	}
	return metricSamples, true, nil
}

func (s *dsdServer) finishParsedMetricMessage(metricSamples []metrics.MetricSample, sample dogstatsdMetricSample, origin string,
	processID uint32, listenerID string, filterList *utilstrings.Matcher, okCnt telemetry.SimpleCounter) []metrics.MetricSample {
	if s.mapper != nil {
		mapResult := s.mapper.Map(sample.name)
		if mapResult != nil {
			s.log.Tracef("Dogstatsd mapper: metric mapped from %q to %q with tags %v", sample.name, mapResult.Name, mapResult.Tags)
			sample.name = mapResult.Name
			if len(mapResult.Tags) > 0 {
				sample.tagsetID = 0
			}
			sample.tags = append(sample.tags, mapResult.Tags...)
		}
	}

	metricSamples = enrichMetricSample(metricSamples, sample, origin, processID, listenerID, s.enrichConfig, filterList)

	if len(sample.values) > 0 {
		s.sharedFloat64List.put(sample.values)
	}

	for idx := range metricSamples {
		// All metricSamples already share the same Tags slice. We can
		// extends the first one and reuse it for the rest.
		if idx == 0 {
			metricSamples[idx].Tags = append(metricSamples[idx].Tags, s.extraTags...)
			if len(s.extraTags) > 0 {
				metricSamples[idx].DogStatsDTagsetID = 0
			}
		} else {
			metricSamples[idx].Tags = metricSamples[0].Tags
			metricSamples[idx].DogStatsDTagsetID = metricSamples[0].DogStatsDTagsetID
		}

		// If we're receiving runtime metrics, we need to convert the default source to the runtime source
		if s.enrichConfig.serverlessMode && strings.HasPrefix(metricSamples[idx].Name, "runtime.") {
			metricSamples[idx].Source = serverlessSourceCustomToRuntime(metricSamples[idx].Source)
		}

		dogstatsdMetricPackets.Add(1)
		okCnt.Inc()
	}
	return metricSamples
}

func (s *dsdServer) parseEventMessage(parser *parser, message []byte, origin string, processID uint32) (*event.Event, error) {
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

func (s *dsdServer) parseServiceCheckMessage(parser *parser, message []byte, origin string, processID uint32) (*servicecheck.ServiceCheck, error) {
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
