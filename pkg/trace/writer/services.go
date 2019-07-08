package writer

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// pathServices is the target host API path for delivering services.
const pathServices = "/api/v0.2/services"

// ServiceWriter aggregates incoming services and flushes them to the Datadog API.
type ServiceWriter struct {
	in      <-chan pb.ServicesMetadata
	stats   *info.ServiceWriterInfo
	buffer  pb.ServicesMetadata
	senders []*sender
	stop    chan struct{}
}

// NewServiceWriter returns a new, ready to use ServiceWriter. Run must be called before
// sending metadata down the channel.
func NewServiceWriter(cfg *config.AgentConfig, in chan pb.ServicesMetadata) *ServiceWriter {
	sw := &ServiceWriter{
		in:     in,
		stats:  &info.ServiceWriterInfo{},
		buffer: make(pb.ServicesMetadata),
		stop:   make(chan struct{}),
	}
	// The services endpoint is being deprecated and is very low volume.
	// It should never reach more than one concurrent connection, but let's
	// allow it to have at least two and a small queue, which will likely never
	// grow.
	climit := 2
	qsize := 10
	sw.senders = newSenders(cfg, sw, pathServices, climit, qsize)
	return sw
}

// Run runs the service writer, awaiting input.
func (w *ServiceWriter) Run() {
	t := time.NewTicker(5 * time.Second)
	defer close(w.stop)
	defer t.Stop()
	for {
		select {
		case md := <-w.in:
			for k, v := range md {
				w.buffer[k] = v
			}
		case <-t.C:
			w.report()
			w.flush()
		case <-w.stop:
			log.Info("Exiting service writer, flushing all modified services.")
			w.flush()
			return
		}
	}
}

func (w *ServiceWriter) resetBuffer() {
	for k := range w.buffer {
		delete(w.buffer, k)
	}
}

func (w *ServiceWriter) flush() {
	if len(w.buffer) == 0 {
		// nothing to flush
		return
	}
	n := len(w.buffer)
	log.Debugf("Flushing %d updated services.", n)
	atomic.StoreInt64(&w.stats.Services, int64(n))

	p := newPayload(map[string]string{
		headerLanguages: strings.Join(info.Languages(), "|"),
		"Content-Type":  "application/json",
	})
	defer w.resetBuffer()
	if err := json.NewEncoder(p.body).Encode(w.buffer); err != nil {
		log.Errorf("Error encoding services payload: %v", err)
		return
	}
	atomic.AddInt64(&w.stats.Bytes, int64(p.body.Len()))
	for _, sender := range w.senders {
		sender.Push(p)
	}

}

// Stop attempts to gracefully stop the service writer.
func (w *ServiceWriter) Stop() {
	w.stop <- struct{}{}
	<-w.stop
	stopSenders(w.senders)
}

func (w *ServiceWriter) report() {
	metrics.Count("datadog.trace_agent.service_writer.payloads", atomic.SwapInt64(&w.stats.Payloads, 0), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.services", atomic.SwapInt64(&w.stats.Services, 0), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.bytes", atomic.SwapInt64(&w.stats.Bytes, 0), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.retries", atomic.SwapInt64(&w.stats.Retries, 0), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.errors", atomic.SwapInt64(&w.stats.Errors, 0), nil, 1)
}

var _ eventRecorder = (*ServiceWriter)(nil)

// recordEvent implements eventRecorder.
func (w *ServiceWriter) recordEvent(t eventType, data *eventData) {
	switch t {
	case eventTypeRetry:
		log.Errorf("Retrying to flush services payload; error: %s", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeSent:
		log.Debugf("Flushed services to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.service_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeRejected:
		log.Warnf("Service writer payload rejected by edge: %v", data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		log.Warnf("Service writer queue full. Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		metrics.Count("datadog.trace_agent.service_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.service_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}
