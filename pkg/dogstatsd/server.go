// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
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
	listeners             []listeners.StatsdListener
	packetsIn             chan listeners.Packets
	samplePool            *metrics.MetricSamplePool
	samplesOut            chan<- []metrics.MetricSample
	eventsOut             chan<- []*metrics.Event
	servicesCheckOut      chan<- []*metrics.ServiceCheck
	Statistics            *util.Stats
	Started               bool
	packetPool            *listeners.PacketPool
	stopChan              chan bool
	health                *health.Handle
	metricPrefix          string
	metricPrefixBlacklist []string
	defaultHostname       string
	histToDist            bool
	histToDistPrefix      string
	extraTags             []string
	debugMetricsStats     bool
	metricsStats          map[string]metricStat
	statsLock             sync.Mutex
}

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// NewServer returns a running Dogstatsd server
func NewServer(samplePool *metrics.MetricSamplePool, samplesOut chan<- []metrics.MetricSample, eventsOut chan<- []*metrics.Event, servicesCheckOut chan<- []*metrics.ServiceCheck) (*Server, error) {
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

	var metricsStats bool
	if config.Datadog.GetBool("dogstatsd_metrics_stats_enable") == true {
		log.Info("Dogstatsd: metrics statistics will be stored.")
		metricsStats = true
	}

	packetsChannel := make(chan listeners.Packets, config.Datadog.GetInt("dogstatsd_queue_size"))
	packetPool := listeners.NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUDSListener(packetsChannel, packetPool)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}
	if config.Datadog.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetsChannel, packetPool)
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

	s := &Server{
		Started:               true,
		Statistics:            stats,
		samplePool:            samplePool,
		packetsIn:             packetsChannel,
		samplesOut:            samplesOut,
		eventsOut:             eventsOut,
		servicesCheckOut:      servicesCheckOut,
		listeners:             tmpListeners,
		packetPool:            packetPool,
		stopChan:              make(chan bool),
		health:                health.Register("dogstatsd-main"),
		metricPrefix:          metricPrefix,
		metricPrefixBlacklist: metricPrefixBlacklist,
		defaultHostname:       defaultHostname,
		histToDist:            histToDist,
		histToDistPrefix:      histToDistPrefix,
		extraTags:             extraTags,
		debugMetricsStats:     metricsStats,
		metricsStats:          make(map[string]metricStat),
	}

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
	s.handleMessages()
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
	batcher := newBatcher(s.samplePool, s.samplesOut, s.eventsOut, s.servicesCheckOut)
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.health.C:
		case packets := <-s.packetsIn:
			s.parsePackets(batcher, packets)
		}
	}
}

func nextMessage(packet *[]byte) (message []byte) {
	if len(*packet) == 0 {
		return nil
	}

	advance, message, err := bufio.ScanLines(*packet, true)
	if err != nil || len(message) == 0 {
		return nil
	}

	*packet = (*packet)[advance:]
	return message
}

func (s *Server) parsePackets(batcher *batcher, packets []*listeners.Packet) {
	for _, packet := range packets {
		originTags := findOriginTags(packet.Origin)
		//log.Tracef("Dogstatsd receive: %s", packet.Contents)
		for {
			message := nextMessage(&packet.Contents)
			if message == nil {
				break
			}
			if s.Statistics != nil {
				s.Statistics.StatEvent(1)
			}
			messageType := findMessageType(message)

			switch messageType {
			case serviceCheckType:
				serviceCheck, err := s.parseServiceCheckMessage(message)
				if err != nil {
					log.Errorf("Dogstatsd: error parsing service check: %s", err)
					continue
				}
				serviceCheck.Tags = append(serviceCheck.Tags, originTags...)
				batcher.appendServiceCheck(serviceCheck)
			case eventType:
				event, err := s.parseEventMessage(message)
				if err != nil {
					log.Errorf("Dogstatsd: error parsing event: %s", err)
					continue
				}
				event.Tags = append(event.Tags, originTags...)
				batcher.appendEvent(event)
			case metricSampleType:
				sample, err := s.parseMetricMessage(message)
				if err != nil {
					log.Errorf("Dogstatsd: error parsing metrics: %s", err)
					continue
				}
				if s.debugMetricsStats {
					s.storeMetricStats(sample.Name)
				}
				sample.Tags = append(sample.Tags, originTags...)
				batcher.appendSample(sample)
				if s.histToDist && sample.Mtype == metrics.HistogramType {
					distSample := sample.Copy()
					distSample.Name = s.histToDistPrefix + distSample.Name
					distSample.Mtype = metrics.DistributionType
					batcher.appendSample(*distSample)
				}
			}
		}
	}
	batcher.flush()
}

func (s *Server) parseMetricMessage(message []byte) (metrics.MetricSample, error) {
	sample, err := parseMetricSample(message)
	if err != nil {
		dogstatsdMetricParseErrors.Add(1)
		return metrics.MetricSample{}, err
	}
	metricSample := enrichMetricSample(sample, s.metricPrefix, s.metricPrefixBlacklist, s.defaultHostname)
	metricSample.Tags = append(metricSample.Tags, s.extraTags...)
	dogstatsdMetricPackets.Add(1)
	return metricSample, nil
}

func (s *Server) parseEventMessage(message []byte) (*metrics.Event, error) {
	sample, err := parseEvent(message)
	if err != nil {
		dogstatsdEventParseErrors.Add(1)
		return nil, err
	}
	event := enrichEvent(sample, s.defaultHostname)
	event.Tags = append(event.Tags, s.extraTags...)
	dogstatsdEventPackets.Add(1)
	return event, nil
}

func (s *Server) parseServiceCheckMessage(message []byte) (*metrics.ServiceCheck, error) {
	sample, err := parseServiceCheck(message)
	if err != nil {
		dogstatsdServiceCheckParseErrors.Add(1)
		return nil, err
	}
	serviceCheck := enrichServiceCheck(sample, s.defaultHostname)
	serviceCheck.Tags = append(serviceCheck.Tags, s.extraTags...)
	dogstatsdServiceCheckPackets.Add(1)
	return serviceCheck, nil
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
	return tags
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
	s.health.Deregister()
	s.Started = false
}

func (s *Server) storeMetricStats(name string) {
	now := time.Now()
	s.statsLock.Lock()
	defer s.statsLock.Unlock()
	ms := s.metricsStats[name]
	ms.Count++
	ms.LastSeen = now
	s.metricsStats[name] = ms
}

// GetJSONDebugStats returns jsonified debug statistics.
func (s *Server) GetJSONDebugStats() ([]byte, error) {
	s.statsLock.Lock()
	defer s.statsLock.Unlock()
	return json.Marshal(s.metricsStats)
}

// FormatDebugStats returns a printable version of debug stats.
func FormatDebugStats(stats []byte) (string, error) {
	var dogStats map[string]metricStat
	if err := json.Unmarshal(stats, &dogStats); err != nil {
		return "", err
	}

	// put tags in order: first is the more frequent
	order := make([]string, len(dogStats))
	i := 0
	for tag := range dogStats {
		order[i] = tag
		i++
	}

	sort.Slice(order, func(i, j int) bool {
		return dogStats[order[i]].Count > dogStats[order[j]].Count
	})

	// write the response
	buf := bytes.NewBuffer(nil)

	header := fmt.Sprintf("%-40s | %-10s | %-20s\n", "Metric", "Count", "Last Seen")
	buf.Write([]byte(header))
	buf.Write([]byte(strings.Repeat("-", len(header)) + "\n"))

	for _, metric := range order {
		stats := dogStats[metric]
		buf.Write([]byte(fmt.Sprintf("%-40s | %-10d | %-20v\n", metric, stats.Count, stats.LastSeen)))
	}

	if len(dogStats) == 0 {
		buf.Write([]byte("No metrics processed yet."))
	}

	return buf.String(), nil
}
