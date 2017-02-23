package dogstatsd

import (
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
func NewServer(out chan *aggregator.MetricSample) (*Server, error) {
	url := fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
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
	go s.handleMessages(out)
	log.Infof("dogstatsd: listening on %s", address)
	return s, nil
}

func (s *Server) handleMessages(out chan *aggregator.MetricSample) {
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
				sample, err := nextMetric(&datagram)
				if err != nil {
					log.Errorf("dogstatsd: error parsing datagram: %s", err)
					continue
				}

				if sample == nil {
					break
				}

				out <- sample
			}
		}()
	}
}

// Stop stops a running Dogstatsd server
func (s *Server) Stop() {
	s.conn.Close()
	s.Started = false
}
