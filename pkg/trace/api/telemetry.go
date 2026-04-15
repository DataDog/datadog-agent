// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"

	msgpack "github.com/vmihailenco/msgpack/v4"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const originTag = "origin"

type cloudResourceType string
type cloudProvider string

const (
	awsFargate                    cloudResourceType = "AWSFargate"
	cloudRun                      cloudResourceType = "GCPCloudRun"
	cloudFunctions                cloudResourceType = "GCPCloudFunctions"
	azureAppService               cloudResourceType = "AzureAppService"
	azureContainerApp             cloudResourceType = "AzureContainerApp"
	aws                           cloudProvider     = "AWS"
	gcp                           cloudProvider     = "GCP"
	azure                         cloudProvider     = "Azure"
	cloudProviderHeader           string            = "Dd-Cloud-Provider"
	cloudResourceTypeHeader       string            = "Dd-Cloud-Resource-Type"
	cloudResourceIdentifierHeader string            = "Dd-Cloud-Resource-Identifier"
)

const (
	// This number was chosen because requests on the EVP are accepted with sizes up to 5Mb, so we
	// want to be able to buffer at least a few max size requests before exerting backpressure.
	//
	// And using 20Mb at most per host seems not too unreasonnable.
	//
	// Looking at payload size distribution, the max requests we get is about than 1Mb anyway,
	// the biggest p99 per language is around 350Kb for nodejs and p95 is around 13Kb.
	// So it should provide enough in normal cases before we start dropping requests.
	maxInflightBytes = 20_000_000

	// Maximum number of concurrent requests to the intake
	//
	// Since we have at most 20MB of data in flight, and payloads are 4MB
	// at most, this allows for 4 * 4MB = 16MB of data being sent and 4 more MB in the
	// batch buffer
	maxConcurrentRequests = 4

	// defaultBatchSizeThreshold is the cumulative body byte size that triggers an immediate flush.
	defaultBatchSizeThreshold = 4_000_000

	// batchMaxAge is the maximum time the oldest event can sit in the buffer before triggering a flush.
	batchMaxAge = 30 * time.Second

	// batchCheckInterval is how often the flusher goroutine checks for age-based flushes.
	batchCheckInterval = 100 * time.Millisecond

	// batchURLPath is the upstream endpoint path for batched telemetry.
	batchURLPath = "/api/v2/apmtelemetry"
)

// agentBatch is the top-level envelope for batched telemetry payloads, serialized as MessagePack.
type agentBatch struct {
	RequestType string             `msgpack:"request_type"`
	Payload     rawTelemetryEvents `msgpack:"payload"`
}

type rawTelemetryEvents struct {
	Events []rawTelemetryEvent `msgpack:"events"`
}

type rawTelemetryEvent struct {
	Headers map[string]string `msgpack:"headers"`
	Content []byte            `msgpack:"content"`
}

type forwardedBatch struct {
	body          []byte // serialized MessagePack
	totalBodySize int    // sum of original event body sizes, for inflightCount tracking
}

type currentBatch struct {
	mu                 sync.Mutex
	bufferedEvents     []rawTelemetryEvent
	bufferedSize       int
	oldestEventTime    time.Time
	batchSizeThreshold int
	nowFn              func() time.Time // injectable for testing, defaults to time.Now
}

func (b *currentBatch) addEvent(ev rawTelemetryEvent) (batch []rawTelemetryEvent, totalBodySize int, shouldFlush bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.bufferedEvents) == 0 {
		b.oldestEventTime = b.nowFn()
	}
	b.bufferedSize += len(ev.Content)
	b.bufferedEvents = append(b.bufferedEvents, ev)
	shouldFlush = b.bufferedSize >= b.batchSizeThreshold
	if shouldFlush {
		batch, totalBodySize = b.takeBatchLocked()
	}
	return batch, totalBodySize, shouldFlush
}
func (b *currentBatch) checkFlushAge() (batch []rawTelemetryEvent, totalBodySize int, shouldFlush bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	shouldFlush = len(b.bufferedEvents) > 0 &&
		!b.oldestEventTime.IsZero() &&
		b.nowFn().Sub(b.oldestEventTime) >= batchMaxAge

	if shouldFlush {
		batch, totalBodySize = b.takeBatchLocked()
	}
	return batch, totalBodySize, shouldFlush
}

// takeBatchLocked extracts all buffered events and resets the buffer.
// Must be called while holding f.mu.
func (b *currentBatch) takeBatchLocked() ([]rawTelemetryEvent, int) {
	events := b.bufferedEvents
	b.bufferedEvents = make([]rawTelemetryEvent, 0, len(events))

	size := b.bufferedSize
	b.bufferedSize = 0

	b.oldestEventTime = time.Time{}
	return events, size
}

func (b *currentBatch) takeBatch() ([]rawTelemetryEvent, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.takeBatchLocked()
}

// TelemetryForwarder batches incoming telemetry HTTP requests and forwards them
// as MessagePack agent-batch payloads to configured intake endpoints.
type TelemetryForwarder struct {
	endpoints []*config.Endpoint
	conf      *config.AgentConfig

	batch currentBatch

	// flushChan carries size-triggered batches from the HTTP handler to the batchFlusher.
	flushChan chan []rawTelemetryEvent
	// forwardChan carries serialized payloads from the batchFlusher to forwarder workers.
	forwardChan chan forwardedBatch

	inflightWaiter   sync.WaitGroup
	inflightCount    atomic.Int64
	maxInflightBytes int64

	cancelCtx context.Context
	cancelFn  context.CancelFunc
	done      chan struct{}

	containerIDProvider IDProvider
	client              *config.ResetClient
	statsd              statsd.ClientInterface
	logger              *log.ThrottledLogger
}

// NewTelemetryForwarder creates a new TelemetryForwarder
func NewTelemetryForwarder(conf *config.AgentConfig, containerIDProvider IDProvider, statsd statsd.ClientInterface) *TelemetryForwarder {
	// extract and validate Hostnames from configured endpoints
	var endpoints []*config.Endpoint
	if conf.TelemetryConfig != nil {
		for _, endpoint := range conf.TelemetryConfig.Endpoints {
			u, err := url.Parse(endpoint.Host)
			if err != nil {
				log.Errorf("Error parsing apm_config.telemetry endpoint %q: %v", endpoint.Host, err)
				continue
			}
			if u.Host != "" {
				endpoint.Host = u.Host
			}

			endpoints = append(endpoints, endpoint)
		}
	}

	cancelCtx, cancelFn := context.WithCancel(context.Background())

	forwarder := &TelemetryForwarder{
		endpoints: endpoints,
		conf:      conf,
		batch: currentBatch{
			bufferedEvents:     make([]rawTelemetryEvent, 0, 64),
			batchSizeThreshold: defaultBatchSizeThreshold,
			nowFn:              time.Now,
		},
		flushChan:        make(chan []rawTelemetryEvent, 1),
		forwardChan:      make(chan forwardedBatch),
		inflightWaiter:   sync.WaitGroup{},
		inflightCount:    atomic.Int64{},
		maxInflightBytes: maxInflightBytes,

		cancelCtx: cancelCtx,
		cancelFn:  cancelFn,
		done:      make(chan struct{}),

		containerIDProvider: containerIDProvider,
		client:              conf.NewHTTPClient(),
		statsd:              statsd,
		logger:              log.NewThrottled(5, 10*time.Second),
	}
	return forwarder
}

func (f *TelemetryForwarder) start() {
	for i := 0; i < maxConcurrentRequests; i++ {
		f.inflightWaiter.Add(1)
		go func() {
			defer f.inflightWaiter.Done()
			for batch := range f.forwardChan {
				f.forwardBatchToEndpoints(batch)
			}
		}()
	}
	f.inflightWaiter.Add(1)
	go f.batchFlusher()
}

// Stop waits for up to 1s to end all telemetry forwarded requests.
func (f *TelemetryForwarder) Stop() {
	close(f.done)
	done := make(chan any)
	go func() {
		f.inflightWaiter.Wait()
		close(done)
	}()
	select {
	case <-done:
	// Give a max 1s timeout to wait for all requests to end
	case <-time.After(1 * time.Second):
	}
	f.cancelFn()
}

func (f *TelemetryForwarder) startRequest(size int64) (accepted bool) {
	for {
		inflight := f.inflightCount.Load()
		newInflight := inflight + size
		if newInflight > f.maxInflightBytes {
			return false
		}
		if f.inflightCount.CompareAndSwap(inflight, newInflight) {
			return true
		}
	}
}

// telemetryForwarderHandler returns a new HTTP handler which buffers incoming telemetry
// requests and batches them for forwarding to the configured intakes.
func (r *HTTPReceiver) telemetryForwarderHandler() http.Handler {
	if len(r.telemetryForwarder.endpoints) == 0 {
		log.Error("None of the configured apm_config.telemetry endpoints are valid. Telemetry proxy is off")
		return http.NotFoundHandler()
	}

	forwarder := r.telemetryForwarder
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read at most maxInflightBytes since we're going to throw out the result anyway if it's bigger
		body, err := io.ReadAll(io.LimitReader(r.Body, forwarder.maxInflightBytes+1))
		if err != nil {
			writeEmptyJSON(w, http.StatusInternalServerError)
			return
		}

		if accepted := forwarder.startRequest(int64(len(body))); !accepted {
			writeEmptyJSON(w, http.StatusTooManyRequests)
			return
		}

		eventHeaders := forwarder.buildEventHeaders(r)
		batch, totalBodySize, shouldFlush := forwarder.batch.addEvent(rawTelemetryEvent{
			Headers: eventHeaders,
			Content: body,
		})

		if shouldFlush {
			select {
			case forwarder.flushChan <- batch:
				writeEmptyJSON(w, http.StatusOK)
			default:
				// This drops not only the current payload but also previously accumulated
				// messages
				forwarder.inflightCount.Add(-int64(totalBodySize))
				writeEmptyJSON(w, http.StatusTooManyRequests)
			}
		} else {
			writeEmptyJSON(w, http.StatusOK)
		}
	})
}

func writeEmptyJSON(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
	w.Write([]byte("{}"))
}

// buildEventHeaders creates the per-event headers map from an incoming SDK request.
// This includes all original request headers plus container-level enrichment.
func (f *TelemetryForwarder) buildEventHeaders(req *http.Request) map[string]string {
	headers := make(map[string]string, len(req.Header)+4)

	// Flatten original SDK request headers
	for key, values := range req.Header {
		headers[key] = strings.Join(values, ", ")
	}

	// Container-level enrichment
	containerID := f.containerIDProvider.GetContainerID(req.Context(), req.Header)
	if containerID == "" {
		_ = f.statsd.Count("datadog.trace_agent.telemetry_proxy.no_container_id_found", 1, []string{}, 1)
	}
	containerTags := getContainerTags(f.conf.ContainerTags, containerID)

	if containerID != "" {
		headers[header.ContainerID] = containerID
	}
	if containerTags != "" {
		headers["X-Datadog-Container-Tags"] = normalizeHTTPHeader(containerTags)
	}

	// Fargate cloud provider headers (per-event, derived from container tags)
	if taskArn, ok := extractFargateTask(containerTags); ok {
		headers[cloudProviderHeader] = string(aws)
		headers[cloudResourceTypeHeader] = string(awsFargate)
		headers[cloudResourceIdentifierHeader] = taskArn
	}

	return headers
}

// setBatchRequestHeaders sets host-level headers on an outbound batch HTTP request.
func (f *TelemetryForwarder) setBatchRequestHeaders(req *http.Request) {
	req.Header.Set("Via", "trace-agent "+f.conf.AgentVersion)
	if _, ok := req.Header["User-Agent"]; !ok {
		// explicitly disable User-Agent so it's not set to the default value
		// that net/http gives it: Go-http-client/1.1
		// See https://codereview.appspot.com/7532043
		req.Header.Set("User-Agent", "")
	}

	req.Header.Set("Dd-Agent-Hostname", f.conf.Hostname)
	req.Header.Set("Dd-Agent-Env", f.conf.DefaultEnv)
	req.Header.Set("Dd-Telemetry-Request-Type", "agent-batch")
	req.Header.Set("Content-Type", "application/msgpack")

	if f.conf.InstallSignature.Found {
		req.Header.Set("Dd-Agent-Install-Id", f.conf.InstallSignature.InstallID)
		req.Header.Set("Dd-Agent-Install-Type", f.conf.InstallSignature.InstallType)
		req.Header.Set("Dd-Agent-Install-Time", strconv.FormatInt(f.conf.InstallSignature.InstallTime, 10))
	}
	if origin, ok := f.conf.GlobalTags[originTag]; ok {
		switch origin {
		case "cloudrun":
			req.Header.Set(cloudProviderHeader, string(gcp))
			req.Header.Set(cloudResourceTypeHeader, string(cloudRun))
			if serviceName, found := f.conf.GlobalTags["service_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, serviceName)
			}
		case "cloudfunction":
			req.Header.Set(cloudProviderHeader, string(gcp))
			req.Header.Set(cloudResourceTypeHeader, string(cloudFunctions))
			if serviceName, found := f.conf.GlobalTags["service_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, serviceName)
			}
		case "appservice":
			req.Header.Set(cloudProviderHeader, string(azure))
			req.Header.Set(cloudResourceTypeHeader, string(azureAppService))
			if appName, found := f.conf.GlobalTags["app_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, appName)
			}
		case "containerapp":
			req.Header.Set(cloudProviderHeader, string(azure))
			req.Header.Set(cloudResourceTypeHeader, string(azureContainerApp))
			if appName, found := f.conf.GlobalTags["app_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, appName)
			}
		}
	}
}

// batchFlusher runs in a dedicated goroutine. It serializes batches from flushChan
// (size-triggered) or from the buffer (age-triggered) and sends them to forwardChan
// for the forwarder workers.
func (f *TelemetryForwarder) batchFlusher() {
	defer f.inflightWaiter.Done()
	ticker := time.NewTicker(batchCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.done:
			// Drain any remaining size-triggered batches from flushChan
			for {
				select {
				case batch := <-f.flushChan:
					f.serializeBatch(batch)
				default:
					goto drainDone
				}
			}
		drainDone:
			// Flush remaining events in the buffer
			batch, totalBodySize := f.batch.takeBatch()
			if totalBodySize > 0 {
				f.serializeBatchWithSize(batch, totalBodySize)
			}
			close(f.forwardChan)
			return

		case batch := <-f.flushChan:
			f.serializeBatch(batch)

		case <-ticker.C:
			batch, totalBodySize, shouldFlush := f.batch.checkFlushAge()
			if shouldFlush {
				f.serializeBatchWithSize(batch, totalBodySize)
			}
		}
	}
}

// serializeBatch computes totalBodySize from the events and delegates to serializeBatchWithSize.
// Used for size-triggered flushes where the totalBodySize isn't pre-computed.
func (f *TelemetryForwarder) serializeBatch(events []rawTelemetryEvent) {
	totalBodySize := 0
	for _, e := range events {
		totalBodySize += len(e.Content)
	}
	f.serializeBatchWithSize(events, totalBodySize)
}

// serializeBatchWithSize serializes a batch of events to MessagePack and sends it to forwardChan.
func (f *TelemetryForwarder) serializeBatchWithSize(events []rawTelemetryEvent, totalBodySize int) {
	body, err := msgpack.Marshal(&agentBatch{
		RequestType: "agent-batch",
		Payload: rawTelemetryEvents{
			Events: events,
		},
	})
	if err != nil {
		f.logger.Error("Failed to serialize telemetry batch: %v", err)
		f.inflightCount.Add(-int64(totalBodySize))
		return
	}

	f.forwardChan <- forwardedBatch{
		body:          body,
		totalBodySize: totalBodySize,
	}
}

// forwardBatchToEndpoints sends a serialized batch to all configured endpoints.
func (f *TelemetryForwarder) forwardBatchToEndpoints(batch forwardedBatch) {
	defer f.inflightCount.Add(-int64(batch.totalBodySize))

	for _, e := range f.endpoints {
		tags := []string{"endpoint:" + e.Host}
		start := time.Now()

		req, err := http.NewRequestWithContext(f.cancelCtx, http.MethodPost, batchURLPath, bytes.NewReader(batch.body))
		if err != nil {
			f.logger.Error("Failed to create batch request: %v", err)
			continue
		}
		f.setBatchRequestHeaders(req)
		req.Host = e.Host
		req.URL.Host = e.Host
		req.URL.Scheme = "https"
		req.Header.Set("DD-API-KEY", e.APIKey)

		resp, err := f.client.Do(req)
		_ = f.statsd.Timing("datadog.trace_agent.telemetry_proxy.roundtrip_ms", time.Since(start), tags, 1)
		if err != nil {
			_ = f.statsd.Count("datadog.trace_agent.telemetry_proxy.error", 1, tags, 1)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func extractFargateTask(containerTags string) (string, bool) {
	return extractTag(containerTags, "task_arn")
}

func extractTag(tags string, name string) (string, bool) {
	leftoverTags := tags
	for {
		if leftoverTags == "" {
			return "", false
		}
		var tag string
		tag, leftoverTags, _ = strings.Cut(leftoverTags, ",")

		tagName, value, hasValue := strings.Cut(tag, ":")
		if hasValue && tagName == name {
			return value, true
		}
	}
}
