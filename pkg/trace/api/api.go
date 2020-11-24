// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"math"
	"mime"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gorilla "github.com/gorilla/mux"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_runtime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/soheilhy/cmux"
	"github.com/tinylib/msgp/msgp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	mainconfig "github.com/DataDog/datadog-agent/pkg/config"
	remote_flare "github.com/DataDog/datadog-agent/pkg/flare/remote"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	pb "github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HTTPReceiver is a collector that uses HTTP protocol and just holds
// a chan where the spans received are sent one by one
type HTTPReceiver struct {
	Stats       *info.ReceiverStats
	RateLimiter *rateLimiter

	out       chan *Payload
	conf      *config.AgentConfig
	dynConf   *sampler.DynamicConfig
	server    *http.Server
	tlsServer *http.Server

	debug               bool
	rateLimiterResponse int // HTTP status code when refusing

	wg   sync.WaitGroup // waits for all requests to be processed
	exit chan struct{}
}

// NewHTTPReceiver returns a pointer to a new HTTPReceiver
func NewHTTPReceiver(conf *config.AgentConfig, dynConf *sampler.DynamicConfig, out chan *Payload) *HTTPReceiver {
	rateLimiterResponse := http.StatusOK
	if config.HasFeature("429") {
		rateLimiterResponse = http.StatusTooManyRequests
	}
	return &HTTPReceiver{
		Stats:       info.NewReceiverStats(),
		RateLimiter: newRateLimiter(),
		out:         out,

		conf:    conf,
		dynConf: dynConf,

		debug:               strings.ToLower(conf.LogLevel) == "debug",
		rateLimiterResponse: rateLimiterResponse,

		exit: make(chan struct{}),
	}
}

// Start starts doing the HTTP server and is ready to receive traces
func (r *HTTPReceiver) Start() {

	addr := fmt.Sprintf("%s:%d", r.conf.ReceiverHost, r.conf.ReceiverPort)

	ln, err := r.listenTCP(addr)
	if err != nil {
		killProcess("Error creating tcp listener: %v", err)
	}

	tcpm := cmux.New(ln)

	// Else, we assume TLS
	tlsl := tcpm.Match(cmux.TLS())
	// We first match on HTTP 1.1 methods.
	httpl := tcpm.Match(cmux.Any())

	go func() {
		defer watchdog.LogOnPanic()
		r.ServeHTTP(httpl)
	}()
	go func() {
		// do we need a watchdog?
		r.ServeGRPC(tlsl)
	}()
	log.Infof("Listening for traces at http://%s", addr)

	// UDS
	if path := r.conf.ReceiverSocket; path != "" {
		ln, err := r.listenUnix(path)
		if err != nil {
			killProcess("Error creating UDS listener: %v", err)
		}
		go func() {
			defer watchdog.LogOnPanic()
			r.ServeHTTP(ln)
		}()
		log.Infof("Listening for traces at unix://%s", path)
	}

	go r.RateLimiter.Run()

	go func() {
		defer watchdog.LogOnPanic()
		r.loop()
	}()

	// start cmux serving
	go func() {
		if err := tcpm.Serve(); !strings.Contains(err.Error(), "use of closed network connection") {
			panic(err)
		}
	}()
}

func (r *HTTPReceiver) attachHandlers(m *http.ServeMux) *http.ServeMux {
	m.HandleFunc("/spans", r.handleWithVersion(v01, r.handleTraces))
	m.HandleFunc("/services", r.handleWithVersion(v01, r.handleServices))
	m.HandleFunc("/v0.1/spans", r.handleWithVersion(v01, r.handleTraces))
	m.HandleFunc("/v0.1/services", r.handleWithVersion(v01, r.handleServices))
	m.HandleFunc("/v0.2/traces", r.handleWithVersion(v02, r.handleTraces))
	m.HandleFunc("/v0.2/services", r.handleWithVersion(v02, r.handleServices))
	m.HandleFunc("/v0.3/traces", r.handleWithVersion(v03, r.handleTraces))
	m.HandleFunc("/v0.3/services", r.handleWithVersion(v03, r.handleServices))
	m.HandleFunc("/v0.4/traces", r.handleWithVersion(v04, r.handleTraces))
	m.HandleFunc("/v0.4/services", r.handleWithVersion(v04, r.handleServices))
	m.HandleFunc("/v0.5/traces", r.handleWithVersion(v05, r.handleTraces))
	m.Handle("/profiling/v1/input", r.profileProxyHandler())

	return m
}

func (r *HTTPReceiver) attachDebugHandlers(m *http.ServeMux) *http.ServeMux {
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)

	m.HandleFunc("/debug/blockrate", func(w http.ResponseWriter, r *http.Request) {
		// this endpoint calls runtime.SetBlockProfileRate(v), where v is an optional
		// query string parameter defaulting to 10000 (1 sample per 10μs blocked).
		rate := 10000
		v := r.URL.Query().Get("v")
		if v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, "v must be an integer", http.StatusBadRequest)
				return
			}
			rate = n
		}
		runtime.SetBlockProfileRate(rate)
		w.Write([]byte(fmt.Sprintf("Block profile rate set to %d. It will automatically be disabled again after calling /debug/pprof/block\n", rate)))
	})

	m.HandleFunc("/debug/pprof/block", func(w http.ResponseWriter, r *http.Request) {
		// serve the block profile and reset the rate to 0.
		pprof.Handler("block").ServeHTTP(w, r)
		runtime.SetBlockProfileRate(0)
	})

	m.Handle("/debug/vars", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// allow the GUI to call this endpoint so that the status can be reported
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:"+mainconfig.Datadog.GetString("GUI_port"))
		expvar.Handler().ServeHTTP(w, req)
	}))

	return m
}

// SetupHandlers adds the specific handlers for /agent endpoints
func (r *HTTPReceiver) attachRESTHandlers(mux *gorilla.Router) *gorilla.Router {
	mux.HandleFunc("/flare/{type}/{op}", remote_flare.Handler).Methods("POST")
	mux.HandleFunc("/flare/log/{flare_id}/{tracer_id}/{type}", remote_flare.LogHandler).Methods("POST")

	remote_flare.InitAPI(r.conf.LogFilePath, "", "", "")

	return mux
}

func (r *HTTPReceiver) ServeHTTP(l net.Listener) {
	// Setup multiplexer
	mux := http.NewServeMux()

	r.attachDebugHandlers(mux)
	r.attachHandlers(mux)

	timeout := 5 * time.Second
	if r.conf.ReceiverTimeout > 0 {
		timeout = time.Duration(r.conf.ReceiverTimeout) * time.Second
	}
	httpLogger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	r.server = &http.Server{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		ErrorLog:     stdlog.New(httpLogger, "http.Server: ", 0),
		Handler:      mux,
	}

	log.Infof("Starting HTTP Server...")
	r.server.Serve(l)

}

func (r *HTTPReceiver) ServeGRPC(l net.Listener) {

	addr := l.Addr().String()
	log.Infof("TLS listener will be configured for: %v", addr)
	hosts := []string{"127.0.0.1", "localhost", "::1", addr}
	tlsKeyPair, tlsCertPool := security.InitializeTLS(hosts)

	// grpc stuff
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, addr)),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(security.GrpcAuth)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(security.GrpcAuth)),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterFlareServer(s, &grpcServer{})

	dcreds := credentials.NewTLS(&tls.Config{
		ServerName: addr,
		RootCAs:    tlsCertPool,
	})
	dopts := []grpc.DialOption{grpc.WithTransportCredentials(dcreds)}

	// starting grpc gateway
	ctx := context.Background()
	gwmux := grpc_runtime.NewServeMux()
	err := pb.RegisterFlareHandlerFromEndpoint(
		ctx, gwmux, addr, dopts)
	if err != nil {
		panic(err)
	}

	// create the REST HTTP router
	restMux := gorilla.NewRouter()

	tlsMux := http.NewServeMux()
	tlsMux.Handle("/rest/", http.StripPrefix("/rest", r.attachRESTHandlers(restMux)))
	tlsMux.Handle("/", gwmux) // grpc-gateway for all other requests

	timeout := 5 * time.Second
	if r.conf.ReceiverTimeout > 0 {
		timeout = time.Duration(r.conf.ReceiverTimeout) * time.Second
	}
	httpLogger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds

	r.tlsServer = &http.Server{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		ErrorLog:     stdlog.New(httpLogger, "https.Server: ", 0),
		Handler:      grpcHandlerFunc(s, tlsMux),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*tlsKeyPair},
			NextProtos:   []string{"h2"},
		},
	}

	// Create TLS listener
	tlsl := tls.NewListener(l, r.tlsServer.TLSConfig)

	log.Infof("Starting TLS Server...")
	r.tlsServer.Serve(tlsl)
}

// listenUnix returns a net.Listener listening on the given "unix" socket path.
func (r *HTTPReceiver) listenUnix(path string) (net.Listener, error) {
	fi, err := os.Stat(path)
	if err == nil {
		// already exists
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("cannot reuse %q; not a unix socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("unable to remove stale socket: %v", err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0722); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	return ln, err
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise. Copied from cockroachdb.
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// This is a partial recreation of gRPC's internal checks https://github.com/grpc/grpc-go/pull/514/files#diff-95e9a25b738459a2d3030e1e6fa2a718R61
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

// listenTCP creates a new net.Listener on the provided TCP address.
func (r *HTTPReceiver) listenTCP(addr string) (net.Listener, error) {
	tcpln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if climit := r.conf.ConnectionLimit; climit > 0 {
		ln, err := newRateLimitedListener(tcpln, climit)
		go func() {
			defer watchdog.LogOnPanic()
			ln.Refresh(climit)
		}()
		return ln, err
	}
	return tcpln, err
}

// Stop stops the receiver and shuts down the HTTP server.
func (r *HTTPReceiver) Stop() error {
	r.exit <- struct{}{}
	<-r.exit

	r.RateLimiter.Stop()

	expiry := time.Now().Add(5 * time.Second) // give it 5 seconds
	ctx, cancel := context.WithDeadline(context.Background(), expiry)
	defer cancel()
	if err := r.server.Shutdown(ctx); err != nil {
		return err
	}
	r.wg.Wait()
	close(r.out)
	return nil
}

func (r *HTTPReceiver) handleWithVersion(v Version, f func(Version, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if mediaType := getMediaType(req); mediaType == "application/msgpack" && (v == v01 || v == v02) {
			// msgpack is only supported for versions >= v0.3
			httpFormatError(w, v, fmt.Errorf("unsupported media type: %q", mediaType))
			return
		}

		// TODO(x): replace with http.MaxBytesReader?
		req.Body = NewLimitedReader(req.Body, r.conf.MaxRequestBytes)

		f(v, w, req)
	}
}

func traceCount(req *http.Request) (int64, error) {
	if _, ok := req.Header[headerTraceCount]; !ok {
		return 0, fmt.Errorf("HTTP header %q not found", headerTraceCount)
	}
	str := req.Header.Get(headerTraceCount)
	if str == "" {
		return 0, fmt.Errorf("HTTP header %q value not set", headerTraceCount)
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0, fmt.Errorf("HTTP header %q can not be parsed: %v", headerTraceCount, err)
	}
	return int64(n), nil
}

const (
	// headerTraceCount is the header client implementation should fill
	// with the number of traces contained in the payload.
	headerTraceCount = "X-Datadog-Trace-Count"

	// headerContainerID specifies the name of the header which contains the ID of the
	// container where the request originated.
	headerContainerID = "Datadog-Container-ID"

	// headerLang specifies the name of the header which contains the language from
	// which the traces originate.
	headerLang = "Datadog-Meta-Lang"

	// headerLangVersion specifies the name of the header which contains the origin
	// language's version.
	headerLangVersion = "Datadog-Meta-Lang-Version"

	// headerLangInterpreter specifies the name of the HTTP header containing information
	// about the language interpreter, where applicable.
	headerLangInterpreter = "Datadog-Meta-Lang-Interpreter"

	// headerLangInterpreterVendor specifies the name of the HTTP header containing information
	// about the language interpreter vendor, where applicable.
	headerLangInterpreterVendor = "Datadog-Meta-Lang-Interpreter-Vendor"

	// headerTracerVersion specifies the name of the header which contains the version
	// of the tracer sending the payload.
	headerTracerVersion = "Datadog-Meta-Tracer-Version"
)

func (r *HTTPReceiver) tagStats(v Version, req *http.Request) *info.TagStats {
	return r.Stats.GetTagStats(info.Tags{
		Lang:            req.Header.Get(headerLang),
		LangVersion:     req.Header.Get(headerLangVersion),
		Interpreter:     req.Header.Get(headerLangInterpreter),
		LangVendor:      req.Header.Get(headerLangInterpreterVendor),
		TracerVersion:   req.Header.Get(headerTracerVersion),
		EndpointVersion: string(v),
	})
}

func decodeTraces(v Version, req *http.Request) (pb.Traces, error) {
	switch v {
	case v01:
		var spans []pb.Span
		if err := json.NewDecoder(req.Body).Decode(&spans); err != nil {
			return nil, err
		}
		return tracesFromSpans(spans), nil
	case v05:
		var traces pb.Traces
		rd := pb.NewMsgpReader(req.Body)
		err := traces.DecodeMsgDictionary(rd)
		pb.FreeMsgpReader(rd)
		return traces, err
	default:
		var traces pb.Traces
		if err := decodeRequest(req, &traces); err != nil {
			return nil, err
		}
		return traces, nil
	}
}

func (r *HTTPReceiver) replyOK(v Version, w http.ResponseWriter) {
	switch v {
	case v01, v02, v03:
		httpOK(w)
	default:
		httpRateByService(w, r.dynConf)
	}
}

// rateLimited reports whether n number of traces should be rejected by the API.
func (r *HTTPReceiver) rateLimited(n int64) bool {
	if n == 0 {
		return false
	}
	if r.conf.MaxMemory == 0 && r.conf.MaxCPU == 0 {
		// rate limiting is off
		return false
	}
	return !r.RateLimiter.Permits(n)
}

// handleTraces knows how to handle a bunch of traces
func (r *HTTPReceiver) handleTraces(v Version, w http.ResponseWriter, req *http.Request) {
	ts := r.tagStats(v, req)
	tracen, err := traceCount(req)
	if err == nil && r.rateLimited(tracen) {
		// this payload can not be accepted
		io.Copy(ioutil.Discard, req.Body)
		w.WriteHeader(r.rateLimiterResponse)
		r.replyOK(v, w)
		atomic.AddInt64(&ts.PayloadRefused, 1)
		return
	}

	traces, err := decodeTraces(v, req)
	if err != nil {
		httpDecodingError(err, []string{"handler:traces", fmt.Sprintf("v:%s", v)}, w)
		switch err {
		case ErrLimitedReaderLimitReached:
			atomic.AddInt64(&ts.TracesDropped.PayloadTooLarge, tracen)
		case io.EOF, io.ErrUnexpectedEOF:
			atomic.AddInt64(&ts.TracesDropped.EOF, tracen)
		default:
			if err, ok := err.(net.Error); ok && err.Timeout() {
				atomic.AddInt64(&ts.TracesDropped.Timeout, tracen)
			} else {
				atomic.AddInt64(&ts.TracesDropped.DecodingError, tracen)
			}
		}
		log.Errorf("Cannot decode %s traces payload: %v", v, err)
		return
	}
	r.replyOK(v, w)

	atomic.AddInt64(&ts.TracesReceived, int64(len(traces)))
	atomic.AddInt64(&ts.TracesBytes, req.Body.(*LimitedReader).Count)
	atomic.AddInt64(&ts.PayloadAccepted, 1)

	payload := &Payload{
		Source:        ts,
		Traces:        traces,
		ContainerTags: getContainerTags(req.Header.Get(headerContainerID)),
	}
	select {
	case r.out <- payload:
		// ok
	default:
		// channel blocked, add a goroutine to ensure we never drop
		r.wg.Add(1)
		go func() {
			metrics.Count("datadog.trace_agent.receiver.queued_send", 1, nil, 1)
			defer func() {
				r.wg.Done()
				watchdog.LogOnPanic()
			}()
			r.out <- payload
		}()
	}
}

// Payload specifies information about a set of traces received by the API.
type Payload struct {
	// Source specifies information about the source of these traces, such as:
	// language, interpreter, tracer version, etc.
	Source *info.TagStats

	// ContainerTags specifies orchestrator tags corresponding to the origin of this
	// trace (e.g. K8S pod, Docker image, ECS, etc). They are of the type "k1:v1,k2:v2".
	ContainerTags string

	// Traces contains all the traces received in the payload
	Traces pb.Traces
}

// handleServices handle a request with a list of several services
func (r *HTTPReceiver) handleServices(v Version, w http.ResponseWriter, req *http.Request) {
	httpOK(w)

	// Do nothing, services are no longer being sent to Datadog as of July 2019
	// and are now automatically extracted from traces.
}

// loop periodically submits stats about the receiver to statsd
func (r *HTTPReceiver) loop() {
	defer close(r.exit)

	var lastLog time.Time
	accStats := info.NewReceiverStats()

	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	tw := time.NewTicker(r.conf.WatchdogInterval)
	defer tw.Stop()

	for {
		select {
		case <-r.exit:
			return
		case now := <-tw.C:
			r.watchdog(now)
		case now := <-t.C:
			metrics.Gauge("datadog.trace_agent.heartbeat", 1, nil, 1)
			metrics.Gauge("datadog.trace_agent.receiver.out_chan_fill", float64(len(r.out))/float64(cap(r.out)), nil, 1)

			// We update accStats with the new stats we collected
			accStats.Acc(r.Stats)

			// Publish the stats accumulated during the last flush
			r.Stats.Publish()

			// We reset the stats accumulated during the last 10s.
			r.Stats.Reset()

			if now.Sub(lastLog) >= time.Minute {
				// We expose the stats accumulated to expvar
				info.UpdateReceiverStats(accStats)

				accStats.LogStats()

				// We reset the stats accumulated during the last minute
				accStats.Reset()
				lastLog = now

				// Also publish rates by service (they are updated by receiver)
				rates := r.dynConf.RateByService.GetAll()
				info.UpdateRateByService(rates)
			}
		}
	}
}

// killProcess exits the process with the given msg; replaced in tests.
var killProcess = func(format string, a ...interface{}) { osutil.Exitf(format, a...) }

// watchdog checks the trace-agent's heap and CPU usage and updates the rate limiter using a correct
// sampling rate to maintain resource usage within set thresholds. These thresholds are defined by
// the configuration MaxMemory and MaxCPU. If these values are 0, all limits are disabled and the rate
// limiter will accept everything.
func (r *HTTPReceiver) watchdog(now time.Time) {
	wi := watchdog.Info{
		Mem: watchdog.Mem(),
		CPU: watchdog.CPU(now),
	}
	rateMem := 1.0
	if r.conf.MaxMemory > 0 {
		if current, allowed := float64(wi.Mem.Alloc), r.conf.MaxMemory*1.5; current > allowed {
			// This is a safety mechanism: if the agent is using more than 1.5x max. memory, there
			// is likely a leak somewhere; we'll kill the process to avoid polluting host memory.
			metrics.Count("datadog.trace_agent.receiver.oom_kill", 1, nil, 1)
			metrics.Flush()
			log.Criticalf("Killing process. Memory threshold exceeded: %.2fM / %.2fM", current/1024/1024, allowed/1024/1024)
			killProcess("OOM")
		}
		rateMem = computeRateLimitingRate(r.conf.MaxMemory, float64(wi.Mem.Alloc), r.RateLimiter.RealRate())
		if rateMem < 1 {
			log.Warnf("Memory threshold exceeded (apm_config.max_memory: %.0f bytes): %d", r.conf.MaxMemory, wi.Mem.Alloc)
		}
	}
	rateCPU := 1.0
	if r.conf.MaxCPU > 0 {
		rateCPU = computeRateLimitingRate(r.conf.MaxCPU, wi.CPU.UserAvg, r.RateLimiter.RealRate())
		if rateCPU < 1 {
			log.Warnf("CPU threshold exceeded (apm_config.max_cpu_percent: %.0f): %.0f", r.conf.MaxCPU*100, wi.CPU.UserAvg)
		}
	}

	r.RateLimiter.SetTargetRate(math.Min(rateCPU, rateMem))

	stats := r.RateLimiter.Stats()

	info.UpdateRateLimiter(*stats)
	info.UpdateWatchdogInfo(wi)

	metrics.Gauge("datadog.trace_agent.heap_alloc", float64(wi.Mem.Alloc), nil, 1)
	metrics.Gauge("datadog.trace_agent.cpu_percent", wi.CPU.UserAvg*100, nil, 1)
	metrics.Gauge("datadog.trace_agent.receiver.ratelimit", stats.TargetRate, nil, 1)
}

// Languages returns the list of the languages used in the traces the agent receives.
func (r *HTTPReceiver) Languages() string {
	// We need to use this map because we can have several tags for a same language.
	langs := make(map[string]bool)
	str := []string{}

	r.Stats.RLock()
	for tags := range r.Stats.Stats {
		if _, ok := langs[tags.Lang]; !ok {
			str = append(str, tags.Lang)
			langs[tags.Lang] = true
		}
	}
	r.Stats.RUnlock()

	sort.Strings(str)
	return strings.Join(str, "|")
}

func decodeRequest(req *http.Request, dest msgp.Decodable) error {
	switch mediaType := getMediaType(req); mediaType {
	case "application/msgpack":
		return msgp.Decode(req.Body, dest)
	case "application/json":
		fallthrough
	case "text/json":
		fallthrough
	case "":
		return json.NewDecoder(req.Body).Decode(dest)
	default:
		// do our best
		if err1 := json.NewDecoder(req.Body).Decode(dest); err1 != nil {
			if err2 := msgp.Decode(req.Body, dest); err2 != nil {
				return fmt.Errorf("could not decode JSON (%q), nor Msgpack (%q)", err1, err2)
			}
		}
		return nil
	}
}

func tracesFromSpans(spans []pb.Span) pb.Traces {
	traces := pb.Traces{}
	byID := make(map[uint64][]*pb.Span)
	for _, s := range spans {
		byID[s.TraceID] = append(byID[s.TraceID], &s)
	}
	for _, t := range byID {
		traces = append(traces, t)
	}

	return traces
}

// getContainerTag returns container and orchestrator tags belonging to containerID. If containerID
// is empty or no tags are found, an empty string is returned.
func getContainerTags(containerID string) string {
	list, err := tagger.Tag("container_id://"+containerID, collectors.HighCardinality)
	if err != nil {
		log.Tracef("Getting container tags for ID %q: %v", containerID, err)
		return ""
	}
	log.Tracef("Getting container tags for ID %q: %v", containerID, list)
	return strings.Join(list, ",")
}

// getMediaType attempts to return the media type from the Content-Type MIME header. If it fails
// it returns the default media type "application/json".
func getMediaType(req *http.Request) string {
	mt, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		log.Debugf(`Error parsing media type: %v, assuming "application/json"`, err)
		return "application/json"
	}
	return mt
}
