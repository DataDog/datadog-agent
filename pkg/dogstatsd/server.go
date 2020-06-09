// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package dogstatsd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	dogstatsdExpvars                 = expvar.NewMap("dogstatsd")
	dogstatsdServiceCheckParseErrors = expvar.Int{}
	dogstatsdServiceCheckPackets     = expvar.Int{}
	dogstatsdEventParseErrors        = expvar.Int{}
	dogstatsdEventPackets            = expvar.Int{}
	dogstatsdMetricParseErrors       = expvar.Int{}
	dogstatsdMetricPackets           = expvar.Int{}
	dogstatsdPacketsLastSec          = expvar.Int{}

	tlmProcessed = telemetry.NewCounter("dogstatsd", "processed",
		[]string{"message_type", "state"}, "Count of service checks/events/metrics processed by dogstatsd")
	tlmProcessedErrorTags = map[string]string{"message_type": "metrics", "state": "error"}
	tlmProcessedOkTags    = map[string]string{"message_type": "metrics", "state": "ok"}
)

func init() {
	dogstatsdExpvars.Set("ServiceCheckParseErrors", &dogstatsdServiceCheckParseErrors)
	dogstatsdExpvars.Set("ServiceCheckPackets", &dogstatsdServiceCheckPackets)
	dogstatsdExpvars.Set("EventParseErrors", &dogstatsdEventParseErrors)
	dogstatsdExpvars.Set("EventPackets", &dogstatsdEventPackets)
	dogstatsdExpvars.Set("MetricParseErrors", &dogstatsdMetricParseErrors)
	dogstatsdExpvars.Set("MetricPackets", &dogstatsdMetricPackets)
}

// Server represent a Dogstatsd server
type Server struct {
	// listeners are the instantiated socket listener (UDS or UDP or both)
	listeners []listeners.StatsdListener
	// aggregator is a pointer to the aggregator that the dogstatsd daemon
	// will send the metrics samples, events and service checks to.
	aggregator *aggregator.BufferedAggregator

	packetsIn                 chan listeners.Packets
	sharedPacketPool          *listeners.PacketPool
	Statistics                *util.Stats
	Started                   bool
	stopChan                  chan bool
	health                    *health.Handle
	metricPrefix              string
	metricPrefixBlacklist     []string
	defaultHostname           string
	histToDist                bool
	histToDistPrefix          string
	extraTags                 []string
	Debug                     dsdServerDebug
	mapper                    *mapper.MetricMapper
	telemetryEnabled          bool
	entityIDPrecedenceEnabled bool
	// disableVerboseLogs is a feature flag to disable the logs capable
	// of flooding the logger output (e.g. parsing messages error).
	// NOTE(remy): this should probably be dropped and use a throttler logger, see
	// package (pkg/trace/logutils) for a possible throttler implemetation.
	disableVerboseLogs bool
}

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Name     string    `json:"name"`
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Tags     string    `json:"tags"`
}

type dsdServerDebug struct {
	sync.Mutex
	// Enabled is an atomic int used as a boolean
	Enabled uint64                         `json:"enabled"`
	Stats   map[ckey.ContextKey]metricStat `json:"stats"`
	// counting number of metrics processed last X seconds
	metricsCounts metricsCountBuckets
	// keyGen is used to generate hashes of the metrics received by dogstatsd
	keyGen *ckey.KeyGenerator
}

// metricsCountBuckets is counting the amount of metrics received for the last 5 seconds.
// It is used to detect spikes.
type metricsCountBuckets struct {
	counts     [5]uint64
	bucketIdx  int
	currentSec time.Time
	metricChan chan struct{}
	closeChan  chan struct{}
}

// NewServer returns a running Dogstatsd server
func NewServer(aggregator *aggregator.BufferedAggregator) (*Server, error) {
	var stats *util.Stats
	if config.Datadog.GetBool("dogstatsd_stats_enable") == true {
		buff := config.Datadog.GetInt("dogstatsd_stats_buffer")
		s, err := util.NewStats(uint32(buff))
		if err != nil {
			log.Errorf("Dogstatsd: unable to start statistics facilities")
		}
		stats = s
		dogstatsdExpvars.Set("PacketsLastSecond", &dogstatsdPacketsLastSec)
	}

	var metricsStatsEnabled uint64 // we're using an uint64 for its atomic capacity
	if config.Datadog.GetBool("dogstatsd_metrics_stats_enable") == true {
		log.Info("Dogstatsd: metrics statistics will be stored.")
		metricsStatsEnabled = 1
	}

	packetsChannel := make(chan listeners.Packets, config.Datadog.GetInt("dogstatsd_queue_size"))
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	// sharedPacketPool is used by the packet assembler to retrieve already allocated
	// buffer in order to avoid allocation. The packets are pushed back by the server.
	sharedPacketPool := listeners.NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUDSListener(packetsChannel, sharedPacketPool)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}
	if config.Datadog.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetsChannel, sharedPacketPool)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, udpListener)
		}
	}

	if len(tmpListeners) == 0 {
		return nil, fmt.Errorf("listening on neither udp nor socket, please check your configuration")
	}

	// check configuration for custom namespace
	metricPrefix := config.Datadog.GetString("statsd_metric_namespace")
	if metricPrefix != "" && !strings.HasSuffix(metricPrefix, ".") {
		metricPrefix = metricPrefix + "."
	}
	metricPrefixBlacklist := config.Datadog.GetStringSlice("statsd_metric_namespace_blacklist")

	defaultHostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Dogstatsd: unable to determine default hostname: %s", err.Error())
	}

	histToDist := config.Datadog.GetBool("histogram_copy_to_distribution")
	histToDistPrefix := config.Datadog.GetString("histogram_copy_to_distribution_prefix")

	extraTags := config.Datadog.GetStringSlice("dogstatsd_tags")

	entityIDPrecedenceEnabled := config.Datadog.GetBool("dogstatsd_entity_id_precedence")

	s := &Server{
		Started:                   true,
		Statistics:                stats,
		packetsIn:                 packetsChannel,
		sharedPacketPool:          sharedPacketPool,
		aggregator:                aggregator,
		listeners:                 tmpListeners,
		stopChan:                  make(chan bool),
		health:                    health.RegisterLiveness("dogstatsd-main"),
		metricPrefix:              metricPrefix,
		metricPrefixBlacklist:     metricPrefixBlacklist,
		defaultHostname:           defaultHostname,
		histToDist:                histToDist,
		histToDistPrefix:          histToDistPrefix,
		extraTags:                 extraTags,
		telemetryEnabled:          telemetry.IsEnabled(),
		entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
		disableVerboseLogs:        config.Datadog.GetBool("dogstatsd_disable_verbose_logs"),
		Debug: dsdServerDebug{
			Enabled: metricsStatsEnabled,
			Stats:   make(map[ckey.ContextKey]metricStat),
			metricsCounts: metricsCountBuckets{
				counts:     [5]uint64{0, 0, 0, 0, 0},
				metricChan: make(chan struct{}),
				closeChan:  make(chan struct{}),
			},
			keyGen: ckey.NewKeyGenerator(),
		},
	}

	// packets forwarding
	// ----------------------

	forwardHost := config.Datadog.GetString("statsd_forward_host")
	forwardPort := config.Datadog.GetInt("statsd_forward_port")
	if forwardHost != "" && forwardPort != 0 {
		forwardAddress := fmt.Sprintf("%s:%d", forwardHost, forwardPort)
		con, err := net.Dial("udp", forwardAddress)
		if err != nil {
			log.Warnf("Could not connect to statsd forward host : %s", err)
		} else {
			s.packetsIn = make(chan listeners.Packets, config.Datadog.GetInt("dogstatsd_queue_size"))
			go s.forwarder(con, packetsChannel)
		}
	}

	// start the workers processing the packets read on the socket
	// ----------------------

	s.handleMessages()

	// start the debug loop
	// ----------------------

	if metricsStatsEnabled == 1 {
		s.EnableMetricsStats()
	}

	// map some metric name
	// ----------------------

	cacheSize := config.Datadog.GetInt("dogstatsd_mapper_cache_size")

	mappings, err := config.GetDogstatsdMappingProfiles()
	if err != nil {
		log.Warnf("Could not parse mapping profiles: %v", err)
	} else if len(mappings) != 0 {
		mapperInstance, err := mapper.NewMetricMapper(mappings, cacheSize)
		if err != nil {
			log.Warnf("Could not create metric mapper: %v", err)
		} else {
			s.mapper = mapperInstance
		}
	}
	return s, nil
}

func (s *Server) handleMessages() {
	if s.Statistics != nil {
		go s.Statistics.Process()
		go s.Statistics.Update(&dogstatsdPacketsLastSec)
	}

	for _, l := range s.listeners {
		go l.Listen()
	}

	// Run min(2, GoMaxProcs-2) workers, we dedicate a core to the
	// listener goroutine and another to aggregator + forwarder
	workers := runtime.GOMAXPROCS(-1) - 2
	if workers < 2 {
		workers = 2
	}

	for i := 0; i < workers; i++ {
		go s.worker()
	}
}

func (s *Server) forwarder(fcon net.Conn, packetsChannel chan listeners.Packets) {
	for {
		select {
		case <-s.stopChan:
			return
		case packets := <-packetsChannel:
			for _, packet := range packets {
				_, err := fcon.Write(packet.Contents)

				if err != nil {
					log.Warnf("Forwarding packet failed : %s", err)
				}
			}
			s.packetsIn <- packets
		}
	}
}

func (s *Server) worker() {
	// the batcher will be responsible of batching a few samples / events / service
	// checks and it will automatically forward them to the aggregator, meaning that
	// the flushing logic to the aggregator is actually in the batcher.
	batcher := newBatcher(s.aggregator)

	parser := newParser()
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.health.C:
		case packets := <-s.packetsIn:
			s.parsePackets(batcher, parser, packets)
		}
	}
}

func nextMessage(packet *[]byte) (message []byte) {
	if len(*packet) == 0 {
		return nil
	}

	advance, message, err := bufio.ScanLines(*packet, true)
	if err != nil {
		return nil
	}

	*packet = (*packet)[advance:]
	return message
}

func (s *Server) parsePackets(batcher *batcher, parser *parser, packets []*listeners.Packet) {
	for _, packet := range packets {
		originTagger := originTags{origin: packet.Origin}
		log.Tracef("Dogstatsd receive: %q", packet.Contents)
		for {
			message := nextMessage(&packet.Contents)
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
				serviceCheck, err := s.parseServiceCheckMessage(parser, message, originTagger.getTags)
				if err != nil {
					originTags := originTagger.getTags()
					if len(originTags) > 0 {
						s.errLog("Dogstatsd: error parsing service check '%q' origin tags %v: %s", message, originTags, err)
					} else {
						s.errLog("Dogstatsd: error parsing service check '%q': %s", message, err)
					}
					continue
				}
				batcher.appendServiceCheck(serviceCheck)
			case eventType:
				event, err := s.parseEventMessage(parser, message, originTagger.getTags)
				if err != nil {
					originTags := originTagger.getTags()
					if len(originTags) > 0 {
						s.errLog("Dogstatsd: error parsing event '%q' origin tags %v: %s", message, originTags, err)
					} else {
						s.errLog("Dogstatsd: error parsing event '%q': %s", message, err)
					}
					continue
				}
				batcher.appendEvent(event)
			case metricSampleType:
				sample, err := s.parseMetricMessage(parser, message, originTagger.getTags)
				if err != nil {
					originTags := originTagger.getTags()
					if len(originTags) > 0 {
						s.errLog("Dogstatsd: error parsing metric message '%q' origin tags %v: %s", message, originTags, err)
					} else {
						s.errLog("Dogstatsd: error parsing metric message '%q': %s", message, err)
					}
					continue
				}
				if atomic.LoadUint64(&s.Debug.Enabled) == 1 {
					s.storeMetricStats(sample)
				}
				batcher.appendSample(sample)
				if s.histToDist && sample.Mtype == metrics.HistogramType {
					distSample := sample.Copy()
					distSample.Name = s.histToDistPrefix + distSample.Name
					distSample.Mtype = metrics.DistributionType
					batcher.appendSample(*distSample)
				}
			}
		}
		s.sharedPacketPool.Put(packet)
	}
	batcher.flush()
}

func (s *Server) errLog(format string, params ...interface{}) {
	if s.disableVerboseLogs {
		log.Debugf(format, params...)
	} else {
		log.Errorf(format, params...)
	}
}

func (s *Server) parseMetricMessage(parser *parser, message []byte, originTagsFunc func() []string) (metrics.MetricSample, error) {
	sample, err := parser.parseMetricSample(message)
	if err != nil {
		dogstatsdMetricParseErrors.Add(1)
		tlmProcessed.IncWithTags(tlmProcessedErrorTags)
		return metrics.MetricSample{}, err
	}
	if s.mapper != nil && len(sample.tags) == 0 {
		mapResult := s.mapper.Map(sample.name)
		if mapResult != nil {
			sample.name = mapResult.Name
			sample.tags = append(sample.tags, mapResult.Tags...)
		}
	}
	metricSample := enrichMetricSample(sample, s.metricPrefix, s.metricPrefixBlacklist, s.defaultHostname, originTagsFunc, s.entityIDPrecedenceEnabled)
	metricSample.Tags = append(metricSample.Tags, s.extraTags...)
	dogstatsdMetricPackets.Add(1)
	tlmProcessed.IncWithTags(tlmProcessedOkTags)
	return metricSample, nil
}

func (s *Server) parseEventMessage(parser *parser, message []byte, originTagsFunc func() []string) (*metrics.Event, error) {
	sample, err := parser.parseEvent(message)
	if err != nil {
		dogstatsdEventParseErrors.Add(1)
		tlmProcessed.Inc("events", "error")
		return nil, err
	}
	event := enrichEvent(sample, s.defaultHostname, originTagsFunc, s.entityIDPrecedenceEnabled)
	event.Tags = append(event.Tags, s.extraTags...)
	tlmProcessed.Inc("events", "ok")
	dogstatsdEventPackets.Add(1)
	return event, nil
}

func (s *Server) parseServiceCheckMessage(parser *parser, message []byte, originTagsFunc func() []string) (*metrics.ServiceCheck, error) {
	sample, err := parser.parseServiceCheck(message)
	if err != nil {
		dogstatsdServiceCheckParseErrors.Add(1)
		tlmProcessed.Inc("service_checks", "error")
		return nil, err
	}
	serviceCheck := enrichServiceCheck(sample, s.defaultHostname, originTagsFunc, s.entityIDPrecedenceEnabled)
	serviceCheck.Tags = append(serviceCheck.Tags, s.extraTags...)
	dogstatsdServiceCheckPackets.Add(1)
	tlmProcessed.Inc("service_checks", "ok")
	return serviceCheck, nil
}

// Stop stops a running Dogstatsd server
func (s *Server) Stop() {
	close(s.stopChan)
	for _, l := range s.listeners {
		l.Stop()
	}
	if s.Statistics != nil {
		s.Statistics.Stop()
	}
	s.health.Deregister() //nolint:errcheck
	s.Started = false
}

func (s *Server) storeMetricStats(sample metrics.MetricSample) {
	now := time.Now()
	s.Debug.Lock()
	defer s.Debug.Unlock()

	// key
	util.SortUniqInPlace(sample.Tags)
	key := s.Debug.keyGen.Generate(sample.Name, "", sample.Tags)

	// store
	ms := s.Debug.Stats[key]
	ms.Count++
	ms.LastSeen = now
	ms.Name = sample.Name
	ms.Tags = strings.Join(sample.Tags, " ") // we don't want/need to share the underlying array
	s.Debug.Stats[key] = ms

	s.Debug.metricsCounts.metricChan <- struct{}{}
}

// EnableMetricsStats enables the debug mode of the DogStatsD server and start
// the debug mainloop collecting the amount of metrics received.
func (s *Server) EnableMetricsStats() {
	s.Debug.Lock()
	defer s.Debug.Unlock()

	// already enabled?
	if atomic.LoadUint64(&s.Debug.Enabled) == 1 {
		return
	}

	atomic.StoreUint64(&s.Debug.Enabled, 1)
	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		var closed bool
		log.Debug("Starting the DogStatsD debug loop.")
		for {
			select {
			case <-ticker.C:
				sec := time.Now().Truncate(time.Second)
				if sec.After(s.Debug.metricsCounts.currentSec) {
					s.Debug.metricsCounts.currentSec = sec

					if s.hasSpike() {
						log.Warnf("A burst of metrics has been detected by DogStatSd: here is the last 5 seconds count of metrics: %v", s.Debug.metricsCounts.counts)
					}

					s.Debug.metricsCounts.bucketIdx++
					if s.Debug.metricsCounts.bucketIdx >= len(s.Debug.metricsCounts.counts) {
						s.Debug.metricsCounts.bucketIdx = 0
					}

					s.Debug.metricsCounts.counts[s.Debug.metricsCounts.bucketIdx] = 0
				}
			case <-s.Debug.metricsCounts.metricChan:
				s.Debug.metricsCounts.counts[s.Debug.metricsCounts.bucketIdx]++
			case <-s.Debug.metricsCounts.closeChan:
				closed = true
				break
			}

			if closed {
				break
			}
		}
		log.Debug("Stopping the DogStatsD debug loop.")
		ticker.Stop()
	}()
}

func (s *Server) hasSpike() bool {
	// compare this one to the sum of all others
	// if the difference is higher than all others sum, consider this
	// as an anomaly.
	var sum uint64
	for _, v := range s.Debug.metricsCounts.counts {
		sum += v
	}
	sum -= s.Debug.metricsCounts.counts[s.Debug.metricsCounts.bucketIdx]
	if s.Debug.metricsCounts.counts[s.Debug.metricsCounts.bucketIdx] > sum {
		return true
	}
	return false
}

// DisableMetricsStats disables the debug mode of the DogStatsD server and
// stops the debug mainloop.
func (s *Server) DisableMetricsStats() {
	s.Debug.Lock()
	defer s.Debug.Unlock()

	if atomic.LoadUint64(&s.Debug.Enabled) == 1 {
		atomic.StoreUint64(&s.Debug.Enabled, 0)
		s.Debug.metricsCounts.closeChan <- struct{}{}
	}

	log.Info("Disabling DogStatsD debug metrics stats.")
}

// GetJSONDebugStats returns jsonified debug statistics.
func (s *Server) GetJSONDebugStats() ([]byte, error) {
	s.Debug.Lock()
	defer s.Debug.Unlock()
	return json.Marshal(s.Debug.Stats)
}

// FormatDebugStats returns a printable version of debug stats.
func FormatDebugStats(stats []byte) (string, error) {
	var dogStats map[uint64]metricStat
	if err := json.Unmarshal(stats, &dogStats); err != nil {
		return "", err
	}

	// put metrics in order: first is the more frequent
	order := make([]uint64, len(dogStats))
	i := 0
	for metric := range dogStats {
		order[i] = metric
		i++
	}

	sort.Slice(order, func(i, j int) bool {
		return dogStats[order[i]].Count > dogStats[order[j]].Count
	})

	// write the response
	buf := bytes.NewBuffer(nil)

	header := fmt.Sprintf("%-40s | %-20s | %-10s | %-20s\n", "Metric", "Tags", "Count", "Last Seen")
	buf.Write([]byte(header))
	buf.Write([]byte(strings.Repeat("-", len(header)) + "\n"))

	for _, key := range order {
		stats := dogStats[key]
		buf.Write([]byte(fmt.Sprintf("%-40s | %-20s | %-10d | %-20v\n", stats.Name, stats.Tags, stats.Count, stats.LastSeen)))
	}

	if len(dogStats) == 0 {
		buf.Write([]byte("No metrics processed yet."))
	}

	return buf.String(), nil
}

func findOriginTags(origin string) []string {
	var tags []string
	if origin != listeners.NoOrigin {
		originTags, err := tagger.Tag(origin, tagger.DogstatsdCardinality)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tags = append(tags, originTags...)
		}
	}

	// Include orchestrator scope tags if the cardinality is set to orchestrator
	if tagger.DogstatsdCardinality == collectors.OrchestratorCardinality {
		orchestratorScopeTags, err := tagger.OrchestratorScopeTag()
		if err != nil {
			log.Error(err.Error())
		} else {
			tags = append(tags, orchestratorScopeTags...)
		}
	}
	return tags
}

type originTags struct {
	origin string
	tags   []string
	// we don't use "sync.Once" here because we know only on one goroutine can call the function `getTags()`
	alreadyRun bool
}

func (o *originTags) getTags() []string {
	if !o.alreadyRun {
		o.tags = findOriginTags(o.origin)
		o.alreadyRun = true
	}
	return o.tags
}
