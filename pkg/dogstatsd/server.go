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
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	dogstatsdExpvar = expvar.NewMap("dogstatsd")
)

// Server represent a Dogstatsd server
type Server struct {
	listeners  []listeners.StatsdListener
	payloadIn  chan *listeners.Payload // Unbuffered channel as payloads processing is done in goroutines
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

	payloadChannel := make(chan *listeners.Payload)
	intakes := make([]listeners.StatsdListener, 0, 2)

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUnixListener(payloadChannel)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			intakes = append(intakes, unixListener)
		}
	}
	if config.Datadog.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUdpListener(payloadChannel)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			intakes = append(intakes, udpListener)
		}
	}

	if len(intakes) == 0 {
		return nil, fmt.Errorf("listening on neither udp nor socket, please check your configuration")
	}

	s := &Server{
		Started:    true,
		Statistics: stats,
		payloadIn:  payloadChannel,
		listeners:  intakes,
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
		payload := <-s.payloadIn

		if payload.Container != "" {
			log.Debugf("dogstatsd receive from %s: %s", payload.Container, payload.Contents)
		} else {
			log.Debugf("dogstatsd receive: %s", payload.Contents)
		}

		go func() {
			for {
				packet := nextPacket(&payload.Contents)
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
					dogstatsdExpvar.Add("ServiceCheckPackets", 1)
					serviceCheckOut <- *serviceCheck
				} else if bytes.HasPrefix(packet, []byte("_e")) {
					event, err := parseEventPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing event: %s", err)
						dogstatsdExpvar.Add("EventParseErrors", 1)
						continue
					}
					dogstatsdExpvar.Add("EventPackets", 1)
					eventOut <- *event
				} else {
					sample, err := parseMetricPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing metrics: %s", err)
						dogstatsdExpvar.Add("MetricParseErrors", 1)
						continue
					}
					dogstatsdExpvar.Add("MetricPackets", 1)
					metricOut <- sample
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
