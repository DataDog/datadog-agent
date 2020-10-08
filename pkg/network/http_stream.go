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
)

// TCP Assembly creates uni-directional streams - this means that http requests
// and responses on the same TCP connection would normally end up being processed
// by separate http stream handlers.
// There's 2 problems with this: 1), we can't easily match requests to responses in
// order to determine which requests were/weren't a success, and 2) we can't easily
// determine whether a given uni-directional stream is going to output http Response
// objects or http Request objects.
// To get around this and allow a single stream handler to process both requests and
// responses, we use the concept of an "http stream" which contain pairs of unidirectional
// streams. Under the assumption that we see requests before we see responses, we can thus
// identify which streams will be producing which types of http objects, and we can read
// requests and their corresponding responses "in order".

// TODO create a wrapper around the readerStream so that we can do pre-processing
// on bytes (ie to see their timestamps)

type httpStreamKey struct {
	net, transport gopacket.Flow
}

type httpStream struct {
	rawRequests  *tcpreader.ReaderStream
	rawResponses *tcpreader.ReaderStream
	requests     chan *http.Request
	responses    chan *http.Response
}

// httpStreamFactory implements tcpassembly.StreamFactory
type httpStreamFactory struct {
	statKeeper *httpStatKeeper
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	key := httpStreamKey{net, transport}

	if _, exists := h.statKeeper.streams[key]; !exists {
		reqStream := tcpreader.NewReaderStream()
		resStream := tcpreader.NewReaderStream()

		stream := &httpStream{
			rawRequests:  &reqStream,
			rawResponses: &resStream,
			requests:     make(chan *http.Request, 5),
			responses:    make(chan *http.Response, 5),
		}

		// Map the "reverse" key to this new stream so that when this function is called for
		// the other direction of this connection, it will find it
		reverseKey := httpStreamKey{net.Reverse(), transport.Reverse()}
		h.statKeeper.streams[reverseKey] = stream

		streamHandler := &httpStreamHandler{
			statKeeper:    h.statKeeper,
			streamKey:     key,
			stream:        stream,
			requestsDone:  int32(0),
			responsesDone: int32(0),
		}

		srcIP, dstIP := net.Endpoints()
		srcPt, dstPt := transport.Endpoints()
		streamStats := httpStreamStats{
			sourceIP:   srcIP,
			destIP:     dstIP,
			sourcePort: srcPt,
			destPort:   dstPt,
			requests:   make([]httpRequest, 0),
			responses:  make([]httpResponse, 0),
		}
		h.statKeeper.streamStats[key] = streamStats

		// Start reading requests ASAP
		go streamHandler.read()

		// We assume that the first time we see a new TCP connection, the first packet
		// processed will be a request packet; thus, this time we will return the "requests"
		// stream, and when we see the "reverse" flow we will return the "responses" stream
		return stream.rawRequests
	}

	return h.statKeeper.streams[key].rawResponses
}

type httpStreamHandler struct {
	statKeeper    *httpStatKeeper
	streamKey     httpStreamKey
	stream        *httpStream
	requestsDone  int32 // not thread-safe
	responsesDone int32 // not thread-safe
}

func (h *httpStreamHandler) read() {
	// Note: we need to read requests/responses from their stream buffers the moment bytes
	// become available in order to not block TCP stream reassembly
	go h.readRequests()
	go h.readResponses()

	for {
		// TODO add early exit

		if atomic.LoadInt32(&h.requestsDone) > 0 && atomic.LoadInt32(&h.responsesDone) > 0 {
			// TODO Remove connection from statKeeper
			return
		}

		select {
		case req := <-h.stream.requests:
			h.processRequest(req)
		case res := <-h.stream.responses:
			h.processResponse(res)
		}

		// TODO package request/response into 1 object along with duration & put THAT
		// in the streamStats
	}
}

func (h *httpStreamHandler) readRequests() {
	log.Infof("Reading requests for %v", h.streamKey.net)
	buf := bufio.NewReader(h.stream.rawRequests)
	for {
		// TODO add early exit

		req, err := http.ReadRequest(buf)

		if err == io.EOF {
			atomic.AddInt32(&h.requestsDone, 1)
			close(h.stream.requests)
			return
		}

		if err != nil {
			log.Errorf("Error reading HTTP request for %v : %v", h.streamKey.net, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
		}
		atomic.AddInt64(&h.statKeeper.messagesRead, 1)

		h.stream.requests <- req
	}
}

func (h *httpStreamHandler) readResponses() {
	// http.ReadResponse has been panicking with an "runtime error: slice bounds out of range"
	// I tried not calling readResponses() until the response stream was actually set up, but
	// this didn't seem to fix it... so this is a bandaid fix until I can figure out a real solution
	// could be related to https://github.com/golang/go/issues/22330
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("Panic caught while reading HTTP response for %v : %v", h.streamKey.net, err)
			// continue reading
			readResponses()
		}
	}()

	log.Infof("Reading responses for %v", h.streamKey.net)
	buf := bufio.NewReader(h.stream.rawResponses)
	for {
		// TODO add early exit

		res, err := http.ReadResponse(buf, nil) // TODO try to match corresponding request
		if err == io.EOF {
			atomic.AddInt32(&h.responsesDone, 1)
			close(h.stream.responses)
			return
		}

		if err != nil {
			log.Errorf("Error reading HTTP response for %v : %v", h.streamKey.net, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
		}
		atomic.AddInt64(&h.statKeeper.messagesRead, 1)

		h.stream.responses <- res
	}
}

func (h *httpStreamHandler) processRequest(req *http.Request) {
	bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
	req.Body.Close()

	stats := h.statKeeper.streamStats[h.streamKey]
	stats.requests = append(stats.requests, httpRequest{
		method:    req.Method,
		bodyBytes: bodyBytes,
	})
	h.statKeeper.streamStats[h.streamKey] = stats

	log.Infof("Processed request from %v:%v -> %v:%v : method=%v, url=%v, body size=%v bytes",
		stats.sourceIP, stats.sourcePort, stats.destIP, stats.destPort, req.Method, req.URL, bodyBytes)
}

func (h *httpStreamHandler) processResponse(res *http.Response) {
	bodyBytes := tcpreader.DiscardBytesToEOF(res.Body)
	res.Body.Close()

	stats := h.statKeeper.streamStats[h.streamKey]
	stats.responses = append(stats.responses, httpResponse{
		status:    res.Status,
		bodyBytes: bodyBytes,
	})
	if res.StatusCode == 200 {
		stats.successes += 1
	} else {
		stats.errors += 1
	}
	h.statKeeper.streamStats[h.streamKey] = stats

	log.Infof("Processed response from %v:%v -> %v:%v : status=%v, body size=%v bytes",
		stats.sourceIP, stats.sourcePort, stats.destIP, stats.destPort, res.Status, bodyBytes)
}
