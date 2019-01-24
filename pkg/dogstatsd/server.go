// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"bytes"
	"expvar"
	"fmt"
	"net"
	"runtime"
	"strings"

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
	listeners        []listeners.StatsdListener
	packetsIn        chan listeners.Packets
	Statistics       *util.Stats
	Started          bool
	packetPool       *listeners.PacketPool
	stopChan         chan bool
	health           *health.Handle
	metricPrefix     string
	defaultHostname  string
	histToDist       bool
	histToDistPrefix string
	extraTags        []string
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

	packetsChannel := make(chan listeners.Packets, 100)
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

	defaultHostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Dogstatsd: unable to determine default hostname: %s", err.Error())
	}

	histToDist := config.Datadog.GetBool("histogram_copy_to_distribution")
	histToDistPrefix := config.Datadog.GetString("histogram_copy_to_distribution_prefix")

	extraTags := config.Datadog.GetStringSlice("dogstatsd_tags")

	s := &Server{
		Started:          true,
		Statistics:       stats,
		packetsIn:        packetsChannel,
		listeners:        tmpListeners,
		packetPool:       packetPool,
		stopChan:         make(chan bool),
		health:           health.Register("dogstatsd-main"),
		metricPrefix:     metricPrefix,
		defaultHostname:  defaultHostname,
		histToDist:       histToDist,
		histToDistPrefix: histToDistPrefix,
		extraTags:        extraTags,
	}

	forwardHost := config.Datadog.GetString("statsd_forward_host")
	forwardPort := config.Datadog.GetInt("statsd_forward_port")

	if forwardHost != "" && forwardPort != 0 {

		forwardAddress := fmt.Sprintf("%s:%d", forwardHost, forwardPort)

		con, err := net.Dial("udp", forwardAddress)

		if err != nil {
			log.Warnf("Could not connect to statsd forward host : %s", err)
		} else {
			s.packetsIn = make(chan listeners.Packets, 100)
			go s.forwarder(con, packetsChannel)
		}
	}

	s.handleMessages(metricOut, eventOut, serviceCheckOut)

	return s, nil
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

	if packet.Origin != listeners.NoOrigin {
		log.Tracef("Dogstatsd receive from %s: %s", packet.Origin, packet.Contents)
		originTags, err := tagger.Tag(packet.Origin, tagger.DogstatsdCardinality)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			extraTags = append(extraTags, originTags...)
		}
		log.Tracef("Tags for %s: %s", packet.Origin, originTags)
	} else {
		log.Tracef("Dogstatsd receive: %s", packet.Contents)
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
			sample, err := parseMetricMessage(message, s.metricPrefix, s.defaultHostname)
			if err != nil {
				log.Errorf("Dogstatsd: error parsing metrics: %s", err)
				dogstatsdMetricParseErrors.Add(1)
				continue
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
