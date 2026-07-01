// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lifecycle implements the AWS Lambda MicroVM lifecycle hook server.
// The MicroVM platform calls POST endpoints on port 9000 at key points in
// the MicroVM lifecycle: ready, validate, run, resume, suspend, terminate.
//
// Hook semantics:
//   - /ready    — "I am booted and ready to be snapshotted." The platform
//     sends this during image build, before snapshot capture; a 200 triggers
//     snapshot, and it is retried on 503 until a 200 or the configured timeout.
//   - /validate — build-time hook. After the snapshot is captured the platform
//     runs a test MicroVM from it and sends /validate to smoke-test the
//     snapshot (retried on 503 until a 200 or the configured timeout); a 200
//     marks the image version valid. It is NOT sent during the normal
//     run/resume lifecycle of a production MicroVM.
//   - /run      — VM is starting (cold start or from snapshot).
//   - /resume   — VM is resuming from a suspended snapshot.
//   - /suspend  — VM is about to be snapshotted/frozen.
//   - /terminate — VM is being torn down permanently.
//
// The agent responds to each hook itself: /ready checks child-process
// liveness; /validate returns 200 directly; /run, /resume, /suspend, and
// /terminate emit an enhanced metric and, for /suspend and /terminate, flush
// telemetry before responding.
//
// /terminate does NOT synthesize SIGTERM. The platform owns process
// termination via OS signals delivered independently of this HTTP event.
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	basePath      = "/aws/lambda-microvms/runtime/v1/"
	pathReady     = basePath + "ready"
	pathValidate  = basePath + "validate"
	pathRun       = basePath + "run"
	pathSuspend   = basePath + "suspend"
	pathResume    = basePath + "resume"
	pathTerminate = basePath + "terminate"

	postReady     = "POST " + pathReady
	postValidate  = "POST " + pathValidate
	postRun       = "POST " + pathRun
	postSuspend   = "POST " + pathSuspend
	postResume    = "POST " + pathResume
	postTerminate = "POST " + pathTerminate

	// flushWorkerCount is the number of goroutines launched by flushAll:
	// one each for the metric, trace, and logs flushers.
	flushWorkerCount = 3

	baseMetricPrefix    = "aws.lambda.enhanced.microvm."
	runMetricName       = baseMetricPrefix + "run"
	suspendMetricName   = baseMetricPrefix + "suspend"
	resumeMetricName    = baseMetricPrefix + "resume"
	terminateMetricName = baseMetricPrefix + "terminate"
	validateMetricName  = baseMetricPrefix + "validate"

	lambdaMicroVMID = "lambda_microvm_id:" // for []string log/metric tags: key:value concatenated
)

// flushMode controls whether and how telemetry is flushed during a lifecycle hook.
type flushMode int

const (
	noFlush         flushMode = iota // /run, /resume: no flush
	flushParallel                    // /suspend: flush concurrently
	flushSequential                  // /terminate: flush after the hook's own work completes
)

// Flusher is satisfied by serverless.FlushableAgent.
type Flusher interface {
	Flush()
}

// LogsFlusher is satisfied by logsAgent.ServerlessLogsAgent.
type LogsFlusher interface {
	Flush(ctx context.Context)
}

// SampleDrainer blocks until all metric samples enqueued before the call have
// been consumed by the aggregator worker. Must be called before Flush to ensure
// lifecycle metrics emitted via AddEnhancedMetric are included in the flush.
// Satisfied by *serverlessMetrics.ServerlessMetricAgent.
type SampleDrainer interface {
	WaitForPendingSamples()
}

// runBody is the JSON payload sent by the MicroVM platform on /run.
type runBody struct {
	MicroVMID string `json:"microvmId"`
}

// Server is the MicroVM lifecycle hook HTTP server.
type Server struct {
	metricFlusher Flusher
	traceFlusher  Flusher
	logsFlusher   LogsFlusher
	metricEmitter MetricEmitter
	sampleDrainer SampleDrainer
	instanceID    *atomic.String // set once from /run body
	metricSource  metrics.MetricSource
	flushTimeout  time.Duration

	childHandle ChildHandle // production: *Child (init) or NewNoopChildHandle() (sidecar); always non-nil after lifecycle.SetupFromEnv. nil only in legacy unit tests; logs WARN if hit.
	child       *Child      // non-nil only in init-container mode; nil in sidecar and unit tests. Derived from childHandle when it is a *Child.
	heartbeat   *Heartbeat  // nil-safe; nil disables periodic heartbeat emission

	httpServer *http.Server
}

// writeTimeoutHeadroom covers work that shares WriteTimeout's wall clock but
// falls outside flushAll's own budget: heartbeat.Stop() (bounded by
// heartbeatStopTimeout) runs before /suspend and /terminate ever reach
// dispatchHook. Without this headroom, WriteTimeout can expire in that gap
// even though the flush completed, and the platform sees a failed hook.
//
// The extra 500ms is not a bounded blocking call like heartbeatStopTimeout —
// it's scheduling/response-write jitter margin (goroutine scheduling delay,
// GC pauses, the final WriteHeader network write). 500ms is comfortably
// above single-digit-millisecond typical jitter while staying small relative
// to flushTimeout (seconds) and the platform's own hook timeout budget
// (1-60s), so it doesn't meaningfully erode either.
const writeTimeoutHeadroom = heartbeatStopTimeout + 500*time.Millisecond

// readTimeout bounds how long the server waits to receive a hook request
// (headers + body) from the platform. Deliberately independent of the
// platform's own hook timeouts (1-60s): those bound how long the platform
// waits for our response, not how long the platform takes to finish sending
// the request. Hook bodies are small, so 30s is generous headroom, not a
// tight fit.
const readTimeout = 30 * time.Second

// NewServer constructs a lifecycle Server. port is typically DefaultPort (9000) but can be overridden via DD_AWS_MICROVM_LIFECYCLE_PORT.
func NewServer(
	port int,
	metricFlusher Flusher,
	traceFlusher Flusher,
	logsFlusher LogsFlusher,
	metricEmitter MetricEmitter,
	sampleDrainer SampleDrainer,
	metricSource metrics.MetricSource,
	flushTimeout time.Duration,
	childHandle ChildHandle, // may be nil
	heartbeat *Heartbeat, // may be nil
) *Server {
	s := &Server{
		metricFlusher: metricFlusher,
		traceFlusher:  traceFlusher,
		logsFlusher:   logsFlusher,
		metricEmitter: metricEmitter,
		sampleDrainer: sampleDrainer,
		metricSource:  metricSource,
		flushTimeout:  flushTimeout,
		childHandle:   childHandle,
		instanceID:    atomic.NewString(""),
		heartbeat:     heartbeat,
	}
	// Derive the concrete *Child from the handle when possible so callers can
	// reach it via Server.Child() without a separate return value.
	if c, ok := childHandle.(*Child); ok {
		s.child = c
	}
	// WriteTimeout must cover the full handler wall-clock: the flush budget
	// (for /suspend and /terminate) plus write headroom.
	writeTimeout := s.flushTimeout + writeTimeoutHeadroom
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.handler(),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
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
// so callers can defer Stop unconditionally. Also stops the heartbeat
// goroutine — defense-in-depth for shutdown paths that don't first hit
// /suspend or /terminate (e.g., the platform SIGKILLs the process).
func (s *Server) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.heartbeat.Stop()
	return s.httpServer.Shutdown(ctx)
}

// Child returns the *Child for this server, or nil when the server is nil,
// running in sidecar mode, or constructed without a *Child handle (e.g. unit
// tests). This lets callers obtain the child through the server rather than
// as a separate return value: a nil server naturally implies a nil child.
func (s *Server) Child() *Child {
	if s == nil {
		return nil
	}
	return s.child
}

// Heartbeat returns the Heartbeat wired into this server.
// Exposed for white-box tests in external packages; not part of the stable API.
func (s *Server) Heartbeat() *Heartbeat { return s.heartbeat }

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(postReady, s.handleReady)
	mux.HandleFunc(postValidate, s.handleValidate)
	mux.HandleFunc(postRun, s.handleRun)
	mux.HandleFunc(postSuspend, s.handleSuspend)
	mux.HandleFunc(postResume, s.handleResume)
	mux.HandleFunc(postTerminate, s.handleTerminate)
	return mux
}

// handleReady answers the platform's "are you booted and ready to snapshot?"
// signal. A 200 tells the platform it may take a snapshot of the VM; non-200
// causes a retry (the platform does not treat /ready failures as fatal).
//
// Alive-check via ChildHandle: child alive → 200, anything else (not yet
// started, already exited, or nil handle) → 503. The pre-spawn race is
// absorbed by the platform's /ready retry behavior; diagnostic detail
// (cmd.Start / cmd.Wait errors) is logged at the call site in mode.RunInit,
// where the actual error value is available.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: ready")
	s.aliveCheckReady(w)
}

// handleValidate answers the platform's build-time snapshot smoke test. After
// the snapshot is captured the platform runs a test MicroVM from it and sends
// /validate; a 200 marks the image version valid, a 503 asks the platform to
// retry until validateTimeout. It is NOT sent during the normal run/resume
// lifecycle of a production MicroVM.
func (s *Server) handleValidate(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: validate")
	// TODO(microvm-validate-metric): /validate is a build-time hook that only
	// fires on the ephemeral test MicroVM during image build (and may be retried
	// on 503), never during a production instance's run/resume lifecycle. This
	// metric therefore reflects build-step activity, not production behavior, and
	// may not reliably flush before the test VM is torn down. Revisit whether it
	// carries enough value to keep alongside the genuine runtime lifecycle metrics.
	s.emitLifecycleMetric(validateMetricName)
	w.WriteHeader(http.StatusOK)
}

// flushAll runs metric/trace/logs flushes in parallel and waits for them,
// bounded by flushCtx. Used by handleSuspend / handleTerminate. It first
// drains any pending metric samples so that lifecycle metrics emitted
// immediately before this call are included in the flush.
func (s *Server) flushAll(flushCtx context.Context) {
	if s.sampleDrainer != nil {
		drained := make(chan struct{})
		go func() {
			s.sampleDrainer.WaitForPendingSamples()
			close(drained)
		}()
		select {
		case <-drained:
		case <-flushCtx.Done():
			log.Warnf("MicroVM lifecycle: timed out waiting for pending samples to drain")
		}
	}
	flushDone := make(chan struct{}, flushWorkerCount)
	go func() { s.metricFlusher.Flush(); flushDone <- struct{}{} }()
	go func() { s.traceFlusher.Flush(); flushDone <- struct{}{} }()
	go func() { s.logsFlusher.Flush(flushCtx); flushDone <- struct{}{} }()
	s.waitForFlushes(flushCtx, flushDone)
}

// aliveCheckReady answers /ready. Binary mapping: child alive → 200, anything
// else → 503. The platform's retry behavior absorbs the pre-spawn window, so
// we don't wait here.
func (s *Server) aliveCheckReady(w http.ResponseWriter) {
	if s.childHandle == nil {
		// Wiring bug: setup() MUST construct either *Child (init-container
		// mode) or NewNoopChildHandle() (sidecar mode). nil here means
		// neither path ran — only possible in legacy unit tests.
		log.Warn("MicroVM lifecycle: /ready called with nil ChildHandle (wiring bug); returning 503")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if s.childHandle.IsAlive() {
		w.WriteHeader(http.StatusOK)
		return
	}
	log.Debug("MicroVM lifecycle: /ready returning 503 (child not yet started or already exited); platform will retry")
	w.WriteHeader(http.StatusServiceUnavailable)
}

// handleRun emits the run metric, captures the MicroVM instance ID from the
// platform's request body, and starts the periodic heartbeat. The ID is
// captured before Start so the first heartbeat emission already carries the
// correct microvm_id tag.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	var body runBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		log.Debugf("MicroVM lifecycle: could not parse run body: %v", err)
	}
	if body.MicroVMID != "" {
		log.Infof("MicroVM lifecycle: run (microvm_id=%s)", body.MicroVMID)
		s.instanceID.Store(body.MicroVMID)
		s.heartbeat.SetMicroVMID(body.MicroVMID)
	} else {
		log.Info("MicroVM lifecycle: run")
	}
	s.heartbeat.Start()
	s.dispatchHook(runMetricName, noFlush, w)
}

// handleResume emits the resume metric and restarts the heartbeat (stopped at
// /suspend).
func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: resume")
	s.heartbeat.Start()
	s.dispatchHook(resumeMetricName, noFlush, w)
}

// handleSuspend stops the heartbeat and flushes telemetry. Heartbeat is
// stopped before flushing so any in-flight tick finishes before the metric
// agent drains; /resume restarts it.
func (s *Server) handleSuspend(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: suspend — flushing telemetry")
	s.heartbeat.Stop()
	s.dispatchHook(suspendMetricName, flushParallel, w)
}

// handleTerminate flushes all telemetry.
//
// No synthetic SIGTERM is sent. AWS Lambda MicroVM does not promise a
// separate SIGTERM (the 200 IS the termination trigger and Firecracker
// destroys the VM on response or at the 60s platform deadline); the agent
// must not simulate a signal channel that the platform does not provide.
// Heartbeat is stopped here so it cannot fire after the VM is torn down.
func (s *Server) handleTerminate(w http.ResponseWriter, _ *http.Request) {
	log.Info("MicroVM lifecycle: terminate — flushing telemetry")
	s.heartbeat.Stop()
	s.dispatchHook(terminateMetricName, flushSequential, w)
}

// dispatchHook is the shared dispatch path for run, resume, suspend, and
// terminate: emit metric, optionally flush, respond 200.
func (s *Server) dispatchHook(metricName string, mode flushMode, w http.ResponseWriter) {
	s.emitLifecycleMetric(metricName)
	if mode != noFlush {
		flushCtx, cancel := context.WithTimeout(context.Background(), s.flushTimeout)
		defer cancel()
		s.flushAll(flushCtx)
	}
	w.WriteHeader(http.StatusOK)
}

// emitLifecycleMetric records a lifecycle event metric, appending the stored
// MicroVM instance ID tag when one has been captured from the /run body.
func (s *Server) emitLifecycleMetric(name string) {
	var extraTags []string
	if id := s.instanceID.Load(); id != "" {
		extraTags = []string{lambdaMicroVMID + id}
	}
	emitMetric(s.metricEmitter, s.metricSource, name, extraTags...)
}

// emitMetric emits a count-1 enhanced metric with the current wall-clock
// timestamp, so every caller uses the same precision and avoids the
// metric-agent's 0-sentinel substitution.
func emitMetric(emitter MetricEmitter, source metrics.MetricSource, name string, tags ...string) {
	timestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	emitter.AddEnhancedMetric(name, 1.0, source, timestamp, tags...)
}

// waitForFlushes collects exactly 3 completion signals from flushDone — one
// per goroutine launched by flushAll (metric, trace, logs). The select inside
// the loop applies a single shared deadline across all three: if flushCtx
// expires at any iteration the remaining flushes are abandoned and the handler
// can return to the platform promptly. The caller allocates flushDone with
// cap=3 so the three goroutines can send without blocking even after an early
// return here, preventing goroutine leaks.
func (s *Server) waitForFlushes(flushCtx context.Context, flushDone <-chan struct{}) {
	for i := 0; i < flushWorkerCount; i++ {
		select {
		case <-flushDone:
		case <-flushCtx.Done():
			log.Warnf("MicroVM lifecycle: flush timed out after %s", s.flushTimeout)
			return
		}
	}
	log.Info("MicroVM lifecycle: flush complete")
}
