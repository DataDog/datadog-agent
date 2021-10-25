package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	// defaultSampleSize is the default allocation size used to store
	// samples when a message contains multiple values. This will
	// automatically be extended if needed when we append to it.
	defaultSampleSize = 1024
)

type worker struct {
	server *Server
	// the batcher will be responsible of batching a few samples / events / service
	// checks and it will automatically forward them to the aggregator, meaning that
	// the flushing logic to the aggregator is actually in the batcher.
	batcher *batcher
	parser  *parser
	// we allocate it once per worker instead of once per packet. This will
	// be used to store the samples out a of packets. Allocating it every
	// time is very costly, especially on the GC.
	samples []metrics.MetricSample
}

func newWorker(s *Server) *worker {
	return &worker{
		server:  s,
		batcher: newBatcher(s.demultiplexer),
		parser:  newParser(s.sharedFloat64List),
		samples: make([]metrics.MetricSample, 0, defaultSampleSize),
	}
}

func (w *worker) flush() {
	w.batcher.flush()
}

func (w *worker) run() {
	for {
		select {
		case <-w.server.stopChan:
			return
		case <-w.server.health.C:
		case packets := <-w.server.packetsIn:
			w.samples = w.samples[0:0]
			// we return the samples in case the slice was extended
			// when parsing the packets
			w.samples = w.server.parsePackets(w.batcher, w.parser, packets, w.samples)
		}
	}
}
