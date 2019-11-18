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
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/mapper"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
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
	mapper   				*mapper.MetricMapper
}

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// NewServer returns a running Dogstatsd server
func NewServer(metricOut chan<- []*metrics.MetricSample, eventOut chan<- []*metrics.Event, serviceCheckOut chan<- []*metrics.ServiceCheck) (*Server, error) {
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
		packetsIn:             packetsChannel,
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

	s.handleMessages(metricOut, eventOut, serviceCheckOut)

	mappingYaml := config.Datadog.GetString("mapping_yaml")
	if mappingYaml != "" {
		s.mapper = &mapper.MetricMapper{}
		err := s.mapper.InitFromYAMLString(mappingYaml, 5000)
		if err != nil {
			log.Error("Error loading config:", err)
		}
		err = dumpFSM(s.mapper, "/tmp/fsm.txt")
		if err != nil {
			log.Error("Error dumping FSM:", err)
		}
	}

	return s, nil
}


func dumpFSM(mapper *mapper.MetricMapper, dumpFilename string) error {
	f, err := os.Create(dumpFilename)
	if err != nil {
		return err
	}
	log.Info("Start dumping FSM to", dumpFilename)
	w := bufio.NewWriter(f)
	mapper.FSM.DumpFSM(w)
	w.Flush()
	f.Close()
	log.Info("Finish dumping FSM")
	return nil
}

func (s *Server) handleMessages(metricOut chan<- []*metrics.MetricSample, eventOut chan<- []*metrics.Event, serviceCheckOut chan<- []*metrics.ServiceCheck) {
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
		go s.worker(metricOut, eventOut, serviceCheckOut)
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

func (s *Server) worker(metricOut chan<- []*metrics.MetricSample, eventOut chan<- []*metrics.Event, serviceCheckOut chan<- []*metrics.ServiceCheck) {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.health.C:
		case packets := <-s.packetsIn:
			events := make([]*metrics.Event, 0, len(packets))
			serviceChecks := make([]*metrics.ServiceCheck, 0, len(packets))
			metricSamples := make([]*metrics.MetricSample, 0, len(packets))

			for _, packet := range packets {
				metricSamples, events, serviceChecks = s.parsePacket(packet, metricSamples, events, serviceChecks)
				s.packetPool.Put(packet)
			}

			if len(metricSamples) != 0 {
				metricOut <- metricSamples
			}
			if len(events) != 0 {
				eventOut <- events
			}
			if len(serviceChecks) != 0 {
				serviceCheckOut <- serviceChecks
			}
		}
	}
}

func (s *Server) parsePacket(packet *listeners.Packet, metricSamples []*metrics.MetricSample, events []*metrics.Event, serviceChecks []*metrics.ServiceCheck) ([]*metrics.MetricSample, []*metrics.Event, []*metrics.ServiceCheck) {
	extraTags := s.extraTags

	log.Tracef("Dogstatsd receive: %s", packet.Contents)
	if packet.Origin != listeners.NoOrigin {
		originTags, err := tagger.Tag(packet.Origin, tagger.DogstatsdCardinality)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			extraTags = append(extraTags, originTags...)
		}
	}

	for {
		message := nextMessage(&packet.Contents)
		if message == nil {
			break
		}

		if s.Statistics != nil {
			s.Statistics.StatEvent(1)
		}

		if bytes.HasPrefix(message, []byte("_sc")) {
			serviceCheck, err := parseServiceCheckMessage(message, s.defaultHostname)
			if err != nil {
				log.Errorf("Dogstatsd: error parsing service check: %s", err)
				dogstatsdServiceCheckParseErrors.Add(1)
				continue
			}
			if len(extraTags) > 0 {
				serviceCheck.Tags = append(serviceCheck.Tags, extraTags...)
			}
			dogstatsdServiceCheckPackets.Add(1)
			serviceChecks = append(serviceChecks, serviceCheck)
		} else if bytes.HasPrefix(message, []byte("_e")) {
			event, err := parseEventMessage(message, s.defaultHostname)
			if err != nil {
				log.Errorf("Dogstatsd: error parsing event: %s", err)
				dogstatsdEventParseErrors.Add(1)
				continue
			}
			if len(extraTags) > 0 {
				event.Tags = append(event.Tags, extraTags...)
			}
			dogstatsdEventPackets.Add(1)
			events = append(events, event)
		} else {
			sample, err := parseMetricMessage(message, s.metricPrefix, s.metricPrefixBlacklist, s.defaultHostname)

			if s.mapper != nil {
				m, labels, present := s.mapper.GetMapping(sample.Name, mapper.MetricTypeGauge)

				if present {
					sample.Name = m.Name
					sample.Tags = append(sample.Tags, labels...)
				}
			}

			if err != nil {
				log.Errorf("Dogstatsd: error parsing metrics: %s", err)
				dogstatsdMetricParseErrors.Add(1)
				continue
			}
			if s.debugMetricsStats {
				s.storeMetricStats(sample.Name)
			}
			if len(extraTags) > 0 {
				sample.Tags = append(sample.Tags, extraTags...)
			}
			dogstatsdMetricPackets.Add(1)
			metricSamples = append(metricSamples, sample)
			if s.histToDist && sample.Mtype == metrics.HistogramType {
				distSample := sample.Copy()
				distSample.Name = s.histToDistPrefix + distSample.Name
				distSample.Mtype = metrics.DistributionType
				metricSamples = append(metricSamples, distSample)
			}
		}
	}
	return metricSamples, events, serviceChecks
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
