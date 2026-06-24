// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lifecycle implements the AWS Lambda MicroVM lifecycle hook server.
// The MicroVM platform calls POST endpoints on port 9000 at key points in the
// MicroVM lifecycle. This server responds to those hooks and coordinates
// telemetry flushing before the process is suspended or terminated.
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultPort is the port the MicroVM platform expects the lifecycle hook server on.
const DefaultPort = 9000

const (
	pathReady     = "/aws/lambda-microvms/runtime/beta/v1/ready"
	pathLaunch    = "/aws/lambda-microvms/runtime/beta/v1/launch"
	pathSuspend   = "/aws/lambda-microvms/runtime/beta/v1/suspend"
	pathResume    = "/aws/lambda-microvms/runtime/beta/v1/resume"
	pathTerminate = "/aws/lambda-microvms/runtime/beta/v1/terminate"

	launchMetricName    = "aws.lambda.microvm.enhanced.launch"
	suspendMetricName   = "aws.lambda.microvm.enhanced.suspend"
	resumeMetricName    = "aws.lambda.microvm.enhanced.resume"
	terminateMetricName = "aws.lambda.microvm.enhanced.terminate"

	instanceIDTagPrefix = "instance_id:"
)

// Flusher is satisfied by serverless.FlushableAgent.
type Flusher interface {
	Flush()
}

// LogsFlusher is satisfied by logsAgent.ServerlessLogsAgent.
type LogsFlusher interface {
	Flush(ctx context.Context)
}

// MetricEmitter can emit a single enhanced metric. Satisfied by
// *serverlessMetrics.ServerlessMetricAgent.
type MetricEmitter interface {
	AddEnhancedMetric(name string, value float64, source metrics.MetricSource, timestamp float64, extraTags ...string)
}

// SampleDrainer blocks until all metric samples enqueued before the call have
// been consumed by the aggregator worker. Must be called before Flush to ensure
// lifecycle metrics emitted via AddEnhancedMetric are included in the flush.
// Satisfied by *serverlessMetrics.ServerlessMetricAgent.
type SampleDrainer interface {
	WaitForPendingSamples()
}

// launchBody is the JSON payload sent by the MicroVM platform on /launch.
type launchBody struct {
	MicroVmID string `json:"microVmId"`
}

// Server is the MicroVM lifecycle hook HTTP server.
type Server struct {
	metricFlusher Flusher
	traceFlusher  Flusher
	logsFlusher   LogsFlusher
	metricEmitter MetricEmitter
	sampleDrainer SampleDrainer
	instanceID    atomic.Value // string; set once from /launch body
	metricSource  metrics.MetricSource
	flushTimeout  time.Duration

	httpServer *http.Server
}

// NewServer constructs a lifecycle Server. port should be 9000 per the MicroVM spec.
func NewServer(
	port int,
	metricFlusher Flusher,
	traceFlusher Flusher,
	logsFlusher LogsFlusher,
	metricEmitter MetricEmitter,
	sampleDrainer SampleDrainer,
	metricSource metrics.MetricSource,
	flushTimeout time.Duration,
) *Server {
	s := &Server{
		metricFlusher: metricFlusher,
		traceFlusher:  traceFlusher,
		logsFlusher:   logsFlusher,
		metricEmitter: metricEmitter,
		sampleDrainer: sampleDrainer,
		metricSource:  metricSource,
		flushTimeout:  flushTimeout,
	}
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: s.flushTimeout + 10*time.Second,
	}
	return s
}

// Listen binds the TCP port synchronously. Call before Serve so the socket is
// ready before the MicroVM platform sends the first lifecycle hook.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

// Serve accepts connections on l until Stop is called. Blocks until the server exits.
// Call in a goroutine after a successful Listen.
func (s *Server) Serve(l net.Listener) {
	log.Infof("MicroVM lifecycle server listening on %s", l.Addr())
	if err := s.httpServer.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Errorf("MicroVM lifecycle server error: %v", err)
	}
}

// Stop gracefully shuts down the lifecycle server, waiting for any in-flight
// requests to complete up to ctx's deadline. Safe to call on a nil receiver
// so callers can defer Stop unconditionally.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(pathReady, s.handleReady)
	mux.HandleFunc(pathLaunch, s.handleLaunch)
	mux.HandleFunc(pathSuspend, s.handleSuspend)
	mux.HandleFunc(pathResume, s.handleResume)
	mux.HandleFunc(pathTerminate, s.handleTerminate)
	return mux
}

// handleReady signals to the platform that the agent is ready. This hook is
// called once at container startup. No telemetry is flushed here; it is a
// build-time signal only.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: ready")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: launch")
	var body launchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Debugf("MicroVM lifecycle: could not parse launch body: %v", err)
	} else if body.MicroVmID != "" {
		s.instanceID.Store(body.MicroVmID)
	}
	s.emitLifecycleMetric(launchMetricName)
	w.WriteHeader(http.StatusOK)
}

// handleSuspend flushes all telemetry before responding 200. The process may
// be frozen by the platform immediately after the 200 response.
func (s *Server) handleSuspend(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: suspend — flushing telemetry")
	s.emitLifecycleMetric(suspendMetricName)
	s.flushAll()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: resume")
	s.emitLifecycleMetric(resumeMetricName)
	w.WriteHeader(http.StatusOK)
}

// handleTerminate flushes all telemetry before responding 200. The /terminate
// hook is informational: the MicroVM platform owns process teardown after this
// handler returns, so the server does not self-signal SIGTERM.
func (s *Server) handleTerminate(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: terminate — flushing telemetry")
	s.emitLifecycleMetric(terminateMetricName)
	s.flushAll()
	w.WriteHeader(http.StatusOK)
}

// emitLifecycleMetric records a lifecycle event with the timestamp captured
// at the call site, so the metric reflects when AWS invoked the hook rather
// than when the sample is later aggregated.
func (s *Server) emitLifecycleMetric(name string) {
	timestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	var extraTags []string
	if id, ok := s.instanceID.Load().(string); ok && id != "" {
		extraTags = []string{instanceIDTagPrefix + id}
	}
	s.metricEmitter.AddEnhancedMetric(name, 1.0, s.metricSource, timestamp, extraTags...)
}

// flushAll flushes the configured telemetry agents in parallel, bounded by flushTimeout.
// It first drains any pending metric samples so that lifecycle metrics emitted
// immediately before this call are included in the flush.
func (s *Server) flushAll() {
	ctx, cancel := context.WithTimeout(context.Background(), s.flushTimeout)
	defer cancel()

	// AddEnhancedMetric enqueues into the demux worker channel asynchronously.
	// Drain before flushing so the just-emitted lifecycle metric is not missed.
	if s.sampleDrainer != nil {
		drained := make(chan struct{})
		go func() {
			s.sampleDrainer.WaitForPendingSamples()
			close(drained)
		}()
		select {
		case <-drained:
		case <-ctx.Done():
			log.Warnf("MicroVM lifecycle: timed out waiting for pending samples to drain")
		}
	}

	// Collect the configured flushers. Any of them may be nil if its agent failed
	// to start (e.g. the logs agent); skip those rather than panicking so the
	// remaining agents still flush. Waiting on len(flushers) keeps the wait count
	// in sync with the goroutines launched, even as flushers are added or removed.
	flushers := []func(){}
	if s.metricFlusher != nil {
		flushers = append(flushers, func() { s.metricFlusher.Flush() })
	}
	if s.traceFlusher != nil {
		flushers = append(flushers, func() { s.traceFlusher.Flush() })
	}
	if s.logsFlusher != nil {
		flushers = append(flushers, func() { s.logsFlusher.Flush(ctx) })
	}

	done := make(chan struct{}, len(flushers))
	for _, flush := range flushers {
		go func() { flush(); done <- struct{}{} }()
	}

	for range flushers {
		select {
		case <-done:
		case <-ctx.Done():
			log.Warnf("MicroVM lifecycle: flush timed out after %s", s.flushTimeout)
			return
		}
	}
	log.Info("MicroVM lifecycle: flush complete")
}
