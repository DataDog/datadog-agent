// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package dogstatsd

import (
	"bytes"
	"expvar"
	"fmt"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	dogstatsdExpvar = expvar.NewMap("dogstatsd")
)

// Server represent a Dogstatsd server
type Server struct {
	listeners  []listeners.StatsdListener
	packetIn   chan *listeners.Packet // Unbuffered channel as packets processing is done in goroutines
	Statistics *util.Stats
	Started    bool
}

// NewServer returns a running Dogstatsd server
func NewServer(metricOut chan<- *metrics.MetricSample, eventOut chan<- metrics.Event, serviceCheckOut chan<- metrics.ServiceCheck) (*Server, error) {
	var stats *util.Stats
	if config.Datadog.GetBool("dogstatsd_stats_enable") == true {
		buff := config.Datadog.GetInt("dogstatsd_stats_buffer")
		s, err := util.NewStats(uint32(buff))
		if err != nil {
			log.Errorf("dogstatsd: unable to start statistics facilities")
		}
		stats = s
	}

	packetChannel := make(chan *listeners.Packet)
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUDSListener(packetChannel)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}
	if config.Datadog.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetChannel)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, udpListener)
		}
	}

	if len(tmpListeners) == 0 {
		return nil, fmt.Errorf("listening on neither udp nor socket, please check your configuration")
	}

	s := &Server{
		Started:    true,
		Statistics: stats,
		packetIn:   packetChannel,
		listeners:  tmpListeners,
	}
	go s.handleMessages(metricOut, eventOut, serviceCheckOut)

	return s, nil
}

func (s *Server) handleMessages(metricOut chan<- *metrics.MetricSample, eventOut chan<- metrics.Event, serviceCheckOut chan<- metrics.ServiceCheck) {
	if s.Statistics != nil {
		go s.Statistics.Process()
		defer s.Statistics.Stop()
	}

	for _, l := range s.listeners {
		go l.Listen()
	}

	for {
		packet := <-s.packetIn
		var originTags []string

		if packet.Origin != "" {
			var err error
			log.Debugf("dogstatsd receive from %s: %s", packet.Origin, packet.Contents)
			originTags, err = tagger.Tag(packet.Origin, false)
			if err != nil {
				log.Errorf(err.Error())
			}
			log.Debugf("tags for %s: %s", packet.Origin, originTags)

		} else {
			log.Debugf("dogstatsd receive: %s", packet.Contents)
		}

		go func() {
			for {
				packet := nextPacket(&packet.Contents)
				if packet == nil {
					break
				}

				if s.Statistics != nil {
					s.Statistics.StatEvent(1)
				}

				if bytes.HasPrefix(packet, []byte("_sc")) {
					serviceCheck, err := parseServiceCheckPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing service check: %s", err)
						dogstatsdExpvar.Add("ServiceCheckParseErrors", 1)
						continue
					}
					if len(originTags) > 0 {
						serviceCheck.Tags = append(serviceCheck.Tags, originTags...)
					}
					dogstatsdExpvar.Add("ServiceCheckPackets", 1)
					select {
					case serviceCheckOut <- *serviceCheck:
					default:
						// Aggregator is too busy, drop packet
						dogstatsdExpvar.Add("ServiceCheckPacketDropped", 1)
					}
				} else if bytes.HasPrefix(packet, []byte("_e")) {
					event, err := parseEventPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing event: %s", err)
						dogstatsdExpvar.Add("EventParseErrors", 1)
						continue
					}
					if len(originTags) > 0 {
						event.Tags = append(event.Tags, originTags...)
					}
					dogstatsdExpvar.Add("EventPackets", 1)
					select {
					case eventOut <- *event:
					default:
						// Aggregator is too busy, drop packet
						dogstatsdExpvar.Add("EventPacketDropped", 1)
					}
				} else {
					sample, err := parseMetricPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing metrics: %s", err)
						dogstatsdExpvar.Add("MetricParseErrors", 1)
						continue
					}
					if len(originTags) > 0 {
						sample.Tags = append(sample.Tags, originTags...)
					}
					dogstatsdExpvar.Add("MetricPackets", 1)
					select {
					case metricOut <- sample:
					default:
						// Aggregator is too busy, drop packet
						dogstatsdExpvar.Add("MetricPacketDropped", 1)
					}
				}
			}
		}()
	}
}

// Stop stops a running Dogstatsd server
func (s *Server) Stop() {
	for _, l := range s.listeners {
		l.Stop()
	}
	s.Started = false
}
