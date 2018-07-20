// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	packetIn         chan *listeners.Packet
	Statistics       *util.Stats
	Started          bool
	packetPool       *listeners.PacketPool
	stopChan         chan bool
	health           *health.Handle
	metricPrefix     string
	defaultHostname  string
	histToDist       bool
	histToDistPrefix string
}

// NewServer returns a running Dogstatsd server
func NewServer(metricOut chan<- *metrics.MetricSample, eventOut chan<- metrics.Event, serviceCheckOut chan<- metrics.ServiceCheck) (*Server, error) {
	var stats *util.Stats
	if config.Datadog.GetBool("dogstatsd_stats_enable") == true {
		buff := config.Datadog.GetInt("dogstatsd_stats_buffer")
		s, err := util.NewStats(uint32(buff))
		if err != nil {
			log.Errorf("Dogstatsd: unable to start statistics facilities")
		}
		stats = s
	}

	packetChannel := make(chan *listeners.Packet, 100)
	packetPool := listeners.NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))
	tmpListeners := make([]listeners.StatsdListener, 0, 2)

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		unixListener, err := listeners.NewUDSListener(packetChannel, packetPool)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tmpListeners = append(tmpListeners, unixListener)
		}
	}
	if config.Datadog.GetInt("dogstatsd_port") > 0 {
		udpListener, err := listeners.NewUDPListener(packetChannel, packetPool)
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
	s := &Server{
		Started:          true,
		Statistics:       stats,
		packetIn:         packetChannel,
		listeners:        tmpListeners,
		packetPool:       packetPool,
		stopChan:         make(chan bool),
		health:           health.Register("dogstatsd-main"),
		metricPrefix:     metricPrefix,
		defaultHostname:  defaultHostname,
		histToDist:       histToDist,
		histToDistPrefix: histToDistPrefix,
	}

	forwardHost := config.Datadog.GetString("statsd_forward_host")
	forwardPort := config.Datadog.GetInt("statsd_forward_port")

	if forwardHost != "" && forwardPort != 0 {

		forwardAddress := fmt.Sprintf("%s:%d", forwardHost, forwardPort)

		con, err := net.Dial("udp", forwardAddress)

		if err != nil {
			log.Warnf("Could not connect to statsd forward host : %s", err)
		} else {
			s.packetIn = make(chan *listeners.Packet, 100)
			go s.forwarder(con, packetChannel)
		}
	}

	s.handleMessages(metricOut, eventOut, serviceCheckOut)

	return s, nil
}

func (s *Server) handleMessages(metricOut chan<- *metrics.MetricSample, eventOut chan<- metrics.Event, serviceCheckOut chan<- metrics.ServiceCheck) {
	if s.Statistics != nil {
		go s.Statistics.Process()
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

func (s *Server) forwarder(fcon net.Conn, packetChannel chan *listeners.Packet) {
	for {
		select {
		case <-s.stopChan:
			return
		case packet := <-packetChannel:
			_, err := fcon.Write(packet.Contents)

			if err != nil {
				log.Warnf("Forwarding packet failed : %s", err)
			}

			s.packetIn <- packet
		}
	}
}

func (s *Server) worker(metricOut chan<- *metrics.MetricSample, eventOut chan<- metrics.Event, serviceCheckOut chan<- metrics.ServiceCheck) {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.health.C:
		case packet := <-s.packetIn:
			var originTags []string

			if packet.Origin != listeners.NoOrigin {
				var err error
				log.Tracef("Dogstatsd receive from %s: %s", packet.Origin, packet.Contents)
				originTags, err = tagger.Tag(packet.Origin, tagger.IsFullCardinality())
				if err != nil {
					log.Errorf(err.Error())
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
					serviceCheck, err := parseServiceCheckMessage(message)
					if err != nil {
						log.Errorf("Dogstatsd: error parsing service check: %s", err)
						dogstatsdServiceCheckParseErrors.Add(1)
						continue
					}
					if len(originTags) > 0 {
						serviceCheck.Tags = append(serviceCheck.Tags, originTags...)
					}
					dogstatsdServiceCheckPackets.Add(1)
					serviceCheckOut <- *serviceCheck
				} else if bytes.HasPrefix(message, []byte("_e")) {
					event, err := parseEventMessage(message)
					if err != nil {
						log.Errorf("Dogstatsd: error parsing event: %s", err)
						dogstatsdEventParseErrors.Add(1)
						continue
					}
					if len(originTags) > 0 {
						event.Tags = append(event.Tags, originTags...)
					}
					dogstatsdEventPackets.Add(1)
					eventOut <- *event
				} else {
					sample, err := parseMetricMessage(message, s.metricPrefix, s.defaultHostname)
					if err != nil {
						log.Errorf("Dogstatsd: error parsing metrics: %s", err)
						dogstatsdMetricParseErrors.Add(1)
						continue
					}
					if len(originTags) > 0 {
						sample.Tags = append(sample.Tags, originTags...)
					}
					dogstatsdMetricPackets.Add(1)
					metricOut <- sample
					if s.histToDist && sample.Mtype == metrics.HistogramType {
						distSample := sample.Copy()
						distSample.Name = s.histToDistPrefix + distSample.Name
						distSample.Mtype = metrics.DistributionType
						metricOut <- distSample
					}
				}
			}
			// Return the packet object back to the object pool for reuse
			s.packetPool.Put(packet)
		}
	}
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
