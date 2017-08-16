package listeners

import (
	"fmt"
	"net"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// UdpListener implements the StatsdListener interface for UDP protocol.
// It listens to a given UDP address and sends back payloads ready to be
// processed.
// Origin detection is not implemented for UDP.
type UdpListener struct {
	conn       net.PacketConn
	payloadOut chan *Payload
	Started    bool
}

// NewUdpListener returns an idle UDP Statsd listener
func NewUdpListener(payloadOut chan *Payload) (*UdpListener, error) {
	var conn net.PacketConn
	var err error
	var url string

	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
	}

	conn, err = net.ListenPacket("udp", url)

	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	listener := &UdpListener{
		Started:    false,
		payloadOut: payloadOut,
		conn:       conn,
	}
	log.Infof("dogstatsd: listening on %s", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (s *UdpListener) Listen() {
	for {
		buf := make([]byte, config.Datadog.GetInt("dogstatsd_buffer_size"))
		n, _, err := s.conn.ReadFrom(buf)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd: error reading packet: %v", err)
			//FIXME//dogstatsdExpvar.Add("PacketReadingErrors", 1)
			continue
		}

		payload := &Payload{
			Contents: buf[:n],
		}
		s.payloadOut <- payload

	}
}

// Stop closes the UDP connection and stops listening
func (l *UdpListener) Stop() {
	l.Started = false
	l.conn.Close()
}
