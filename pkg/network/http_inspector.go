// +build linux_bpf

package network

import (
	"sort"
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

// HTTPSocketFilterInspector is a HTTP traffic inspector built on top of an eBPF SOCKET_FILTER
type HTTPSocketFilterInspector struct {
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

// NewHTTPSocketFilterInspector returns a new HTTPSocketFilterInspector
func NewHTTPSocketFilterInspector(rootPath string, filter *manager.Probe, httpTimeout time.Duration) (*HTTPSocketFilterInspector, error) {
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
		stats: &sync.Map{},
	}
	inspector := &HTTPSocketFilterInspector{
		source:     packetSrc,
		statKeeper: statKeeper,
		exit:       make(chan struct{}),
	}

	// Start consuming packets
	inspector.wg.Add(1)
	go func() {
		inspector.pollPackets(httpTimeout)
		inspector.wg.Done()
	}()

	// Start polling socket stats
	inspector.wg.Add(1)
	go func() {
		inspector.pollStats()
		inspector.wg.Done()
	}()

	return inspector, nil
}

func (s *HTTPSocketFilterInspector) GetHTTPConnections() map[httpKey]httpStats {
	// TODO
	return nil
}

func (s *HTTPSocketFilterInspector) GetStats() map[string]int64 {
	return map[string]int64{
		"socket_polls":         atomic.LoadInt64(&s.socketPolls),
		"packets_processed":    atomic.LoadInt64(&s.processedPackets),
		"packets_captured":     atomic.LoadInt64(&s.capturedPackets),
		"packets_dropped":      atomic.LoadInt64(&s.droppedPackets),
		"packets_skipped":      atomic.LoadInt64(&s.skippedPackets),
		"packets_valid":        atomic.LoadInt64(&s.validPackets),
		"connections_flushed":  atomic.LoadInt64(&s.connectionsFlushed),
		"connections_closed":   atomic.LoadInt64(&s.connectionsClosed),
		"http_messages_read":   atomic.LoadInt64(&s.statKeeper.messagesRead),
		"http_read_errors":     atomic.LoadInt64(&s.statKeeper.readErrors),
		"timestamp_micro_secs": time.Now().UnixNano() / 1000,
	}
}

// Close terminates the HTTP traffic inspector as well as the underlying socket and the attached filter
func (s *HTTPSocketFilterInspector) Close() {
	close(s.exit)
	// TODO close the http stream handlers (send an EOF msg to all streams in the stream pool) &
	// close the http event handlers (close all the events channels in the stream factory)
	s.wg.Wait()
	s.source.Close()
}

var _ HTTPTracker = &HTTPSocketFilterInspector{}

func (s *HTTPSocketFilterInspector) pollPackets(httpTimeout time.Duration) {
	streamFactory := &httpStreamFactory{
		statKeeper: s.statKeeper,
		events:     make(map[httpKey](chan httpEvent)),
	}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	// Note: as an optimization, we could have multiple assemblers working on the same
	// stream pool. For an even better optimization, we could have multiple assemblers reading
	// from multiple stream pools - but in this case you must be able to guarantee that packets
	// from the same connection will end up being handled by assemblers in the same pool (ie
	// by hashing the packets)

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {
		case <-s.exit:
			return

		case <-ticker.C:
			// Every 30 seconds, flush old connections
			expiration := time.Now().Add(-1 * httpTimeout)
			flushed, closed := assembler.FlushOlderThan(expiration)

			// TODO remove closed connections from the stats map and close their events channels

			atomic.AddInt64(&s.connectionsFlushed, int64(flushed))
			atomic.AddInt64(&s.connectionsClosed, int64(closed))
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

			tcp := packet.TransportLayer().(*layers.TCP)
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

func (s *HTTPSocketFilterInspector) pollStats() {
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

func (s *HTTPSocketFilterInspector) PrintStats() {
	stats := s.GetStats()

	// sort keys so we always print in a stable order
	var keys []string
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	log.Infof("HTTP Telemetry:")
	for _, k := range keys {
		log.Infof("  %v, %v", k, stats[k])
	}
}

func (s *HTTPSocketFilterInspector) PrintConnections() {
	var connCount, reqCount, resCount, errCount, successCount int64
	var avgLatencies []time.Duration
	s.statKeeper.stats.Range(func(key, value interface{}) bool {
		conn, ok := value.(httpStats)
		if !ok {
			log.Infof("  %v Invalid connection stats object found for key %v ", key)
			return true
		}

		latencies := conn.getLatencies()
		avgLatency := avg(latencies)

		// log.Infof("  %v:%v -> %v:%v \t %v requests, %v responses, %v errors, %v successes, %v avg latency",
		// 	conn.sourceIP, conn.sourcePort, conn.destIP, conn.destPort,
		// 	conn.numRequests, conn.numResponses, conn.errors, conn.successes, avgLatency)

		connCount++
		reqCount += conn.numRequests
		resCount += conn.numResponses
		errCount += conn.errors
		successCount += conn.successes
		avgLatencies = append(avgLatencies, avgLatency)

		return true
	})
	log.Infof("%v active connections: %v requests, %v responses, %v errors, %v successes, %v average request latency",
		connCount, reqCount, resCount, errCount, successCount, avg(avgLatencies))
}

func avg(arr []time.Duration) time.Duration {
	if len(arr) == 0 {
		return 0
	}

	total := int64(0)
	for _, v := range arr {
		total += v.Microseconds()
	}

	return time.Duration(total/int64(len(arr))) * time.Microsecond
}
