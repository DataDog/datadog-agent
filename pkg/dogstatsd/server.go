package dogstatsd

import (
	"bytes"
	"expvar"
	"fmt"
	"net"
	"os"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	dogstatsdExpvar = expvar.NewMap("dogstatsd")
)

// Server represent a Dogstatsd server
type Server struct {
	conn       net.PacketConn
	Statistics *util.Stats
	Started    bool
}

// NewServer returns a running Dogstatsd server
func NewServer(metricOut chan<- *aggregator.MetricSample, eventOut chan<- aggregator.Event, serviceCheckOut chan<- aggregator.ServiceCheck) (*Server, error) {
	var conn net.PacketConn
	var err error

	var stats *util.Stats
	if config.Datadog.GetBool("dogstatsd_stats_enable") == true {
		buff := config.Datadog.GetInt("dogstatsd_stats_buffer")
		s, err := util.NewStats(uint32(buff))
		if err != nil {
			log.Errorf("dogstatsd: unable to start statistics facilities")
		}
		stats = s
	}

	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) == 0 {
		var url string

		if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
			// Listen to all network interfaces
			url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
		} else {
			url = fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
		}

		conn, err = net.ListenPacket("udp", url)
	} else {
		conn, err = net.ListenPacket("unixgram", socketPath)
	}

	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	s := &Server{
		Started:    true,
		Statistics: stats,
		conn:       conn,
	}
	go s.handleMessages(metricOut, eventOut, serviceCheckOut)
	log.Infof("dogstatsd: listening on %s", conn.LocalAddr())
	return s, nil
}

func (s *Server) handleMessages(metricOut chan<- *aggregator.MetricSample, eventOut chan<- aggregator.Event, serviceCheckOut chan<- aggregator.ServiceCheck) {
	if s.Statistics != nil {
		go s.Statistics.Process()
		defer s.Statistics.Stop()
	}
	for {
		buf := make([]byte, config.Datadog.GetInt("dogstatsd_buffer_size"))
		n, _, err := s.conn.ReadFrom(buf)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd: error reading packet: %v", err)
			dogstatsdExpvar.Add("PacketReadingErrors", 1)
			continue
		}

		datagram := buf[:n]
		log.Debugf("dogstatsd receive: %s", datagram)

		go func() {
			for {
				packet := nextPacket(&datagram)
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
	s.conn.Close()

	// Socket cleanup on exit
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		err := os.Remove(socketPath)
		if err != nil {
			log.Infof("dogstatsd: error removing socket file: %s", err)
		}
	}
	s.Started = false
}
