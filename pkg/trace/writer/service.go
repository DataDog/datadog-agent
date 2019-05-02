package writer

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const pathServices = "/api/v0.2/services"

// ServiceWriter ingests service metadata and flush them to the API.
type ServiceWriter struct {
	stats      info.ServiceWriterInfo
	conf       writerconfig.ServiceWriterConfig
	InServices <-chan pb.ServicesMetadata

	serviceBuffer pb.ServicesMetadata

	sender payloadSender
	exit   chan struct{}
}

// NewServiceWriter returns a new writer for services.
func NewServiceWriter(conf *config.AgentConfig, InServices <-chan pb.ServicesMetadata) *ServiceWriter {
	cfg := conf.ServiceWriterConfig
	endpoints := newEndpoints(conf, pathServices)
	sender := newMultiSender(endpoints, cfg.SenderConfig)
	log.Infof("Service writer initializing with config: %+v", cfg)

	return &ServiceWriter{
		conf:          cfg,
		InServices:    InServices,
		serviceBuffer: pb.ServicesMetadata{},
		sender:        sender,
		exit:          make(chan struct{}),
	}
}

// Start starts the writer.
func (w *ServiceWriter) Start() {
	w.sender.Start()
	go func() {
		defer watchdog.LogOnPanic()
		w.Run()
	}()
}

// Run runs the main loop of the writer goroutine. If buffers
// services read from input chan and flushes them when necessary.
func (w *ServiceWriter) Run() {
	defer close(w.exit)

	// for now, simply flush every x seconds
	flushTicker := time.NewTicker(w.conf.FlushPeriod)
	defer flushTicker.Stop()

	updateInfoTicker := time.NewTicker(w.conf.UpdateInfoPeriod)
	defer updateInfoTicker.Stop()

	log.Debug("Starting service writer")

	// Monitor sender for events
	go func() {
		for event := range w.sender.Monitor() {
			switch event.typ {
			case eventTypeSuccess:
				url := event.stats.host
				log.Debugf("Flushed service payload; url:%s, time:%s, size:%d bytes", url, event.stats.sendTime,
					len(event.payload.bytes))
				tags := []string{"url:" + url}
				metrics.Gauge("datadog.trace_agent.service_writer.flush_duration",
					event.stats.sendTime.Seconds(), tags, 1)
				atomic.AddInt64(&w.stats.Payloads, 1)
			case eventTypeFailure:
				url := event.stats.host
				log.Errorf("Failed to flush service payload; url:%s, time:%s, size:%d bytes, error: %s",
					url, event.stats.sendTime, len(event.payload.bytes), event.err)
				atomic.AddInt64(&w.stats.Errors, 1)
			case eventTypeRetry:
				log.Errorf("Retrying flush service payload, retryNum: %d, delay:%s, error: %s",
					event.retryNum, event.retryDelay, event.err)
				atomic.AddInt64(&w.stats.Retries, 1)
			default:
				log.Debugf("Unable to handle event with type %T", event)
			}
		}
	}()

	// Main loop
	for {
		select {
		case sm := <-w.InServices:
			w.handleServiceMetadata(sm)
		case <-flushTicker.C:
			w.flush()
		case <-updateInfoTicker.C:
			go w.updateInfo()
		case <-w.exit:
			log.Info("Exiting service writer, flushing all modified services")
			w.flush()
			return
		}
	}
}

// Stop stops the main Run loop.
func (w *ServiceWriter) Stop() {
	w.exit <- struct{}{}
	<-w.exit
	w.sender.Stop()
}

func (w *ServiceWriter) handleServiceMetadata(metadata pb.ServicesMetadata) {
	for k, v := range metadata {
		w.serviceBuffer[k] = v
	}
}

func (w *ServiceWriter) flush() {
	// If no services, we can't construct anything
	if len(w.serviceBuffer) == 0 {
		return
	}

	numServices := len(w.serviceBuffer)
	log.Debugf("Going to flush updated service metadata, %d services", numServices)
	atomic.StoreInt64(&w.stats.Services, int64(numServices))

	data, err := json.Marshal(w.serviceBuffer)
	if err != nil {
		log.Errorf("Error while encoding service payload: %v", err)
		w.serviceBuffer = make(pb.ServicesMetadata)
		return
	}

	headers := map[string]string{
		languageHeaderKey: strings.Join(info.Languages(), "|"),
		"Content-Type":    "application/json",
	}

	atomic.AddInt64(&w.stats.Bytes, int64(len(data)))

	payload := newPayload(data, headers)
	w.sender.Send(payload)

	w.serviceBuffer = make(pb.ServicesMetadata)
}

func (w *ServiceWriter) updateInfo() {
	var swInfo info.ServiceWriterInfo

	// Load counters and reset them for the next flush
	swInfo.Payloads = atomic.SwapInt64(&w.stats.Payloads, 0)
	swInfo.Services = atomic.SwapInt64(&w.stats.Services, 0)
	swInfo.Bytes = atomic.SwapInt64(&w.stats.Bytes, 0)
	swInfo.Errors = atomic.SwapInt64(&w.stats.Errors, 0)
	swInfo.Retries = atomic.SwapInt64(&w.stats.Retries, 0)

	// TODO(gbbr): Scope these stats per endpoint (see (config.AgentConfig).AdditionalEndpoints))
	metrics.Count("datadog.trace_agent.service_writer.payloads", int64(swInfo.Payloads), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.services", int64(swInfo.Services), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.bytes", int64(swInfo.Bytes), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.retries", int64(swInfo.Retries), nil, 1)
	metrics.Count("datadog.trace_agent.service_writer.errors", int64(swInfo.Errors), nil, 1)

	info.UpdateServiceWriterInfo(swInfo)
}
