// +build linux_bpf

package network

import (
	"bufio"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
	"github.com/pkg/errors"
)

// httpStreamFactory implements tcpassembly.StreamFactory
type httpStreamFactory struct {
	pktDirection packetDirection
	statKeeper   *httpStatKeeper
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	reader := tcpreader.NewReaderStream()
	streamHandler := &httpStreamHandler{
		pktDirection:  h.pktDirection,
		statKeeper:    h.statKeeper,
		netFlow:       net,
		transportFlow: transport,
		buf:           bufio.NewReader(&reader),
	}

	// Start reading immediately because not reading available bytes will block
	// TCP stream reassembly
	go streamHandler.read()

	return &reader
}

type httpStreamHandler struct {
	pktDirection  packetDirection
	statKeeper    *httpStatKeeper
	netFlow       gopacket.Flow
	transportFlow gopacket.Flow
	buf           *bufio.Reader
}

func (h *httpStreamHandler) read() {
	for {
		req, res, err := h.readNext()

		if err == io.EOF {
			return
		}

		if err != nil {
			log.Errorf("Error reading HTTP stream %v, %v : %v", h.netFlow, h.transportFlow, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
		}
		atomic.AddInt64(&h.statKeeper.messagesRead, 1)

		h.processRequest(req)  // noop if this message is a response
		h.processResponse(res) // noop if this message is a request
	}
}

func (h *httpStreamHandler) readNext() (*http.Request, *http.Response, error) {
	if h.pktDirection == request {
		req, err := http.ReadRequest(h.buf)
		return req, nil, err
	}
	if h.pktDirection == response {
		res, err := http.ReadResponse(h.buf, nil) // TODO try using previous request? it "should" be processed by this time...
		return nil, res, err
	}
	return nil, nil, errors.New("invalid packet direction")
}

func (h *httpStreamHandler) processRequest(req *http.Request) {
	if req == nil {
		return
	}

	srcIP, dstIP := h.netFlow.Endpoints()
	srcPort, dstPort := h.transportFlow.Endpoints()

	bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
	req.Body.Close()

	key := httpKey{
		sourceIP:   srcIP,
		destIP:     dstIP,
		sourcePort: srcPort,
		destPort:   dstPort,
	}

	h.statKeeper.mux.Lock()
	conn := h.statKeeper.connections[key]
	conn.requests = append(conn.requests, httpRequest{
		method:    req.Method,
		url:       req.URL,
		bodyBytes: bodyBytes,
	})
	h.statKeeper.connections[key] = conn
	h.statKeeper.mux.Unlock()

	log.Infof("Processed request from %v:%v -> %v:%v : method=%v, url=%v, body size=%v bytes",
		srcIP, srcPort, dstIP, dstPort, req.Method, req.URL, bodyBytes)
}

func (h *httpStreamHandler) processResponse(res *http.Response) {
	if res == nil {
		return
	}

	srcIP, dstIP := h.netFlow.Endpoints()
	srcPort, dstPort := h.transportFlow.Endpoints()

	key := httpKey{
		sourceIP:   dstIP,
		destIP:     srcIP,
		sourcePort: dstPort,
		destPort:   srcPort,
	}

	h.statKeeper.mux.Lock()
	conn := h.statKeeper.connections[key]
	conn.responses = append(conn.responses, httpResponse{status: res.Status})
	h.statKeeper.connections[key] = conn
	h.statKeeper.mux.Unlock()

	log.Infof("Processed response from %v:%v -> %v:%v : status=%v",
		srcIP, srcPort, dstIP, dstPort, res.Status)
}
