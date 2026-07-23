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
// This server handles those hooks in two modes:
//
//   - When DD_AWS_MICROVM_USER_APP_PORT is unset: the agent responds
//     to each hook itself. /ready checks child-process liveness; /validate
//     returns 200 directly; /run, /resume, /suspend, and /terminate emit
//     an enhanced metric and, for /suspend and /terminate, flush telemetry
//     before responding.
//
//   - When the env var is set: the agent forwards each hook to
//     127.0.0.1:<user-app-port> on the same path and mirrors the user app's
//     response (status, body, Content-Type) back to the platform. /ready and
//     /validate wait for TCP reachability before forwarding. For /run,
//     /resume, /suspend, and /terminate the agent's own work — metric
//     emission, /suspend and /terminate telemetry flush — runs in a goroutine
//     in parallel with the pass-through.
//
// /terminate does NOT synthesize SIGTERM. The platform owns process
// termination via OS signals delivered independently of this HTTP event.
package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strconv"
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

	lambdaMicroVMIDKey = "lambda_microvm_id"      // for map[string]string trace tags: key only
	lambdaMicroVMID    = lambdaMicroVMIDKey + ":" // for []string log/metric tags: key:value concatenated

	// mirrorResponseTimeout is the deadline for writing the mirrored response
	// back to the platform after the handler's main work (forward + optional
	// flush) is done. It covers the io.Copy in mirrorResponse, which has no
	// other deadline. Lifecycle responses are expected to be small; 5s is
	// generous even for slow platform connections.
	mirrorResponseTimeout = 5 * time.Second

	// maxRunBodyBytes caps the /run request body (runHookPayload) buffered in
	// handleRun. The payload is expected to be small; a 1 MiB cap prevents a
	// misconfigured RunMicrovm call from forcing an unbounded read.
	maxRunBodyBytes int64 = 1 << 20
)

// flushMode controls whether and how telemetry is flushed during a lifecycle hook.
type flushMode int

const (
	noFlush         flushMode = iota // /run, /resume: no flush
	flushParallel                    // /suspend: flush concurrently with forward
	flushSequential                  // /terminate: flush after forward completes
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
// Optional: a nil SampleDrainer disables draining (see flushAll).
type SampleDrainer interface {
	WaitForPendingSamples()
}

// LogsTagSetter can replace the full tag slice on the live log pipeline.
// Satisfied by serverlessLogs.SetLogsTags (wrapped via LogsTagSetterFunc).
type LogsTagSetter interface {
	SetLogsTags(tags []string)
}

// LogsTagSetterFunc wraps a bare function so it satisfies LogsTagSetter.
type LogsTagSetterFunc func([]string)

// SetLogsTags implements LogsTagSetter.
func (f LogsTagSetterFunc) SetLogsTags(tags []string) { f(tags) }

// TraceTagSetter can replace the full tag map on the live trace pipeline.
// Satisfied by trace.ServerlessTraceAgent.SetTags (wrapped via TraceTagSetterFunc).
type TraceTagSetter interface {
	SetTraceTags(tags map[string]string)
}

// TraceTagSetterFunc wraps a bare function so it satisfies TraceTagSetter.
type TraceTagSetterFunc func(map[string]string)

// SetTraceTags implements TraceTagSetter.
func (f TraceTagSetterFunc) SetTraceTags(tags map[string]string) { f(tags) }

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
	fwd         *Forwarder  // nil = no opt-in; today's behavior preserved
	heartbeat   *Heartbeat  // nil-safe; nil disables periodic heartbeat emission

	logsTagSetter  LogsTagSetter     // nil-safe; set via SetLogsTagSetter after construction
	baseTags       []string          // startup tag snapshot; lambda_microvm_id is appended at /run
	traceTagSetter TraceTagSetter    // nil-safe; set via SetTraceTagSetter after construction
	baseTraceTags  map[string]string // startup trace tag snapshot; lambda_microvm_id is added at /run

	httpServer *http.Server
}

// writeTimeoutHeadroom covers work that shares WriteTimeout's wall clock but
// falls outside the per-path budget computed in NewServer:
//   - heartbeatStopTimeout bounds heartbeat.Stop(), which runs before
//     /suspend and /terminate ever reach dispatchHook.
//   - mirrorResponseTimeout bounds the final mirrored-response write
//     (io.Copy in mirrorResponse), which runs after the handler's main work.
//
// Without this headroom, WriteTimeout can expire in one of those gaps even
// though the handler's own work already completed, and the platform sees a
// failed hook.
const writeTimeoutHeadroom = heartbeatStopTimeout + mirrorResponseTimeout

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
	fwd *Forwarder, // may be nil
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
		fwd:           fwd,
		instanceID:    atomic.NewString(""),
		heartbeat:     heartbeat,
	}
	// Derive the concrete *Child from the handle when possible so callers can
	// reach it via Server.Child() without a separate return value.
	if c, ok := childHandle.(*Child); ok {
		s.child = c
	}
	// WriteTimeout must cover the full handler wall-clock for every path:
	//   - No forwarder: flushTimeout (flush budget + write headroom)
	//   - /run, /resume, /suspend, /terminate: forwardTimeout (default 1s)
	//   - /ready: readyTimeout (default 60s, matching platform /ready timeout)
	//   - /validate: validateTimeout (default 1s)
	// Use the largest of all applicable budgets so the HTTP server does not
	// close the platform-facing connection before the handler writes the
	// mirrored response.
	maxTimeout := s.flushTimeout
	if s.fwd != nil {
		// /terminate uses flushSequential: flush runs after the forward, so its
		// wall-clock is forwardTimeout+flushTimeout, not max of the two.
		maxTimeout = max(maxTimeout, s.fwd.forwardTimeout+s.flushTimeout, s.fwd.readyTimeout, s.fwd.validateTimeout)
	}
	writeTimeout := maxTimeout + writeTimeoutHeadroom
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.handler(),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
	return s
}

// SetLogsTagSetter wires a LogsTagSetter and a baseline tag slice into the server.
// Must be called before the first /run request. baseTags is the startup tag
// snapshot; lambda_microvm_id is appended to it when /run fires.
func (s *Server) SetLogsTagSetter(setter LogsTagSetter, baseTags []string) {
	s.logsTagSetter = setter
	s.baseTags = baseTags
}

// SetTraceTagSetter wires a TraceTagSetter and a baseline trace tag map into the
// server. Must be called before the first /run request. baseTraceTags is the
// startup trace tag snapshot; lambda_microvm_id is added to it when /run fires.
func (s *Server) SetTraceTagSetter(setter TraceTagSetter, baseTraceTags map[string]string) {
	s.traceTagSetter = setter
	s.baseTraceTags = baseTraceTags
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
// Dispatcher:
//   - If a Forwarder is configured (env-var opt-in), pass-through to the user
//     app with TCP-wait: dial errors map to 503, deadline to 504.
//   - Otherwise, alive-check via ChildHandle: child alive → 200, anything
//     else (not yet started, already exited, or nil handle) → 503. The
//     pre-spawn race is absorbed by the platform's /ready retry behavior;
//     diagnostic detail (cmd.Start / cmd.Wait errors) is logged at the
//     call site in mode.RunInit, where the actual error value is available.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: ready")
	if s.fwd != nil {
		s.passThroughReady(w, r)
		return
	}
	s.aliveCheckReady(w)
}

func (s *Server) passThroughReady(w http.ResponseWriter, r *http.Request) {
	resp := s.fwd.PassThroughWaiting(s.fwd.readyTimeout, pathReady, r.Header, r.Body)
	defer resp.Body.Close()
	mirrorResponse(w, resp)
}

// handleValidate answers the platform's build-time snapshot smoke test. After
// the snapshot is captured the platform runs a test MicroVM from it and sends
// /validate; a 200 marks the image version valid, a 503 asks the platform to
// retry until validateTimeout. It is NOT sent during the normal run/resume
// lifecycle of a production MicroVM.
//
// When a Forwarder is configured (DD_AWS_MICROVM_USER_APP_PORT set):
// pass-through to the user app with TCP-wait, mirroring the response, so the
// user app's own smoke test drives the build's validity decision. The TCP-wait
// absorbs the window before the app is reachable on the test run. Without a
// forwarder the agent returns 200 directly; the user app is not required to
// implement /validate in that mode.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: validate")
	// TODO(microvm-validate-metric): /validate is a build-time hook that only
	// fires on the ephemeral test MicroVM during image build (and may be retried
	// on 503), never during a production instance's run/resume lifecycle. This
	// metric therefore reflects build-step activity, not production behavior, and
	// may not reliably flush before the test VM is torn down. Revisit whether it
	// carries enough value to keep alongside the genuine runtime lifecycle metrics.
	s.emitLifecycleMetric(validateMetricName)
	if s.fwd != nil {
		s.passThroughValidate(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) passThroughValidate(w http.ResponseWriter, r *http.Request) {
	resp := s.fwd.PassThroughWaiting(s.fwd.validateTimeout, pathValidate, r.Header, r.Body)
	defer resp.Body.Close()
	mirrorResponse(w, resp)
}

// mirrorResponse writes the user app's status, Content-Type (if set), and
// body to the platform-facing ResponseWriter. Used by every pass-through path.
//
// Only Content-Type is forwarded: without it, Go's ResponseWriter.Write would
// sniff the first 512 bytes via http.DetectContentType and may misidentify the
// body (e.g. a JSON error payload detected as text/plain). See
// https://pkg.go.dev/net/http#ResponseWriter — "If the Header does not contain
// a Content-Type line, Write adds a Content-Type set to the result of passing
// the initial 512 bytes of written data to DetectContentType."
// Other response headers (Set-Cookie, cache directives, custom headers) are
// irrelevant to the platform's machine-to-machine lifecycle calls and are
// intentionally dropped.
func mirrorResponse(w http.ResponseWriter, resp *http.Response) {
	// Bound the write to the platform so a slow or stalled connection does not
	// block the handler indefinitely. mirrorResponseTimeout is sized to cover
	// the io.Copy below; it is also reflected in the server's WriteTimeout.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(mirrorResponseTimeout)); err != nil {
		log.Warnf("MicroVM lifecycle: could not set write deadline on mirror response: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Warnf("MicroVM lifecycle: error copying response body to platform: %v", err)
	}
}

// flushAll runs metric/trace/logs flushes in parallel and waits for them,
// bounded by flushCtx. Used by handleWithForwarder for the env-var-set path
// and by handleSuspend / handleTerminate for the env-var-unset path.
// It first drains any pending metric samples so that lifecycle metrics
// emitted immediately before this call are included in the flush.
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
	go func() {
		if s.logsFlusher != nil {
			s.logsFlusher.Flush(flushCtx)
		}
		flushDone <- struct{}{}
	}()
	s.waitForFlushes(flushCtx, flushDone)
}

// handleWithForwarder runs the agent-side path (metric emission, optionally
// followed by flush) in a goroutine while a synchronous PassThrough waits
// for the user app's response, then joins and mirrors the user app's
// response back to the platform.
//
// Used for /run and /resume (noFlush), /suspend (flushParallel), and
// /terminate (flushSequential). /ready and /validate use passThroughReady
// and passThroughValidate directly (TCP-wait path, not this function).
//
// flushParallel: flush runs concurrently with the forward; wall-clock is
// max(forwardTimeout, flushTimeout)+ε. Known limitation: telemetry produced
// by the user app during PassThrough may arrive in the pipeline after
// Flush() returns and be missed. Residual items survive a suspend→resume
// cycle (Firecracker preserves process memory), so the race is recoverable.
//
// flushSequential: flush runs after PassThrough returns; wall-clock is
// forwardTimeout+flushTimeout. Eliminates the race at the cost of higher
// latency. Used for /terminate where in-memory buffers are discarded with
// the VM and residual items cannot be recovered.
//
// flushCtx is deliberately NOT derived from r.Context(): if the platform
// disconnects mid-handler, the flush still runs to its own budget.
func (s *Server) handleWithForwarder(metricName, path string, mode flushMode, w http.ResponseWriter, r *http.Request) {
	sideDone := make(chan struct{})

	var parallelCtx context.Context
	var cancelParallel context.CancelFunc
	if mode == flushParallel {
		parallelCtx, cancelParallel = context.WithTimeout(context.Background(), s.flushTimeout)
		defer cancelParallel()
	}

	go func() {
		defer close(sideDone)
		s.emitLifecycleMetric(metricName)
		if mode == flushParallel {
			s.flushAll(parallelCtx)
		}
	}()

	resp := s.fwd.PassThrough(path, r.Header, r.Body)

	// Buffer the response body immediately, before waiting on the flush.
	// PassThrough returns as soon as response headers arrive; the body is
	// still tied to the forwardTimeout context (via cancelOnCloseReader).
	// Both flushParallel (/suspend) and flushSequential (/terminate) can run
	// longer than forwardTimeout (default 1s vs flushTimeout default 5s), so
	// waiting on sideDone/flushAll before reading resp.Body risks the context
	// firing mid-read and making the io.Copy in mirrorResponse fail with a
	// partial or empty body. Buffering here — while the context is fresh —
	// makes mirrorResponse context-independent. The body is already capped at
	// 1 MiB by limitedReadCloser, so io.ReadAll cannot allocate unboundedly.
	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Debugf("MicroVM lifecycle: partial %s response body (%d bytes read): %v", path, len(bodyBytes), err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	<-sideDone

	if mode == flushSequential {
		seqCtx, cancel := context.WithTimeout(context.Background(), s.flushTimeout)
		defer cancel()
		s.flushAll(seqCtx)
	}

	mirrorResponse(w, resp)
}

// aliveCheckReady answers /ready when no user-app forwarder is configured.
// Binary mapping: child alive → 200, anything else → 503. The platform's
// retry behavior absorbs the pre-spawn window, so we don't wait here.
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

// handleRun emits the run metric, captures the MicroVM instance ID
// from the platform's request body, starts the periodic heartbeat, and
// (when a forwarder is configured) mirrors the user app's response.
// handleRun is the only hook that reads r.Body and is therefore not
// collapsed into dispatchHook directly. The ID is captured before Start
// so the first heartbeat emission already carries the correct microvm_id tag.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	// Read the body once so we can parse the instance ID AND still forward the
	// original payload to the user app. Without this, the forwarder path would
	// consume r.Body before the decode, losing the instance_id tag on all
	// subsequent lifecycle metrics.
	//
	// MaxBytesReader bounds the read so a misconfigured runHookPayload cannot
	// force unbounded memory growth; a read error (oversized or otherwise)
	// aborts before any state is mutated, since forwarding a silently
	// truncated body would be worse than failing the hook outright.
	r.Body = http.MaxBytesReader(w, r.Body, maxRunBodyBytes)
	bodyBytes, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		log.Debugf("MicroVM lifecycle: could not read run body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var body runBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		log.Debugf("MicroVM lifecycle: could not parse run body: %v", err)
	}
	if body.MicroVMID != "" {
		log.Infof("MicroVM lifecycle: run (microvm_id=%s)", body.MicroVMID)
		s.instanceID.Store(body.MicroVMID)
		s.heartbeat.SetMicroVMID(body.MicroVMID)
		if s.logsTagSetter != nil {
			s.logsTagSetter.SetLogsTags(append(append([]string{}, s.baseTags...), lambdaMicroVMID+body.MicroVMID))
		}
		if s.traceTagSetter != nil {
			tags := maps.Clone(s.baseTraceTags)
			if tags == nil {
				tags = make(map[string]string, 1)
			}
			tags[lambdaMicroVMIDKey] = body.MicroVMID
			s.traceTagSetter.SetTraceTags(tags)
		}
	} else {
		log.Info("MicroVM lifecycle: run")
	}
	s.heartbeat.Start()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	// Go strips Content-Length from server request headers into r.ContentLength
	// so the forwarder's header-based Content-Length path in do() sees an empty
	// string. Put it back now that we know the exact buffered length, so the
	// forwarded /run request carries a fixed Content-Length rather than
	// chunked encoding (which some user-app HTTP servers reject).
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	s.dispatchHook(runMetricName, pathRun, noFlush, w, r)
}

// handleResume emits the resume metric, restarts the heartbeat (stopped at
// /suspend), and (when a forwarder is configured) mirrors the user app's response.
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: resume")
	s.heartbeat.Start()
	s.dispatchHook(resumeMetricName, pathResume, noFlush, w, r)
}

// handleSuspend stops the heartbeat, flushes telemetry, and (when a forwarder
// is configured) mirrors the user app's response. Heartbeat is stopped before
// flushing so any in-flight tick finishes before the metric agent drains;
// /resume restarts it.
//
// Flush mode rationale: flushParallel is used here because Firecracker
// snapshots the full process memory, preserving any pipeline-buffer items that
// the parallel flush misses. Those residual items are drained by the agent's
// periodic flush on the next resume. /terminate uses flushSequential (which
// buffers the response body, waits for the user app to finish writing it, then
// flushes) because the VM is torn down with no recovery path.
//
// TODO(platform-contract): verify whether the AWS MicroVM platform guarantees
// that /terminate is always called before a suspended VM is permanently
// discarded (e.g., snapshot eviction, spot termination, platform crash).
// If /terminate is NOT guaranteed after /suspend, residual pipeline-buffer
// items that survived the parallel flush on /suspend will be lost with no
// recovery path — identical to the /terminate data-loss risk we already
// address with flushSequential.
//
// If the guarantee cannot be confirmed, change flushParallel → flushSequential
// here. The implementation already supports it (handleWithForwarder and
// dispatchHook both handle flushSequential). The impact of that change:
//   - /suspend wall-clock increases from max(forwardTimeout, flushTimeout) to
//     forwardTimeout + flushTimeout (default: 1s + 5s = 6s vs 5s today).
//   - The response body from the user app is fully buffered before flushing
//     (same as /terminate), so logs emitted by the user app while writing its
//     suspend response are included in the flush rather than risking the race.
//   - Server WriteTimeout must be updated: the formula in NewServer already
//     uses forwardTimeout+flushTimeout for the sequential path; adding /suspend
//     to that path requires no additional change to the WriteTimeout formula
//     since /terminate already sets the ceiling.
func (s *Server) handleSuspend(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: suspend — flushing telemetry")
	s.heartbeat.Stop()
	s.dispatchHook(suspendMetricName, pathSuspend, flushParallel, w, r)
}

// handleTerminate flushes all telemetry and, when a forwarder is configured,
// mirrors the user app's response.
//
// No synthetic SIGTERM is sent. AWS Lambda MicroVM does not promise a
// separate SIGTERM (the 200 IS the termination trigger and Firecracker
// destroys the VM on response or at the 60s platform deadline); the agent
// must not simulate a signal channel that the platform does not provide.
// User apps that opt in receive /terminate via pass-through and own their
// own graceful exit; users without a /terminate handler rely on the
// platform's own termination of the VM. Heartbeat is stopped here so it
// cannot fire after the VM is torn down.
func (s *Server) handleTerminate(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: terminate — flushing telemetry")
	s.heartbeat.Stop()
	s.dispatchHook(terminateMetricName, pathTerminate, flushSequential, w, r)
}

// dispatchHook is the shared dispatch path for run, resume, suspend, and
// terminate. When a forwarder is configured (DD_AWS_MICROVM_USER_APP_PORT
// set): delegates to handleWithForwarder which mirrors the user app's response.
// When no forwarder is configured: emit metric, optionally flush, return 200.
// In standalone mode the parallel/sequential distinction does not apply, so
// both flush modes result in a single flushAll call.
func (s *Server) dispatchHook(metricName, path string, mode flushMode, w http.ResponseWriter, r *http.Request) {
	if s.fwd != nil {
		s.handleWithForwarder(metricName, path, mode, w, r)
		return
	}
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
