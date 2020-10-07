// +build linux_bpf

package network

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/tcpassembly"
)

// HTTPSocketFilterSnooper is a HTTP traffic snooper built on top of an eBPF SOCKET_FILTER
type HTTPSocketFilterSnooper struct {
	source     *packetSource
	statKeeper *httpStatKeeper
	exit       chan struct{}
	wg         sync.WaitGroup

	// Telemetry
	socketPolls        int64
	processedPackets   int64
	capturedPackets    int64
	droppedPackets     int64
	skippedPackets     int64
	validPackets       int64
	responses          int64
	requests           int64
	connectionsFlushed int64
	connectionsClosed  int64
}

// NewHTTPSocketFilterSnooper returns a new HTTPSocketFilterSnooper
func NewHTTPSocketFilterSnooper(rootPath string, filter *manager.Probe, httpTimeout time.Duration) (*HTTPSocketFilterSnooper, error) {
	var (
		packetSrc *packetSource
		srcErr    error
	)

	// Create the RAW_SOCKET inside the root network namespace
	nsErr := util.WithRootNS(rootPath, func() {
		packetSrc, srcErr = newPacketSource(filter)
	})
	if nsErr != nil {
		return nil, nsErr
	}
	if srcErr != nil {
		return nil, srcErr
	}
	statKeeper := &httpStatKeeper{
		requests: make(map[httpKey]httpInfo),
	}
	snooper := &HTTPSocketFilterSnooper{
		source:     packetSrc,
		statKeeper: statKeeper,
		exit:       make(chan struct{}),
	}

	// Start consuming packets
	snooper.wg.Add(1)
	go func() {
		snooper.pollPackets(httpTimeout)
		snooper.wg.Done()
	}()

	// Start polling socket stats
	snooper.wg.Add(1)
	go func() {
		snooper.pollStats()
		snooper.wg.Done()
	}()

	return snooper, nil
}

func (s *HTTPSocketFilterSnooper) GetHTTPStats() map[httpKey]httpInfo {
	return s.statKeeper.requests
}

func (s *HTTPSocketFilterSnooper) GetStats() map[string]int64 {
	return map[string]int64{
		"socket_polls":         atomic.LoadInt64(&s.socketPolls),
		"packets_processed":    atomic.LoadInt64(&s.processedPackets),
		"packets_captured":     atomic.LoadInt64(&s.capturedPackets),
		"packets_dropped":      atomic.LoadInt64(&s.droppedPackets),
		"packets_skipped":      atomic.LoadInt64(&s.skippedPackets),
		"packets_valid":        atomic.LoadInt64(&s.validPackets),
		"http_requests":        atomic.LoadInt64(&s.requests),
		"http_responses":       atomic.LoadInt64(&s.responses),
		"connections_flushed":  atomic.LoadInt64(&s.connectionsFlushed),
		"connections_closed":   atomic.LoadInt64(&s.connectionsClosed),
		"timestamp_micro_secs": time.Now().UnixNano() / 1000,
	}
}

// Close terminates the HTTP traffic snooper as well as the underlying socket and the attached filter
func (s *HTTPSocketFilterSnooper) Close() {
	close(s.exit)
	s.wg.Wait()
	s.source.Close()
}

var _ HTTPTracker = &HTTPSocketFilterSnooper{}

func (s *HTTPSocketFilterSnooper) pollPackets(httpTimeout time.Duration) {
	streamFactory := &httpStreamFactory{
		statKeeper: s.statKeeper,
	}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	// Poll packets and feed them to the tcp assembler, which creates a new readerStream
	// and corresponding httpStreamHandler for each request
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		data, captureInfo, err := s.source.ZeroCopyReadPacketData()

		select {
		case <-s.exit:
			// Properly synchronize termination process
			// TODO send an EOF to all http streams in the streampool to shut them down
			return
		case <-ticker.C:
			// Every 30 seconds, flush old connections
			flushed, closed := assembler.FlushOlderThan(time.Now().Add(-1 * httpTimeout))
			atomic.AddInt64(&s.connectionsFlushed, int64(flushed))
			atomic.AddInt64(&s.connectionsClosed, int64(closed))
		default:
		}

		if err == nil {
			packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				atomic.AddInt64(&s.skippedPackets, 1)
				continue
			}
			atomic.AddInt64(&s.validPackets, 1)

			tcp := packet.TransportLayer().(*layers.TCP)
			srcIP, dstIP := packet.NetworkLayer().NetworkFlow().Endpoints()
			srcPort, dstPort := packet.TransportLayer().TransportFlow().Endpoints()

			key := httpKey{
				serverIP:   srcIP,
				clientIP:   dstIP,
				serverPort: srcPort,
				clientPort: dstPort,
			}

			inverseKey := httpKey{
				serverIP:   dstIP,
				clientIP:   srcIP,
				serverPort: dstPort,
				clientPort: srcPort,
			}

			if _, keyExists := s.statKeeper.requests[key]; !keyExists {
				if requestInfo, inverseKeyExists := s.statKeeper.requests[inverseKey]; inverseKeyExists {
					// We saw the "inverse key" prior to seeing this key; this packet is part of a response
					if captureInfo.Timestamp.Before(requestInfo.timeResponseRcvd) {
						requestInfo.timeResponseRcvd = captureInfo.Timestamp
						requestInfo.latency = requestInfo.timeResponseRcvd.Sub(requestInfo.timeSent)
						s.statKeeper.requests[inverseKey] = requestInfo

						atomic.AddInt64(&s.responses, 1)
						log.Infof("Received response from %v:%v -> %v:%v", srcIP, srcPort, dstIP, dstPort)
					}

					// Check if this packet contains the response code
					// TODO
					// For now we can't assemble responses, so we'll just skip this packet
					continue
				} else {
					// We have not seen this key or it's inverse; this packet is the first packet of a new request
					s.statKeeper.requests[key] = httpInfo{
						timeSent: captureInfo.Timestamp,
					}
					atomic.AddInt64(&s.requests, 1)

					log.Infof("Received request from %v:%v -> %v:%v", srcIP, srcPort, dstIP, dstPort)
				}
			}
			// If the key already exists in our map, we don't need to do anything except continuing assembling this request

			assembler.AssembleWithTimestamp(packet.NetworkLayer().NetworkFlow(), tcp, captureInfo.Timestamp)
			continue
		}

		// Immediately retry for EAGAIN
		if err == syscall.EAGAIN {
			continue
		}

		// Sleep briefly and try again
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *HTTPSocketFilterSnooper) pollStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var (
		prevPolls     int64
		prevProcessed int64
		prevCaptured  int64
		prevDropped   int64
	)

	for {
		select {
		case <-ticker.C:
			sourceStats, _ := s.source.Stats()
			_, socketStats, err := s.source.SocketStats()
			if err != nil {
				log.Errorf("error polling http socket stats: %s", err)
				continue
			}

			atomic.AddInt64(&s.socketPolls, sourceStats.Polls-prevPolls)
			atomic.AddInt64(&s.processedPackets, sourceStats.Packets-prevProcessed)
			atomic.AddInt64(&s.capturedPackets, int64(socketStats.Packets())-prevCaptured)
			atomic.AddInt64(&s.droppedPackets, int64(socketStats.Drops())-prevDropped)

			prevPolls = sourceStats.Polls
			prevProcessed = sourceStats.Packets
			prevCaptured = int64(socketStats.Packets())
			prevDropped = int64(socketStats.Drops())
		case <-s.exit:
			return
		}
	}
}
