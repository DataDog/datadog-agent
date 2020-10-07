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

	exit chan struct{}
	wg   sync.WaitGroup

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

type packetWithTS struct {
	packet gopacket.Packet
	ts     time.Time
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
		connections: make(map[httpKey]httpConnection),
	}
	snooper := &HTTPSocketFilterSnooper{
		source:     packetSrc,
		statKeeper: statKeeper,
		exit:       make(chan struct{}),
	}

	// Create the tcp assemblers and the channels they'll read packets from
	reqStreamFactory := &httpStreamFactory{
		pktDirection: request,
		statKeeper:   statKeeper,
	}
	reqStreamPool := tcpassembly.NewStreamPool(reqStreamFactory)
	reqAssembler := tcpassembly.NewAssembler(reqStreamPool)
	reqPackets := make(chan packetWithTS, 100)

	resStreamFactory := &httpStreamFactory{
		pktDirection: response,
		statKeeper:   statKeeper,
	}
	resStreamPool := tcpassembly.NewStreamPool(resStreamFactory)
	resAssembler := tcpassembly.NewAssembler(resStreamPool)
	resPackets := make(chan packetWithTS, 100)

	// Get ready to assemble packets
	snooper.wg.Add(1)
	go func() {
		snooper.assemblePackets(reqPackets, reqAssembler, httpTimeout)
		snooper.wg.Done()
	}()

	snooper.wg.Add(1)
	go func() {
		snooper.assemblePackets(resPackets, resAssembler, httpTimeout)
		snooper.wg.Done()
	}()

	// Start consuming packets
	snooper.wg.Add(1)
	go func() {
		snooper.pollPackets(reqPackets, resPackets)
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

func (s *HTTPSocketFilterSnooper) GetHTTPConnections() map[httpKey]httpConnection {
	return s.statKeeper.connections
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

		// TODO add statkeeper telemetry
	}
}

// Close terminates the HTTP traffic snooper as well as the underlying socket and the attached filter
func (s *HTTPSocketFilterSnooper) Close() {
	close(s.exit)

	// TODO close the packet channels?
	// TODO send an EOF to all http streams in the streampools to shut them down?

	s.wg.Wait()
	s.source.Close()
}

var _ HTTPTracker = &HTTPSocketFilterSnooper{}

func (s *HTTPSocketFilterSnooper) pollPackets(reqPackets chan packetWithTS, resPackets chan packetWithTS) {
	for {
		select {
		case <-s.exit:
			return
		default:
		}

		data, captureInfo, err := s.source.ZeroCopyReadPacketData()
		if err == nil {
			packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				atomic.AddInt64(&s.skippedPackets, 1)
				continue
			}
			atomic.AddInt64(&s.validPackets, 1)

			direction := s.determineDirection(packet)
			ts := captureInfo.Timestamp

			// TODO: latency calculation
			// key := HTTPKey.New(packet)
			// if lastDirection was diff that what we currently see, then we can add timestamps,
			// calculate latency, and increase response/request counts
			// NOTE: this logic fails if we see request/response packets out of order (though this
			// should never happen)

			if direction == request {
				reqPackets <- packetWithTS{packet, ts}
			} else if direction == response {
				resPackets <- packetWithTS{packet, ts}
			}
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

func (s *HTTPSocketFilterSnooper) assemblePackets(ch chan packetWithTS, assembler *tcpassembly.Assembler, timeout time.Duration) {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {

		case <-s.exit:
			return

		case <-ticker.C:
			// Every 30 seconds, flush old connections
			flushed, closed := assembler.FlushOlderThan(time.Now().Add(-1 * timeout))
			atomic.AddInt64(&s.connectionsFlushed, int64(flushed))
			atomic.AddInt64(&s.connectionsClosed, int64(closed))

		case p := <-ch:
			tcp := p.packet.TransportLayer().(*layers.TCP)
			assembler.AssembleWithTimestamp(p.packet.NetworkLayer().NetworkFlow(), tcp, p.ts)
		}
	}
}

func (s *HTTPSocketFilterSnooper) determineDirection(packet gopacket.Packet) packetDirection {
	key := newKey(packet)
	if _, exists := s.statKeeper.connections[key]; exists {
		return request
	}

	inverseKey := httpKey{
		sourceIP:   key.destIP,
		destIP:     key.sourceIP,
		sourcePort: key.destPort,
		destPort:   key.sourcePort,
	}
	if _, exists := s.statKeeper.connections[inverseKey]; exists {
		return response
	}

	// Never seen this connection before -> assume it's a new connection beginning with a request
	s.statKeeper.connections[key] = httpConnection{}
	return request
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
