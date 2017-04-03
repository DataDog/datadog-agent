package dogstatsd

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Server represent a Dogstatsd server
type Server struct {
	conn    *net.UDPConn
	Started bool
}

// NewServer returns a running Dogstatsd server
func NewServer(metricOut chan<- *aggregator.MetricSample, eventOut chan<- aggregator.Event, serviceCheckOut chan<- aggregator.ServiceCheck) (*Server, error) {
	var url string
	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
	}

	address, addrErr := net.ResolveUDPAddr("udp", url)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd: can't ResolveUDPAddr %s: %v", url, addrErr)
	}

	conn, err := net.ListenUDP("udp", address)
	if err != nil {
		return nil, fmt.Errorf("dogstatsd: can't listen: %s", err)
	}

	s := &Server{
		Started: true,
		conn:    conn,
	}
	go s.handleMessages(metricOut, eventOut, serviceCheckOut)
	log.Infof("dogstatsd: listening on %s", address)
	return s, nil
}

func (s *Server) handleMessages(metricOut chan<- *aggregator.MetricSample, eventOut chan<- aggregator.Event, serviceCheckOut chan<- aggregator.ServiceCheck) {
	for {
		buf := make([]byte, config.Datadog.GetInt("dogstatsd_buffer_size"))
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd: error reading packet: %v", err)
			continue
		}

		datagram := buf[:n]

		go func() {
			for {
				packet := nextPacket(&datagram)
				if packet == nil {
					break
				}

				if bytes.HasPrefix(packet, []byte("_sc")) {
					serviceCheck, err := parseServiceCheckPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing service check: %s", err)
						continue
					}
					serviceCheckOut <- *serviceCheck
				} else if bytes.HasPrefix(packet, []byte("_e")) {
					event, err := parseEventPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing evet: %s", err)
						continue
					}
					eventOut <- *event
				} else {
					sample, err := parseMetricPacket(packet)
					if err != nil {
						log.Errorf("dogstatsd: error parsing metrics: %s", err)
						continue
					}
					metricOut <- sample
				}
			}
		}()
	}
}

// Stop stops a running Dogstatsd server
func (s *Server) Stop() {
	s.conn.Close()
	s.Started = false
}
