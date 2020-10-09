// +build linux_bpf

package network

import (
	"bufio"
	"container/heap"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
	"github.com/pkg/errors"
)

// httpStream implements tcpassembly.Stream. It acts as a wrapper around tcpreader.ReaderStream
// and allows us to intercept packet reassembly in order to capture timestamps
type httpStream struct {
	lastMsgTime time.Time
	reader      tcpreader.ReaderStream // also implements tcpassembly.Stream
}

// Reassembled handles reassembled TCP stream data
func (s *httpStream) Reassembled(rs []tcpassembly.Reassembly) {
	earliestByteSeen := time.Now()
	for _, r := range rs {
		if earliestByteSeen.After(r.Seen) {
			// Seen is the timestamp this set of bytes was pulled off the wire
			earliestByteSeen = r.Seen
		}
	}

	s.lastMsgTime = earliestByteSeen

	s.reader.Reassembled(rs)
}

// ReassemblyComplete marks this stream as finished
func (s *httpStream) ReassemblyComplete() {
	s.reader.ReassemblyComplete()
}

// httpStreamFactory implements tcpassembly.StreamFactory. Every time a packet with a new
// flow is encountered, the factory is called to create a new stream which data from
// this flow will be sent to once it's been re-assembled
type httpStreamFactory struct {
	statKeeper *httpStatKeeper
	events     map[httpKey](chan httpEvent)
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	stream := &httpStream{
		reader: tcpreader.NewReaderStream(),
	}

	key := httpKey{net, transport}

	streamHandler := &httpStreamHandler{
		statKeeper: h.statKeeper,
		stream:     stream,
	}
	defer func() {
		go streamHandler.readStreamData()
	}()

	// Check if we've already seen the "reverse" of this flow - if so, this flow is
	// the response side of a TCP connection for a request we've already seen
	reverseKey := httpKey{net.Reverse(), transport.Reverse()}
	if _, ok := h.statKeeper.stats.Load(reverseKey); ok {
		// The 2 streams which make up an HTTP connection (request direction & response
		// direction) share an an events channel as well as an object in the stats map
		streamHandler.key = reverseKey
		streamHandler.events = h.events[reverseKey]
		streamHandler.direction = response
		return stream
	}

	h.handleNewHTTPConnection(key)

	streamHandler.key = key
	streamHandler.events = h.events[key]
	streamHandler.direction = request
	return stream
}

func (h *httpStreamFactory) handleNewHTTPConnection(key httpKey) {
	srcIP, dstIP := key.net.Endpoints()
	srcPt, dstPt := key.transport.Endpoints()
	stats := httpStats{
		sourceIP:      srcIP,
		destIP:        dstIP,
		sourcePort:    srcPt,
		destPort:      dstPt,
		orderedEvents: &httpEventHeap{},
	}
	h.statKeeper.stats.Store(key, stats)

	eventChan := make(chan httpEvent, 10)
	h.events[key] = eventChan

	eventHandler := &httpEventHandler{
		statKeeper: h.statKeeper,
		key:        key,
		events:     eventChan,
	}
	go eventHandler.readHTTPEvents()
}

type flowDirection uint8

const (
	response = iota
	request
)

type httpStreamHandler struct {
	statKeeper *httpStatKeeper
	key        httpKey
	stream     *httpStream
	direction  flowDirection
	events     chan httpEvent
}

func (h *httpStreamHandler) readStreamData() {
	// Note: we need to read requests/responses from their stream buffers the moment bytes
	// become available in order to not block TCP stream reassembly
	buf := bufio.NewReader(&h.stream.reader)
	for {
		req, res, err := h.readNextMessage(buf)
		ts := h.stream.lastMsgTime // TODO check if this is a possible race condition

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			// srcIP, dstIP := h.key.net.Endpoints()
			// srcPrt, dstPrt := h.key.transport.Endpoints()
			// log.Errorf("Error reading HTTP stream %v:%v -> %v:%v : %v",
			// 	srcIP, dstIP, srcPrt, dstPrt, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
			continue
		}
		atomic.AddInt64(&h.statKeeper.messagesRead, 1)

		if req != nil {
			h.events <- h.processRequest(req, ts)
		}
		if res != nil {
			h.events <- h.processResponse(res, ts)
		}
	}
}

func (h *httpStreamHandler) readNextMessage(buf *bufio.Reader) (*http.Request, *http.Response, error) {
	if h.direction == request {
		req, err := http.ReadRequest(buf)
		return req, nil, err
	}
	if h.direction == response {
		res, err := http.ReadResponse(buf, nil)
		return nil, res, err
	}
	return nil, nil, errors.New("invalid packet direction")
}

func (h *httpStreamHandler) processRequest(req *http.Request, ts time.Time) httpEvent {
	bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
	req.Body.Close()

	request := &httpRequest{
		method:    req.Method,
		bodyBytes: bodyBytes,
		ts:        ts,
	}

	return request
}

func (h *httpStreamHandler) processResponse(res *http.Response, ts time.Time) httpEvent {
	bodyBytes := tcpreader.DiscardBytesToEOF(res.Body)
	res.Body.Close()

	response := &httpResponse{
		status:     res.Status,
		statusCode: res.StatusCode,
		bodyBytes:  bodyBytes,
		ts:         ts,
	}

	return response
}

// httpEventHandler reads requests and responses happening over a single HTTP connection. Since
// the responses & requests are being processed in different goroutines, this object handles updating
// the statKeeper stats map in order to avoid lock contention
type httpEventHandler struct {
	statKeeper *httpStatKeeper
	key        httpKey
	events     chan httpEvent
}

func (h *httpEventHandler) readHTTPEvents() {
	for {
		select {
		case e := <-h.events:
			value, _ := h.statKeeper.stats.Load(h.key)
			connStats, _ := value.(httpStats)

			heap.Push(connStats.orderedEvents, e)

			if _, ok := e.(*httpRequest); ok {
				connStats.numRequests += 1
			}

			if res, ok := e.(*httpResponse); ok {
				connStats.numResponses += 1
				if res.statusCode == 200 {
					connStats.successes += 1
				} else {
					connStats.errors += 1
				}
			}

			h.statKeeper.stats.Store(h.key, connStats)
		}
	}
}
