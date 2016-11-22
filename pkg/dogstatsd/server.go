package dogstatsd

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	log "github.com/cihub/seelog"
)

// RunServer starts and run a dogstatsd server
func RunServer(out chan *aggregator.MetricSample) {
	address, _ := net.ResolveUDPAddr("udp", "localhost:8126") // TODO: configurable bind address
	serverConn, err := net.ListenUDP("udp", address)
	log.Infof("listening on %s", address)
	defer serverConn.Close()

	if err != nil {
		log.Criticalf("Can't listen: %s", err)
	}

	for {
		buf := make([]byte, 1024) // TODO: configurable
		n, _, err := serverConn.ReadFromUDP(buf)
		if err != nil {
			log.Error("Error reading packet")
			continue
		}

		datagram := buf[:n]

		for {
			sample, err := nextMetric(&datagram)
			if err != nil {
				log.Errorf("Error parsing datagram: %s", err)
				continue
			}

			if sample == nil {
				break
			}

			out <- sample
		}
	}
}
