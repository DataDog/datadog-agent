// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/injectortelemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// defaultFlushInterval is how often accumulated metrics are flushed to the telemetry forwarder.
	defaultFlushInterval = 10 * time.Second

	// maxDatagramSize is the maximum size of a single datagram we'll read.
	// Individual telemetry metrics are well under 1KB.
	maxDatagramSize = 8192

	// maxBatchAge is how long we keep an idle batch before discarding it.
	maxBatchAge = 5 * time.Minute
)

// batchedMetric holds a single metric point accumulated from the injector.
type batchedMetric struct {
	Name  string   `json:"metric"`
	Type  string   `json:"type"`
	Value float64  `json:"-"`
	Tags  []string `json:"tags"`

	// Points is formatted as [[timestamp, value], ...] for the telemetry payload.
	Points [][]interface{} `json:"points"`
	Common bool            `json:"common"`
}

// injectorBatch accumulates metrics from a single runtime instance (identified by runtime_id).
type injectorBatch struct {
	runtimeID       string
	languageName    string
	languageVersion string
	injectorVersion string
	externalEnv     string
	metrics         []batchedMetric
	lastSeen        time.Time
}

// InjectorTelemetryReceiver listens on a SOCK_DGRAM Unix domain socket for
// lightweight telemetry from the auto-inject injector. It accumulates metrics
// and periodically flushes them as batched telemetry payloads via the
// TelemetryForwarder.
type InjectorTelemetryReceiver struct {
	conn      *net.UnixConn
	conf      *config.AgentConfig
	forwarder *TelemetryForwarder

	mu      sync.Mutex
	batches map[string]*injectorBatch // keyed by runtime_id

	flushInterval time.Duration
	done          chan struct{}
	statsd        statsd.ClientInterface
}

// NewInjectorTelemetryReceiver creates a new receiver.
func NewInjectorTelemetryReceiver(
	conf *config.AgentConfig,
	forwarder *TelemetryForwarder,
	statsd statsd.ClientInterface,
) *InjectorTelemetryReceiver {
	return &InjectorTelemetryReceiver{
		conf:          conf,
		forwarder:     forwarder,
		batches:       make(map[string]*injectorBatch),
		flushInterval: defaultFlushInterval,
		done:          make(chan struct{}),
		statsd:        statsd,
	}
}

// Start begins listening for injector telemetry datagrams.
func (r *InjectorTelemetryReceiver) Start() error {
	socketPath := r.conf.InjectorTelemetrySocket

	// Remove stale socket file if it exists.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stale socket %s: %w", socketPath, err)
	}

	addr := &net.UnixAddr{Name: socketPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", socketPath, err)
	}

	// Make the socket writable by all users so the injector (running in
	// arbitrary processes) can send datagrams.
	if err := os.Chmod(socketPath, 0722); err != nil {
		conn.Close()
		return fmt.Errorf("failed to chmod socket %s: %w", socketPath, err)
	}

	r.conn = conn
	log.Infof("Injector telemetry receiver listening on %s", socketPath)

	go r.readLoop()
	go r.flushLoop()

	return nil
}

// Stop shuts down the receiver.
func (r *InjectorTelemetryReceiver) Stop() {
	close(r.done)
	if r.conn != nil {
		r.conn.Close()
	}
	// Flush any remaining metrics.
	r.flush()
}

// readLoop continuously reads datagrams and processes them.
func (r *InjectorTelemetryReceiver) readLoop() {
	buf := make([]byte, maxDatagramSize)
	for {
		select {
		case <-r.done:
			return
		default:
		}

		// Set a read deadline so we periodically check the done channel.
		r.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := r.conn.ReadFromUnix(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-r.done:
				return
			default:
				log.Errorf("Injector telemetry read error: %v", err)
				continue
			}
		}

		if n == 0 {
			continue
		}

		r.processDatagram(buf[:n])
	}
}

// processDatagram parses a single flatbuffer datagram and accumulates the metric.
func (r *InjectorTelemetryReceiver) processDatagram(data []byte) {
	msg := injectortelemetry.GetRootAsTelemetryMetric(data, 0)
	if msg == nil {
		_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.parse_error", 1, nil, 1)
		return
	}

	runtimeID := string(msg.RuntimeId())
	if runtimeID == "" {
		_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.missing_runtime_id", 1, nil, 1)
		return
	}

	metricName := string(msg.Name())
	metricType := "count"
	if msg.Type() == injectortelemetry.MetricTypeDistribution {
		metricType = "distribution"
	}

	tags := make([]string, msg.TagsLength())
	for i := 0; i < msg.TagsLength(); i++ {
		tags[i] = string(msg.Tags(i))
	}

	now := time.Now()
	metric := batchedMetric{
		Name:   metricName,
		Type:   metricType,
		Value:  msg.Value(),
		Tags:   tags,
		Points: [][]interface{}{{now.Unix(), msg.Value()}},
		Common: true,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[runtimeID]
	if !ok {
		batch = &injectorBatch{
			runtimeID:       runtimeID,
			languageName:    string(msg.LanguageName()),
			languageVersion: string(msg.LanguageVersion()),
			injectorVersion: string(msg.InjectorVersion()),
			externalEnv:     string(msg.ExternalEnv()),
		}
		r.batches[runtimeID] = batch
	}

	batch.metrics = append(batch.metrics, metric)
	batch.lastSeen = now

	_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.metrics_received", 1, nil, 1)
}

// flushLoop periodically flushes accumulated metrics.
func (r *InjectorTelemetryReceiver) flushLoop() {
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.flush()
		}
	}
}

// flush sends all accumulated batches to the telemetry forwarder.
func (r *InjectorTelemetryReceiver) flush() {
	r.mu.Lock()
	if len(r.batches) == 0 {
		r.mu.Unlock()
		return
	}

	// Swap out the batches map so we can release the lock quickly.
	batches := r.batches
	r.batches = make(map[string]*injectorBatch)
	r.mu.Unlock()

	now := time.Now()
	for _, batch := range batches {
		// Skip stale batches.
		if now.Sub(batch.lastSeen) > maxBatchAge {
			continue
		}

		if len(batch.metrics) == 0 {
			continue
		}

		body, headers, err := r.buildTelemetryPayload(batch)
		if err != nil {
			log.Errorf("Failed to build injector telemetry payload: %v", err)
			continue
		}

		r.forwardPayload(body, headers)
		_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.payloads_flushed", 1, nil, 1)
	}
}

// telemetryPayload is the JSON structure expected by the telemetry intake.
type telemetryPayload struct {
	APIVersion  string                 `json:"api_version"`
	RequestType string                 `json:"request_type"`
	SeqID       int                    `json:"seq_id"`
	RuntimeID   string                 `json:"runtime_id"`
	TracerTime  int64                  `json:"tracer_time"`
	Payload     telemetryPayloadInner  `json:"payload"`
	Application telemetryApplication   `json:"application"`
	Host        map[string]interface{} `json:"host"`
}

type telemetryPayloadInner struct {
	Namespace string          `json:"namespace"`
	Series    []batchedMetric `json:"series"`
}

type telemetryApplication struct {
	ServiceName    string `json:"service_name"`
	TracerVersion  string `json:"tracer_version"`
	LanguageName   string `json:"language_name"`
	LanguageVersion string `json:"language_version"`
}

// buildTelemetryPayload constructs the JSON payload and headers for a batch.
func (r *InjectorTelemetryReceiver) buildTelemetryPayload(batch *injectorBatch) ([]byte, http.Header, error) {
	payload := telemetryPayload{
		APIVersion:  "v2",
		RequestType: "generate-metrics",
		SeqID:       1,
		RuntimeID:   batch.runtimeID,
		TracerTime:  time.Now().Unix(),
		Payload: telemetryPayloadInner{
			Namespace: "tracers",
			Series:    batch.metrics,
		},
		Application: telemetryApplication{
			ServiceName:    "unknown",
			TracerVersion:  batch.injectorVersion,
			LanguageName:   batch.languageName,
			LanguageVersion: batch.languageVersion,
		},
		Host: map[string]interface{}{
			"hostname":       r.conf.Hostname,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal telemetry payload: %w", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("DD-Telemetry-Request-Type", "generate-metrics")
	if batch.externalEnv != "" {
		headers.Set("Datadog-External-Env", batch.externalEnv)
	}

	return body, headers, nil
}

// forwardPayload enqueues the payload on the TelemetryForwarder.
func (r *InjectorTelemetryReceiver) forwardPayload(body []byte, headers http.Header) {
	req, err := http.NewRequest(http.MethodPost, "/telemetry/proxy/api/v2/apmtelemetry", bytes.NewReader(body))
	if err != nil {
		log.Errorf("Failed to create injector telemetry forward request: %v", err)
		return
	}
	req.Header = headers

	if accepted := r.forwarder.startRequest(int64(len(body))); !accepted {
		_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.dropped", 1, nil, 1)
		return
	}

	select {
	case r.forwarder.forwardedReqChan <- forwardedRequest{
		req:  req,
		body: body,
	}:
	default:
		r.forwarder.endRequest(forwardedRequest{body: body})
		_ = r.statsd.Count("datadog.trace_agent.injector_telemetry.dropped", 1, nil, 1)
	}
}
