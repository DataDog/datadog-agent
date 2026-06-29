// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lifecycle implements the AWS Lambda MicroVM lifecycle hook server.
// The MicroVM platform calls POST endpoints on port 9000 at key points in
// the MicroVM lifecycle: ready, launch, resume, suspend, terminate. This
// server handles those hooks in two modes:
//
//   - When DD_SERVERLESS_MICROVM_USER_APP_PORT is unset: the agent
//     responds to each hook itself (200), emitting an enhanced metric and,
//     for /suspend and /terminate, flushing telemetry before responding.
//
//   - When the env var is set: the agent forwards each hook to
//     127.0.0.1:<user-app-port> on the same path and mirrors the user
//     app's response (status, body, Content-Type) back to the platform.
//     The agent's own work — metric emission, /suspend and /terminate
//     telemetry flush — runs in a goroutine in parallel with the
//     pass-through.
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
	"net"
	"net/http"
	"time"

	"go.uber.org/atomic"

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
	MicroVMID string `json:"microVmId"`
}

// Server is the MicroVM lifecycle hook HTTP server.
type Server struct {
	metricFlusher Flusher
	traceFlusher  Flusher
	logsFlusher   LogsFlusher
	metricEmitter MetricEmitter
	sampleDrainer SampleDrainer
	instanceID    *atomic.String // set once from /launch body
	metricSource  metrics.MetricSource
	flushTimeout  time.Duration

	childHandle ChildHandle // production: *Child (init) or NewNoopChildHandle() (sidecar); always non-nil after lifecycle.SetupFromEnv. nil only in legacy unit tests; logs WARN if hit.
	child       *Child      // non-nil only in init-container mode; nil in sidecar and unit tests. Derived from childHandle when it is a *Child.
	fwd         *Forwarder  // nil = no opt-in; today's behavior preserved
	heartbeat   *Heartbeat  // nil-safe; nil disables periodic heartbeat emission

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
	// WriteTimeout must cover the full handler wall-clock: for flush-only paths
	// (no forwarder) that is flushTimeout; for pass-through paths the forwarder
	// can hold the connection open for up to forwardTimeout (default 30s), which
	// exceeds flushTimeout (default 5s). Use the larger of the two so the HTTP
	// server does not close the platform-facing connection before the handler
	// can write the mirrored response.
	writeTimeout := s.flushTimeout + 10*time.Second
	if s.fwd != nil && s.fwd.forwardTimeout+10*time.Second > writeTimeout {
		writeTimeout = s.fwd.forwardTimeout + 10*time.Second
	}
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.handler(),
		ReadTimeout:  30 * time.Second,
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
	log.Warnf("Port %d is reserved for the MicroVM lifecycle server — ensure your application does not bind this port", DefaultPort)
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
	mux.HandleFunc(pathReady, s.handleReady)
	mux.HandleFunc(pathLaunch, s.handleLaunch)
	mux.HandleFunc(pathSuspend, s.handleSuspend)
	mux.HandleFunc(pathResume, s.handleResume)
	mux.HandleFunc(pathTerminate, s.handleTerminate)
	return mux
}

// handleReady signals to the platform whether the agent (and, optionally,
// the user app) is ready. No telemetry is flushed here; it is a readiness
// signal only.
//
// Dispatcher:
//   - If a Forwarder is configured (env-var opt-in), strict pass-through to
//     the user app: dial errors map to 503, deadline to 504.
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
	resp := s.fwd.PassThroughReady(pathReady, r.Header, r.Body)
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
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Warnf("MicroVM lifecycle: error copying response body to platform: %v", err)
	}
}

// flushAll runs metric/trace/logs flushes in parallel and waits for them,
// bounded by flushCtx. Used by handleParallel for the env-var-set path
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
	flushDone := make(chan struct{}, 3)
	go func() { s.metricFlusher.Flush(); flushDone <- struct{}{} }()
	go func() { s.traceFlusher.Flush(); flushDone <- struct{}{} }()
	go func() { s.logsFlusher.Flush(flushCtx); flushDone <- struct{}{} }()
	s.waitForFlushes(flushCtx, flushDone)
}

// handleParallel runs the agent-side path (metric emission, optionally
// followed by flush) in a goroutine while a synchronous PassThrough waits
// for the user app's response, then joins and mirrors the user app's
// response back to the platform.
//
// Used for /launch and /resume (withFlush=false) and for /suspend and
// /terminate (withFlush=true). /ready uses passThroughReady directly.
//
// Wall-clock is bounded by max(forwardTimeout, flushTimeout)+ε. flushCtx is
// deliberately NOT derived from r.Context(): if the platform disconnects
// mid-handler, the flush goroutine continues to its own budget so telemetry
// is preserved. The handler never imposes its own outer deadline; the join
// is unconditional.
func (s *Server) handleParallel(metricName, path string, withFlush bool, w http.ResponseWriter, r *http.Request) {
	flushCtx, cancel := context.WithTimeout(context.Background(), s.flushTimeout)
	defer cancel()

	sideDone := make(chan struct{})
	go func() {
		defer close(sideDone)
		s.emitLifecycleMetric(metricName)
		if withFlush {
			s.flushAll(flushCtx)
		}
	}()

	resp := s.fwd.PassThrough(path, r.Header, r.Body)
	defer resp.Body.Close()

	<-sideDone

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

// handleLaunch emits the launch metric, captures the MicroVM instance ID
// from the platform's request body, starts the periodic heartbeat, and
// (when a forwarder is configured) mirrors the user app's response.
// handleLaunch is the only hook that reads r.Body and is therefore not
// collapsed into dispatchHook directly. The ID is captured before Start
// so the first heartbeat emission already carries the correct microvm_id tag.
func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: launch")
	// Read the body once so we can parse the instance ID AND still forward the
	// original payload to the user app. Without this, the forwarder path would
	// consume r.Body before the decode, losing the instance_id tag on all
	// subsequent lifecycle metrics.
	bodyBytes, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	var body launchBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		log.Debugf("MicroVM lifecycle: could not parse launch body: %v", err)
	}
	if body.MicroVMID != "" {
		s.instanceID.Store(body.MicroVMID)
		s.heartbeat.SetMicroVMID(body.MicroVMID)
	}
	s.heartbeat.Start()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	s.dispatchHook(launchMetricName, pathLaunch, false, w, r)
}

// handleResume emits the resume metric, restarts the heartbeat (stopped at
// /suspend), and (when a forwarder is configured) mirrors the user app's response.
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: resume")
	s.heartbeat.Start()
	s.dispatchHook(resumeMetricName, pathResume, false, w, r)
}

// handleSuspend stops the heartbeat, flushes telemetry, and (when a forwarder
// is configured) mirrors the user app's response. Heartbeat is stopped before
// flushing so any in-flight tick finishes before the metric agent drains;
// /resume restarts it.
func (s *Server) handleSuspend(w http.ResponseWriter, r *http.Request) {
	log.Info("MicroVM lifecycle: suspend — flushing telemetry")
	s.heartbeat.Stop()
	s.dispatchHook(suspendMetricName, pathSuspend, true, w, r)
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
	s.dispatchHook(terminateMetricName, pathTerminate, true, w, r)
}

// dispatchHook is the shared dispatch path for launch, resume, suspend, and
// terminate. When a forwarder is configured (DD_SERVERLESS_MICROVM_USER_APP_PORT
// set): metric emission and optional flush run in a goroutine in parallel with
// a pass-through to the user app, then the user app's response is mirrored back
// to the platform. When no forwarder is configured: emit metric, optionally
// flush telemetry, return 200.
func (s *Server) dispatchHook(metricName, path string, withFlush bool, w http.ResponseWriter, r *http.Request) {
	if s.fwd != nil {
		s.handleParallel(metricName, path, withFlush, w, r)
		return
	}
	s.emitLifecycleMetric(metricName)
	if withFlush {
		flushCtx, cancel := context.WithTimeout(context.Background(), s.flushTimeout)
		defer cancel()
		s.flushAll(flushCtx)
	}
	w.WriteHeader(http.StatusOK)
}

// emitLifecycleMetric records a lifecycle event metric, appending the stored
// MicroVM instance ID tag when one has been captured from the /launch body.
func (s *Server) emitLifecycleMetric(name string) {
	var extraTags []string
	if id := s.instanceID.Load(); id != "" {
		extraTags = []string{instanceIDTagPrefix + id}
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

func (s *Server) waitForFlushes(flushCtx context.Context, flushDone <-chan struct{}) {
	for i := 0; i < 3; i++ {
		select {
		case <-flushDone:
		case <-flushCtx.Done():
			log.Warnf("MicroVM lifecycle: flush timed out after %s", s.flushTimeout)
			return
		}
	}
	log.Info("MicroVM lifecycle: flush complete")
}
