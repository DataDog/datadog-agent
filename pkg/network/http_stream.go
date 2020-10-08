// +build linux_bpf

package network

import (
	"bufio"
	"container/heap"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
	"github.com/pkg/errors"
)

// httpStream implements tcpassembly.Stream. It acts as a wrapper around tcpreader.ReaderStream
// which allows us to do some pre-processing on packets
type httpStream struct {
	lastMsgTime time.Time
	reader      tcpreader.ReaderStream // also implements tcpassembly.Stream
}

// Reassembled handles reassembled TCP stream data.
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

// ReassemblyComplete marks this stream as finished.
func (s *httpStream) ReassemblyComplete() {
	s.reader.ReassemblyComplete()
}

// httpStreamFactory implements tcpassembly.StreamFactory
type httpStreamFactory struct {
	statKeeper *httpStatKeeper
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	stream := &httpStream{
		reader: tcpreader.NewReaderStream(),
	}

	key := httpKey{net, transport}

	streamHandler := &httpStreamHandler{
		statKeeper: h.statKeeper,
		key:        key,
		stream:     stream,
	}
	defer func() {
		go streamHandler.read()
	}()

	// Check if we've already seen the "reverse" of this flow - if so, this stream is
	// the response side of a TCP connection for a request we've already seen
	reverseKey := httpKey{net.Reverse(), transport.Reverse()}
	if _, exists := h.statKeeper.stats[reverseKey]; exists {
		streamHandler.key = reverseKey
		streamHandler.dir = responseDir

		return stream
	}

	streamHandler.dir = requestDir

	srcIP, dstIP := net.Endpoints()
	srcPt, dstPt := transport.Endpoints()
	stats := httpStats{
		sourceIP:      srcIP,
		destIP:        dstIP,
		sourcePort:    srcPt,
		destPort:      dstPt,
		orderedEvents: &httpEventHeap{},
	}
	h.statKeeper.stats[key] = stats
	h.statKeeper.muxMap[key] = &sync.Mutex{}

	return stream
}

type flowDirection uint8

const (
	responseDir = iota
	requestDir
)

type httpStreamHandler struct {
	statKeeper *httpStatKeeper
	key        httpKey
	stream     *httpStream
	dir        flowDirection
}

func (h *httpStreamHandler) read() {
	// Note: we need to read requests/responses from their stream buffers the moment bytes
	// become available in order to not block TCP stream reassembly
	buf := bufio.NewReader(&h.stream.reader)
	for {
		req, res, err := h.readNextMessage(buf)
		ts := h.stream.lastMsgTime // possible race condition?

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			srcIP, dstIP := h.key.net.Endpoints()
			srcPrt, dstPrt := h.key.transport.Endpoints()
			log.Errorf("Error reading HTTP stream %v:%v -> %v:%v : %v",
				srcIP, dstIP, srcPrt, dstPrt, err)
			atomic.AddInt64(&h.statKeeper.readErrors, 1)
			continue
		}
		atomic.AddInt64(&h.statKeeper.messagesRead, 1)

		h.processRequest(req, ts)  // noop if this message is a response
		h.processResponse(res, ts) // noop if this message is a request
	}
}

func (h *httpStreamHandler) readNextMessage(buf *bufio.Reader) (*http.Request, *http.Response, error) {
	if h.dir == requestDir {
		req, err := http.ReadRequest(buf)
		return req, nil, err
	}
	if h.dir == responseDir {
		res, err := http.ReadResponse(buf, nil)
		return nil, res, err
	}
	return nil, nil, errors.New("invalid packet direction")
}

func (h *httpStreamHandler) processRequest(req *http.Request, ts time.Time) {
	if req == nil {
		return
	}

	bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
	req.Body.Close()

	request := &httpRequest{
		method:    req.Method,
		bodyBytes: bodyBytes,
		ts:        ts,
	}

	h.statKeeper.muxMap[h.key].Lock()
	defer h.statKeeper.muxMap[h.key].Unlock()

	stats := h.statKeeper.stats[h.key]
	heap.Push(stats.orderedEvents, request)
	stats.numRequests += 1
	h.statKeeper.stats[h.key] = stats

	// log.Infof("Processed request from %v:%v -> %v:%v : method=%v, url=%v, body size=%v bytes",
	// 	stats.sourceIP, stats.sourcePort, stats.destIP, stats.destPort, req.Method, req.URL, bodyBytes)
}

func (h *httpStreamHandler) processResponse(res *http.Response, ts time.Time) {
	if res == nil {
		return
	}

	bodyBytes := tcpreader.DiscardBytesToEOF(res.Body)
	res.Body.Close()

	response := &httpResponse{
		status:    res.Status,
		bodyBytes: bodyBytes,
		ts:        ts,
	}

	h.statKeeper.muxMap[h.key].Lock()
	defer h.statKeeper.muxMap[h.key].Unlock()

	stats := h.statKeeper.stats[h.key]
	heap.Push(stats.orderedEvents, response)
	stats.numResponses += 1
	if res.StatusCode == 200 {
		stats.successes += 1
	} else {
		stats.errors += 1
	}
	h.statKeeper.stats[h.key] = stats

	// log.Infof("Processed response from %v:%v -> %v:%v : status=%v, body size=%v bytes",
	// 	stats.sourceIP, stats.sourcePort, stats.destIP, stats.destPort, res.Status, bodyBytes)
}
