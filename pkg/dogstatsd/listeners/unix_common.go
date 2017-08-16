package listeners

import (
	"fmt"
	"net"
	"os"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// UnixListener implements the StatsdListener interface for UDS protocol.
// It listens to a given Unix Domain Socket path and sends back payloads
// ready to be processed.
// Origin detection will be implemented for UDS.
type UnixListener struct {
	conn            *net.UnixConn
	payloadOut      chan *Payload
	Started         bool
	OriginDetection bool
}

// NewUdsListener returns an idle UDS Statsd listener
func NewUnixListener(payloadOut chan *Payload) (*UnixListener, error) {
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	originDection := config.Datadog.GetBool("dogstatsd_origin_detection")

	address, addrErr := net.ResolveUnixAddr("unixgram", socketPath)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd: can't ResolveUnixAddr: %v", addrErr)
	}
	conn, err := net.ListenUnixgram("unixgram", address)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	if originDection {
		err = enablePassCred(conn)
		if err != nil {
			log.Errorf("dogstatsd: error enabling origin detection: %s", err)
			originDection = false
		} else {
			log.Infof("dogstatsd: enabling origin detection on %s", conn.LocalAddr())
		}
	}

	listener := &UnixListener{
		Started:         false,
		OriginDetection: originDection,
		payloadOut:      payloadOut,
		conn:            conn,
	}

	log.Infof("dogstatsd: listening on %s", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (s *UnixListener) Listen() {
	for {
		buf := make([]byte, config.Datadog.GetInt("dogstatsd_buffer_size"))
		var n int
		var err error
		payload := &Payload{}

		if s.OriginDetection {
			// Read datagram + credentials in ancilary data
			oob := make([]byte, oob_size)
			var oobn int
			n, oobn, _, _, err = s.conn.ReadMsgUnix(buf, oob)
			log.Infof("dogstatsd: n %d oobn %d", n, oobn)

			// Extract PID from credentials
			container, err := processOrigin(oob[:oobn])
			if err != nil {
				log.Warnf("dogstatsd: error processing origin, data will not be tagged : %v", err)
			} else {
				payload.Container = container
			}
		} else {
			// Read only datagram contents with no credentials
			n, _, err = s.conn.ReadFromUnix(buf)
		}

		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd: error reading packet: %v", err)
			//FIXME//dogstatsdExpvar.Add("PacketReadingErrors", 1)
			continue
		}

		payload.Contents = buf[:n]
		s.payloadOut <- payload
	}
}

// Stop closes the UDS connection and stops listening
func (l *UnixListener) Stop() {
	l.Started = false
	l.conn.Close()

	// Socket cleanup on exit
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		err := os.Remove(socketPath)
		if err != nil {
			log.Infof("dogstatsd: error removing socket file: %s", err)
		}
	}
}
