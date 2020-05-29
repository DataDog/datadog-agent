// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	addrToEntityCacheKeyPrefix = "addr_to_entity"
	addrToEntityCacheDuration  = time.Minute
)

var (
	udpExpvars             = expvar.NewMap("dogstatsd-udp")
	udpPacketReadingErrors = expvar.Int{}
	udpPackets             = expvar.Int{}
	udpBytes               = expvar.Int{}

	tlmUDPPackets = telemetry.NewCounter("dogstatsd", "udp_packets",
		[]string{"state"}, "Dogstatsd UDP packets count")
	tlmUDPPacketsBytes = telemetry.NewCounter("dogstatsd", "udp_packets_bytes",
		nil, "Dogstatsd UDP packets bytes count")
)

func init() {
	udpExpvars.Set("PacketReadingErrors", &udpPacketReadingErrors)
	udpExpvars.Set("Packets", &udpPackets)
	udpExpvars.Set("Bytes", &udpBytes)
}

// UDPListener implements the StatsdListener interface for UDP protocol.
// It listens to a given UDP address and sends back packets ready to be
// processed.
type UDPListener struct {
	conn             *net.UDPConn
	packetsBuffer    *packetsBuffer
	packetAssembler  *packetAssembler
	sharedPacketPool *PacketPool
	buffer           []byte
	OriginDetection  bool
}

// NewUDPListener returns an idle UDP Statsd listener
func NewUDPListener(packetOut chan Packets, sharedPacketPool *PacketPool) (*UDPListener, error) {
	var err error
	var url string

	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = net.JoinHostPort(config.Datadog.GetString("bind_host"), config.Datadog.GetString("dogstatsd_port"))
	}

	addr, err := net.ResolveUDPAddr("udp", url)
	if err != nil {
		return nil, fmt.Errorf("could not resolve udp addr: %s", err)
	}
	conn, err := net.ListenUDP("udp", addr)

	if rcvbuf := config.Datadog.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			return nil, fmt.Errorf("could not set socket rcvbuf: %s", err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")
	originDetection := config.Datadog.GetBool("dogstatsd_origin_detection")

	buffer := make([]byte, bufferSize)
	packetsBuffer := newPacketsBuffer(uint(packetsBufferSize), flushTimeout, packetOut)
	packetAssembler := newPacketAssembler(flushTimeout, packetsBuffer, sharedPacketPool)

	listener := &UDPListener{
		OriginDetection:  originDetection,
		conn:             conn,
		packetsBuffer:    packetsBuffer,
		packetAssembler:  packetAssembler,
		sharedPacketPool: sharedPacketPool,
		buffer:           buffer,
	}
	log.Debugf("dogstatsd-udp: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDPListener) Listen() {
	log.Infof("dogstatsd-udp: starting to listen on %s", l.conn.LocalAddr())
	for {
		udpPackets.Add(1)
		n, addr, err := l.conn.ReadFrom(l.buffer)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-udp: error reading packet: %v", err)
			udpPacketReadingErrors.Add(1)
			tlmUDPPackets.Inc("error")
			continue
		}
		tlmUDPPackets.Inc("ok")

		udpBytes.Add(int64(n))
		tlmUDPPacketsBytes.Add(float64(n))

		// TODO: extend packetAssembler to support setting per-packet origin
		if l.OriginDetection {
			// retrieve an available packet from the packet pool,
			// which will be pushed back by the server when processed.
			packet := l.sharedPacketPool.Get()
			packet.Contents = l.buffer[:n]

			// Extract container id from source address
			container, taggingErr := getEntityForAddr(addr)
			if taggingErr != nil {
				log.Warnf("dogstatsd-udp: error processing origin, data will not be tagged : %v", taggingErr)
				udsOriginDetectionErrors.Add(1)
				tlmUDSOriginDetectionError.Inc()
			} else {
				packet.Origin = container
			}

			// packetsBuffer handles the forwarding of the packets to the dogstatsd server intake channel
			l.packetsBuffer.append(packet)
		} else {
			// packetAssembler merges multiple packets together and sends them when its buffer is full
			l.packetAssembler.addMessage(l.buffer[:n])
		}
	}
}

// Stop closes the UDP connection and stops listening
func (l *UDPListener) Stop() {
	l.packetAssembler.close()
	l.packetsBuffer.close()
	l.conn.Close()
}

// getEntityForAddr returns the container entity name and caches the value for future lookups
// As the result is cached and the lookup is really fast (parsing local files), it can be
// called from the intake goroutine.
func getEntityForAddr(addr net.Addr) (string, error) {
	key := cache.BuildAgentKey(addrToEntityCacheKeyPrefix, addr.String())
	if x, found := cache.Cache.Get(key); found {
		return x.(string), nil
	}

	entity, err := entityForAddr(addr)
	switch err {
	case nil:
		// No error, yay!
		cache.Cache.Set(key, entity, addrToEntityCacheDuration)
		return entity, nil
	case errNoContainerMatch:
		// No runtime detected, cache the `NoOrigin` result
		cache.Cache.Set(key, NoOrigin, addrToEntityCacheDuration)
		return NoOrigin, nil
	default:
		// Other lookup error, retry next time
		return NoOrigin, err
	}
}

// entityForAddr returns the entity ID for a given network address.
func entityForAddr(addr net.Addr) (string, error) {
	// TODO: support other providers (e.g. docker)
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return "", err
	}

	podIP := addr.(*net.UDPAddr).IP.String()
	pod, err := ku.GetPodFromPodIP(podIP)
	if err != nil {
		return "", err
	}

	return kubelet.PodUIDToTaggerEntityName(pod.Metadata.UID), nil
}
