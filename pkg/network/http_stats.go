// +build linux_bpf

package network

import (
	"bufio"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

type httpStatKeeper struct {
	requests map[httpKey]httpInfo

	// Telemetry
	requestsRead int64
	readErrors   int64
}

type httpKey struct {
	serverIP   gopacket.Endpoint
	clientIP   gopacket.Endpoint
	serverPort gopacket.Endpoint
	clientPort gopacket.Endpoint
}

type httpInfo struct {
	requestMethod    string
	bodyBytes        uint64
	timeSent         time.Time
	responseCode     string
	timeResponseRcvd time.Time
	latency          time.Duration
}

// httpStreamFactory implements tcpassembly.StreamFactory
type httpStreamFactory struct {
	statKeeper *httpStatKeeper
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	streamHandler := &httpStreamHandler{
		statKeeper:    h.statKeeper,
		netFlow:       net,
		transportFlow: transport,
		r:             tcpreader.NewReaderStream(), // implements tcpassembly.Stream
	}

	// Start reading immediately because not reading available bytes will block
	// TCP stream reassembly
	go streamHandler.read()

	return &streamHandler.r
}

type httpStreamHandler struct {
	statKeeper    *httpStatKeeper
	netFlow       gopacket.Flow
	transportFlow gopacket.Flow
	r             tcpreader.ReaderStream
}

func (h *httpStreamHandler) read() {
	buf := bufio.NewReader(&h.r)

	for {
		req, err := http.ReadRequest(buf)
		if err == io.EOF {
			return
		}

		atomic.AddInt64(&h.statKeeper.requestsRead, 1)

		if err != nil {
			log.Errorf("Error reading HTTP stream %v, %v : %v", h.netFlow, h.transportFlow, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
		} else {
			srcIP, dstIP := h.netFlow.Endpoints()
			srcPort, dstPort := h.transportFlow.Endpoints()

			bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
			req.Body.Close()

			key := httpKey{
				serverIP:   srcIP,
				clientIP:   dstIP,
				serverPort: srcPort,
				clientPort: dstPort,
			}

			info := h.statKeeper.requests[key]
			info.requestMethod = req.Method
			info.bodyBytes = uint64(bodyBytes)

			h.statKeeper.requests[key] = info

			log.Infof("Processed request from %v:%v -> %v:%v : method=%v, url=%v, body size=%v bytes",
				srcIP, srcPort, dstIP, dstPort, req.Method, req.URL, bodyBytes)
		}
	}
}
