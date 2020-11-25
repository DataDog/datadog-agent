package dogstatsd

type worker struct {
	server *Server
	// the batcher will be responsible of batching a few samples / events / service
	// checks and it will automatically forward them to the aggregator, meaning that
	// the flushing logic to the aggregator is actually in the batcher.
	batcher *batcher
	parser  *parser
}

func newWorker(s *Server) *worker {
	return &worker{
		server:  s,
		batcher: newBatcher(s.aggregator),
		parser:  newParser(),
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
			w.server.parsePackets(w.batcher, w.parser, packets)
		}
	}
}
