// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
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

	"github.com/tinylib/msgp/msgp"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/appsec"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	buffer := bufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	return buffer
}

func putBuffer(buffer *bytes.Buffer) {
	bufferPool.Put(buffer)
}

// HTTPReceiver is a collector that uses HTTP protocol and just holds
// a chan where the spans received are sent one by one
type HTTPReceiver struct {
	Stats       *info.ReceiverStats
	RateLimiter *rateLimiter

	out            chan *Payload
	conf           *config.AgentConfig
	dynConf        *sampler.DynamicConfig
	server         *http.Server
	statsProcessor StatsProcessor
	appsecHandler  http.Handler

	debug               bool
	rateLimiterResponse int // HTTP status code when refusing

	wg   sync.WaitGroup // waits for all requests to be processed
	exit chan struct{}
}

// NewHTTPReceiver returns a pointer to a new HTTPReceiver
func NewHTTPReceiver(conf *config.AgentConfig, dynConf *sampler.DynamicConfig, out chan *Payload, statsProcessor StatsProcessor) *HTTPReceiver {
	rateLimiterResponse := http.StatusOK
	if features.Has("429") {
		rateLimiterResponse = http.StatusTooManyRequests
	}
	appsecHandler, err := appsec.NewIntakeReverseProxy(conf)
	if err != nil {
		log.Errorf("Could not instantiate AppSec: %v", err)
	}
	return &HTTPReceiver{
		Stats:       info.NewReceiverStats(),
		RateLimiter: newRateLimiter(),

		out:            out,
		statsProcessor: statsProcessor,
		conf:           conf,
		dynConf:        dynConf,
		appsecHandler:  appsecHandler,

		debug:               strings.ToLower(conf.LogLevel) == "debug",
		rateLimiterResponse: rateLimiterResponse,

		exit: make(chan struct{}),
	}
}

func (r *HTTPReceiver) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	hash, infoHandler := r.makeInfoHandler()
	r.attachDebugHandlers(mux)
	for _, e := range endpoints {
		if e.IsEnabled != nil && !e.IsEnabled(r.conf) {
			continue
		}
		mux.Handle(e.Pattern, replyWithVersion(hash, e.Handler(r)))
	}
	mux.HandleFunc("/info", infoHandler)

	return mux
}

// replyWithVersion returns an http.Handler which calls h with an addition of some
// HTTP headers containing version and state information.
func replyWithVersion(hash string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Datadog-Agent-Version", info.Version)
		w.Header().Set("Datadog-Agent-State", hash)
		h.ServeHTTP(w, r)
	})
}

// Start starts doing the HTTP server and is ready to receive traces
func (r *HTTPReceiver) Start() {
	if r.conf.ReceiverPort == 0 {
		log.Debug("HTTP receiver disabled by config (apm_config.receiver_port: 0).")
		return
	}
	mux := r.buildMux()

	timeout := 5 * time.Second
	if r.conf.ReceiverTimeout > 0 {
		timeout = time.Duration(r.conf.ReceiverTimeout) * time.Second
	}
	httpLogger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	r.server = &http.Server{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		ErrorLog:     stdlog.New(httpLogger, "http.Server: ", 0),
		Handler:      mux,
	}

	addr := fmt.Sprintf("%s:%d", r.conf.ReceiverHost, r.conf.ReceiverPort)
	ln, err := r.listenTCP(addr)
	if err != nil {
		killProcess("Error creating tcp listener: %v", err)
	}
	go func() {
		defer watchdog.LogOnPanic()
		if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Errorf("Could not start HTTP server: %v. HTTP receiver disabled.", err)
		}
	}()
	log.Infof("Listening for traces at http://%s", addr)

	if path := r.conf.ReceiverSocket; path != "" {
		ln, err := r.listenUnix(path)
		if err != nil {
			killProcess("Error creating UDS listener: %v", err)
		}
		go func() {
			defer watchdog.LogOnPanic()
			if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Could not start UDS server: %v. UDS receiver disabled.", err)
			}
		}()
		log.Infof("Listening for traces at unix://%s", path)
	}

	if path := r.conf.WindowsPipeName; path != "" {
		pipepath := `\\.\pipe\` + path
		bufferSize := r.conf.PipeBufferSize
		secdec := r.conf.PipeSecurityDescriptor
		ln, err := listenPipe(pipepath, secdec, bufferSize)
		if err != nil {
			killProcess("Error creating %q named pipe: %v", pipepath, err)
		}
		go func() {
			defer watchdog.LogOnPanic()
			if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Could not start Windows Pipes server: %v. Windows Pipes receiver disabled.", err)
			}
		}()
		log.Infof("Listening for traces on Windowes pipe %q. Security descriptor is %q", pipepath, secdec)
	}

	go r.RateLimiter.Run()

	go func() {
		defer watchdog.LogOnPanic()
		r.loop()
	}()
}

func (r *HTTPReceiver) attachDebugHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/debug/blockrate", func(w http.ResponseWriter, r *http.Request) {
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
		fmt.Fprintf(w, "Block profile rate set to %d. It will automatically be disabled again after calling /debug/pprof/block\n", rate)
	})

	mux.HandleFunc("/debug/pprof/block", func(w http.ResponseWriter, r *http.Request) {
		// serve the block profile and reset the rate to 0.
		pprof.Handler("block").ServeHTTP(w, r)
		runtime.SetBlockProfileRate(0)
	})

	mux.Handle("/debug/vars", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// allow the GUI to call this endpoint so that the status can be reported
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:"+r.conf.GUIPort)
		expvar.Handler().ServeHTTP(w, req)
	}))
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
	return NewMeasuredListener(ln, "uds_connections"), err
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
	return NewMeasuredListener(tcpln, "tcp_connections"), err
}

// Stop stops the receiver and shuts down the HTTP server.
func (r *HTTPReceiver) Stop() error {
	if r.conf.ReceiverPort == 0 {
		return nil
	}
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
		req.Body = apiutil.NewLimitedReader(req.Body, r.conf.MaxRequestBytes)

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

	// headerComputedTopLevel specifies that the client has marked top-level spans, when set.
	// Any non-empty value will mean 'yes'.
	headerComputedTopLevel = "Datadog-Client-Computed-Top-Level"

	// headerComputedStats specifies whether the client has computed stats so that the agent
	// doesn't have to.
	headerComputedStats = "Datadog-Client-Computed-Stats"

	// headderDroppedP0Traces contains the number of P0 trace chunks dropped by the client.
	// This value is used to adjust priority rates computed by the agent.
	headerDroppedP0Traces = "Datadog-Client-Dropped-P0-Traces"

	// headderDroppedP0Spans contains the number of P0 spans dropped by the client.
	// This value is used for metrics and could be used in the future to adjust priority rates.
	headerDroppedP0Spans = "Datadog-Client-Dropped-P0-Spans"

	// headerRatesPayloadVersion contains the version of sampling rates.
	// If both agent and client have the same version, the agent won't return rates in API response.
	headerRatesPayloadVersion = "Datadog-Rates-Payload-Version"

	// tagContainersTags specifies the name of the tag which holds key/value
	// pairs representing information about the container (Docker, EC2, etc).
	tagContainersTags = "_dd.tags.container"
)

// TagStats returns the stats and tags coinciding with the information found in header.
// For more information, check the "Datadog-Meta-*" HTTP headers defined in this file.
func (r *HTTPReceiver) TagStats(v Version, header http.Header) *info.TagStats {
	return r.tagStats(v, header)
}

func (r *HTTPReceiver) tagStats(v Version, header http.Header) *info.TagStats {
	return r.Stats.GetTagStats(info.Tags{
		Lang:            header.Get(headerLang),
		LangVersion:     header.Get(headerLangVersion),
		Interpreter:     header.Get(headerLangInterpreter),
		LangVendor:      header.Get(headerLangInterpreterVendor),
		TracerVersion:   header.Get(headerTracerVersion),
		EndpointVersion: string(v),
	})
}

// decodeTracerPayload decodes the payload in http request `req`.
// - tp is the decoded payload
// - ranHook reports whether the decoder was able to run the pb.MetaHook
// - err is the first error encountered
func decodeTracerPayload(v Version, req *http.Request, ts *info.TagStats) (tp *pb.TracerPayload, ranHook bool, err error) {
	switch v {
	case v01:
		var spans []pb.Span
		if err = json.NewDecoder(req.Body).Decode(&spans); err != nil {
			return nil, false, err
		}
		return &pb.TracerPayload{
			LanguageName:    ts.Lang,
			LanguageVersion: ts.LangVersion,
			ContainerID:     req.Header.Get(headerContainerID),
			Chunks:          traceChunksFromSpans(spans),
			TracerVersion:   ts.TracerVersion,
		}, false, nil
	case v05:
		buf := getBuffer()
		defer putBuffer(buf)
		if _, err = io.Copy(buf, req.Body); err != nil {
			return nil, false, err
		}
		var traces pb.Traces
		err = traces.UnmarshalMsgDictionary(buf.Bytes())
		return &pb.TracerPayload{
			LanguageName:    ts.Lang,
			LanguageVersion: ts.LangVersion,
			ContainerID:     req.Header.Get(headerContainerID),
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   ts.TracerVersion,
		}, true, err
	case V07:
		buf := getBuffer()
		defer putBuffer(buf)
		if _, err = io.Copy(buf, req.Body); err != nil {
			return nil, false, err
		}
		var tracerPayload pb.TracerPayload
		_, err = tracerPayload.UnmarshalMsg(buf.Bytes())
		return &tracerPayload, true, err
	default:
		var traces pb.Traces
		if ranHook, err = decodeRequest(req, &traces); err != nil {
			return nil, false, err
		}
		return &pb.TracerPayload{
			LanguageName:    ts.Lang,
			LanguageVersion: ts.LangVersion,
			ContainerID:     req.Header.Get(headerContainerID),
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   ts.TracerVersion,
		}, ranHook, nil
	}
}

// replyOK replies to the given http.ReponseWriter w based on the endpoint version, with either status 200/OK
// or with a list of rates by service. It returns the number of bytes written along with reporting if the operation
// was successful.
func (r *HTTPReceiver) replyOK(req *http.Request, v Version, w http.ResponseWriter) (n uint64, ok bool) {
	switch v {
	case v01, v02, v03:
		return httpOK(w)
	default:
		ratesVersion := req.Header.Get(headerRatesPayloadVersion)
		return httpRateByService(ratesVersion, w, r.dynConf)
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

// StatsProcessor implementations are able to process incoming client stats.
type StatsProcessor interface {
	// ProcessStats takes a stats payload and consumes it. It is considered to be originating
	// from the given lang.
	ProcessStats(p pb.ClientStatsPayload, lang, tracerVersion string)
}

// handleStats handles incoming stats payloads.
func (r *HTTPReceiver) handleStats(w http.ResponseWriter, req *http.Request) {
	defer timing.Since("datadog.trace_agent.receiver.stats_process_ms", time.Now())

	ts := r.tagStats(V07, req.Header)
	rd := apiutil.NewLimitedReader(req.Body, r.conf.MaxRequestBytes)
	req.Header.Set("Accept", "application/msgpack")
	var in pb.ClientStatsPayload
	if err := msgp.Decode(rd, &in); err != nil {
		httpDecodingError(err, []string{"handler:stats", "codec:msgpack", "v:v0.6"}, w)
		return
	}

	metrics.Count("datadog.trace_agent.receiver.stats_payload", 1, ts.AsTags(), 1)
	metrics.Count("datadog.trace_agent.receiver.stats_bytes", rd.Count, ts.AsTags(), 1)
	metrics.Count("datadog.trace_agent.receiver.stats_buckets", int64(len(in.Stats)), ts.AsTags(), 1)

	r.statsProcessor.ProcessStats(in, req.Header.Get(headerLang), req.Header.Get(headerTracerVersion))
}

// handleTraces knows how to handle a bunch of traces
func (r *HTTPReceiver) handleTraces(v Version, w http.ResponseWriter, req *http.Request) {
	ts := r.tagStats(v, req.Header)
	tracen, err := traceCount(req)
	if err == nil && r.rateLimited(tracen) {
		// this payload can not be accepted
		io.Copy(ioutil.Discard, req.Body) //nolint:errcheck
		w.WriteHeader(r.rateLimiterResponse)
		r.replyOK(req, v, w)
		atomic.AddInt64(&ts.PayloadRefused, 1)
		return
	}

	start := time.Now()
	defer func(err error) {
		tags := append(ts.AsTags(), fmt.Sprintf("success:%v", err == nil))
		metrics.Histogram("datadog.trace_agent.receiver.serve_traces_ms", float64(time.Since(start))/float64(time.Millisecond), tags, 1)
	}(err)
	tp, ranHook, err := decodeTracerPayload(v, req, ts)
	if err != nil {
		httpDecodingError(err, []string{"handler:traces", fmt.Sprintf("v:%s", v)}, w)
		switch err {
		case apiutil.ErrLimitedReaderLimitReached:
			atomic.AddInt64(&ts.TracesDropped.PayloadTooLarge, tracen)
		case io.EOF, io.ErrUnexpectedEOF, msgp.ErrShortBytes:
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
	if !ranHook {
		// The decoder of this request did not run the pb.MetaHook. The user is either using
		// a deprecated endpoint or Content-Type, or, a new decoder was implemented and the
		// the hook was not added.
		log.Debug("Decoded the request without running pb.MetaHook. If this is a newly implemented endpoint, please make sure to run it!")
		if _, ok := pb.MetaHook(); ok {
			log.Warn("Received request on deprecated API endpoint or Content-Type. Performance is degraded. If you think this is an error, please contact support with this message.")
			runMetaHook(tp.Chunks)
		}
	}
	if n, ok := r.replyOK(req, v, w); ok {
		tags := append(ts.AsTags(), "endpoint:traces_"+string(v))
		metrics.Histogram("datadog.trace_agent.receiver.rate_response_bytes", float64(n), tags, 1)
	}

	atomic.AddInt64(&ts.TracesReceived, int64(len(tp.Chunks)))
	atomic.AddInt64(&ts.TracesBytes, req.Body.(*apiutil.LimitedReader).Count)
	atomic.AddInt64(&ts.PayloadAccepted, 1)

	if ctags := getContainerTags(r.conf.ContainerTags, tp.ContainerID); ctags != "" {
		if tp.Tags == nil {
			tp.Tags = make(map[string]string)
		}
		tp.Tags[tagContainersTags] = ctags
	}

	payload := &Payload{
		Source:                 ts,
		TracerPayload:          tp,
		ClientComputedTopLevel: req.Header.Get(headerComputedTopLevel) != "",
		ClientComputedStats:    req.Header.Get(headerComputedStats) != "",
		ClientDroppedP0s:       droppedTracesFromHeader(req.Header, ts),
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

// runMetaHook runs the pb.MetaHook on all spans from traces.
func runMetaHook(chunks []*pb.TraceChunk) {
	hook, ok := pb.MetaHook()
	if !ok {
		return
	}
	for _, chunk := range chunks {
		for _, span := range chunk.Spans {
			for k, v := range span.Meta {
				if newv := hook(k, v); newv != v {
					span.Meta[k] = newv
				}
			}
		}
	}
}

func droppedTracesFromHeader(h http.Header, ts *info.TagStats) int64 {
	var dropped int64
	if v := h.Get(headerDroppedP0Traces); v != "" {
		count, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			dropped = count
			atomic.AddInt64(&ts.ClientDroppedP0Traces, count)
		}
	}
	if v := h.Get(headerDroppedP0Spans); v != "" {
		count, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			atomic.AddInt64(&ts.ClientDroppedP0Spans, count)
		}
	}
	return dropped
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
				rates := r.dynConf.RateByService.GetNewState("").Rates
				info.UpdateRateByService(rates)
			}
		}
	}
}

// killProcess exits the process with the given msg; replaced in tests.
var killProcess = func(format string, a ...interface{}) {
	log.Criticalf(format, a...)
	os.Exit(1)
}

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

// decodeRequest decodes the payload in http request `req` into `dest`.
// It handles only v02, v03, v04 requests.
// - ranHook reports whether the decoder was able to run the pb.MetaHook
// - err is the first error encountered
func decodeRequest(req *http.Request, dest *pb.Traces) (ranHook bool, err error) {
	switch mediaType := getMediaType(req); mediaType {
	case "application/msgpack":
		buf := getBuffer()
		defer putBuffer(buf)
		_, err = io.Copy(buf, req.Body)
		if err != nil {
			return false, err
		}
		_, err = dest.UnmarshalMsg(buf.Bytes())
		return true, err
	case "application/json":
		fallthrough
	case "text/json":
		fallthrough
	case "":
		err = json.NewDecoder(req.Body).Decode(&dest)
		return false, err
	default:
		// do our best
		if err1 := json.NewDecoder(req.Body).Decode(&dest); err1 != nil {
			buf := getBuffer()
			defer putBuffer(buf)
			_, err2 := io.Copy(buf, req.Body)
			if err2 != nil {
				return false, err2
			}
			_, err2 = dest.UnmarshalMsg(buf.Bytes())
			return true, err2
		}
		return false, nil
	}
}

func traceChunksFromSpans(spans []pb.Span) []*pb.TraceChunk {
	traceChunks := []*pb.TraceChunk{}
	byID := make(map[uint64][]*pb.Span)
	for _, s := range spans {
		byID[s.TraceID] = append(byID[s.TraceID], &s)
	}
	for _, t := range byID {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    t,
		})
	}
	return traceChunks
}

func traceChunksFromTraces(traces pb.Traces) []*pb.TraceChunk {
	traceChunks := make([]*pb.TraceChunk, 0, len(traces))
	for _, trace := range traces {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    trace,
		})
	}

	return traceChunks
}

// getContainerTag returns container and orchestrator tags belonging to containerID. If containerID
// is empty or no tags are found, an empty string is returned.
func getContainerTags(fn func(string) ([]string, error), containerID string) string {
	if containerID == "" {
		return ""
	}
	if fn == nil {
		log.Warn("ContainerTags not configured")
		return ""
	}
	list, err := fn("container_id://" + containerID)
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
