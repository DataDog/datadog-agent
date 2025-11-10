// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api implements the HTTP server that receives payloads from clients.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/tinylib/msgp/msgp"
	"go.uber.org/atomic"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/api/loader"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

// defaultReceiverBufferSize is used as a default for the initial size of http body buffer
// if no content-length is provided (Content-Encoding: Chunked) which happens in some tracers.
//
// This value has been picked as a "safe" default. Most real life traces should be at least a few KiB
// so allocating 8KiB should provide a big enough buffer to prevent initial resizing, without blowing
// up memory usage of the tracer.
const defaultReceiverBufferSize = 8192 // 8KiB

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

func copyRequestBody(buf *bytes.Buffer, req *http.Request) (written int64, err error) {
	reserveBodySize(buf, req)
	return io.Copy(buf, req.Body)
}

func reserveBodySize(buf *bytes.Buffer, req *http.Request) {
	var err error
	bufferSize := 0
	if contentSize := req.Header.Get("Content-Length"); contentSize != "" {
		bufferSize, err = strconv.Atoi(contentSize)
		if err != nil {
			log.Debugf("could not parse Content-Length header value as integer: %v", err)
		}
	}
	if bufferSize == 0 {
		bufferSize = defaultReceiverBufferSize
	}
	buf.Grow(bufferSize)
}

// HTTPReceiver is a collector that uses HTTP protocol and just holds
// a chan where the spans received are sent one by one
type HTTPReceiver struct {
	Stats *info.ReceiverStats

	out                 chan *Payload
	outV1               chan *PayloadV1
	conf                *config.AgentConfig
	dynConf             *sampler.DynamicConfig
	server              *http.Server
	statsProcessor      StatsProcessor
	containerIDProvider IDProvider

	telemetryCollector telemetry.TelemetryCollector
	telemetryForwarder *TelemetryForwarder

	rateLimiterResponse int // HTTP status code when refusing

	wg   sync.WaitGroup // waits for all requests to be processed
	exit chan struct{}

	// recvsem is a semaphore that controls the number goroutines that can
	// be simultaneously deserializing incoming payloads.
	// It is important to control this in order to prevent decoding incoming
	// payloads faster than we can process them, and buffering them, resulting
	// in memory limit issues.
	recvsem chan struct{}

	// outOfCPUCounter is counter to throttle the out of cpu warning log
	outOfCPUCounter *atomic.Uint32

	statsd   statsd.ClientInterface
	timing   timing.Reporter
	info     *watchdog.CurrentInfo
	Handlers map[string]http.Handler
}

// NewHTTPReceiver returns a pointer to a new HTTPReceiver
func NewHTTPReceiver(
	conf *config.AgentConfig,
	dynConf *sampler.DynamicConfig,
	out chan *Payload,
	outV1 chan *PayloadV1,
	statsProcessor StatsProcessor,
	telemetryCollector telemetry.TelemetryCollector,
	statsd statsd.ClientInterface,
	timing timing.Reporter) *HTTPReceiver {
	rateLimiterResponse := http.StatusOK
	if conf.HasFeature("429") {
		rateLimiterResponse = http.StatusTooManyRequests
	}
	semcount := conf.Decoders
	if semcount == 0 {
		semcount = runtime.GOMAXPROCS(0) / 2
		if semcount == 0 {
			semcount = 1
		}
	}
	log.Infof("Receiver configured with %d decoders and a timeout of %dms", semcount, conf.DecoderTimeout)
	containerIDProvider := NewIDProvider(conf.ContainerProcRoot, conf.ContainerIDFromOriginInfo)
	telemetryForwarder := NewTelemetryForwarder(conf, containerIDProvider, statsd)
	return &HTTPReceiver{
		Stats: info.NewReceiverStats(conf.SendAllInternalStats),

		out:                 out,
		outV1:               outV1,
		statsProcessor:      statsProcessor,
		conf:                conf,
		dynConf:             dynConf,
		containerIDProvider: containerIDProvider,

		telemetryCollector: telemetryCollector,
		telemetryForwarder: telemetryForwarder,

		rateLimiterResponse: rateLimiterResponse,

		exit: make(chan struct{}),

		// Based on experimentation, 4 simultaneous readers
		// is enough to keep 16 threads busy processing the
		// payloads, without overwhelming the available memory.
		// This also works well with a smaller GOMAXPROCS, since
		// the processor backpressure ensures we have at most
		// 4 payloads waiting to be queued and processed.
		recvsem: make(chan struct{}, semcount),

		outOfCPUCounter: atomic.NewUint32(0),

		statsd:   statsd,
		timing:   timing,
		info:     watchdog.NewCurrentInfo(),
		Handlers: make(map[string]http.Handler),
	}
}

// timeoutMiddleware sets a timeout for a handler. This lets us have different
// timeout values for each handler
func timeoutMiddleware(timeout time.Duration, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func (r *HTTPReceiver) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	defaultTimeout := getConfiguredRequestTimeoutDuration(r.conf)

	hash, infoHandler := r.makeInfoHandler()
	for _, e := range endpoints {
		if e.IsEnabled != nil && !e.IsEnabled(r.conf) {
			continue
		}
		timeout := defaultTimeout
		if e.TimeoutOverride != nil {
			timeout = e.TimeoutOverride(r.conf)
		}
		h := replyWithVersion(hash, r.conf.AgentVersion, timeoutMiddleware(timeout, e.Handler(r)))
		r.Handlers[e.Pattern] = h
		mux.Handle(e.Pattern, h)
	}
	r.Handlers["/info"] = infoHandler
	mux.HandleFunc("/info", infoHandler)

	return mux
}

// replyWithVersion returns an http.Handler which calls h with an addition of some
// HTTP headers containing version and state information.
func replyWithVersion(hash string, version string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Datadog-Agent-Version", version)
		w.Header().Set("Datadog-Agent-State", hash)
		h.ServeHTTP(w, r)
	})
}

func getConfiguredRequestTimeoutDuration(conf *config.AgentConfig) time.Duration {
	timeout := 5 * time.Second
	if conf.ReceiverTimeout > 0 {
		timeout = time.Duration(conf.ReceiverTimeout) * time.Second
	}
	return timeout
}

func getConfiguredEVPRequestTimeoutDuration(conf *config.AgentConfig) time.Duration {
	timeout := 30 * time.Second
	if conf.EVPProxy.ReceiverTimeout > 0 {
		timeout = time.Duration(conf.EVPProxy.ReceiverTimeout) * time.Second
	}
	return timeout
}

func getConfiguredProfilingRequestTimeoutDuration(conf *config.AgentConfig) time.Duration {
	timeout := 5 * time.Second
	if conf.ProfilingProxy.ReceiverTimeout > 0 {
		timeout = time.Duration(conf.ProfilingProxy.ReceiverTimeout) * time.Second
	}
	return timeout
}

// Start starts doing the HTTP server and is ready to receive traces
func (r *HTTPReceiver) Start() {
	r.telemetryForwarder.start()

	if !r.conf.ReceiverEnabled {
		log.Debug("HTTP Server is off: HTTPReceiver is disabled.")
		return
	}
	if r.conf.ReceiverPort == 0 &&
		r.conf.ReceiverSocket == "" &&
		r.conf.WindowsPipeName == "" {
		// none of the HTTP listeners are enabled; exit early
		log.Debug("HTTP Server is off: all listeners are disabled.")
		return
	}

	httpLogger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	r.server = &http.Server{
		// Note: We don't set WriteTimeout since we want to have different timeouts per-handler
		ReadTimeout: getConfiguredRequestTimeoutDuration(r.conf),
		ErrorLog:    stdlog.New(httpLogger, "http.Server: ", 0),
		Handler:     r.buildMux(),
		ConnContext: connContext,
	}

	if r.conf.ReceiverPort > 0 {
		addr := net.JoinHostPort(r.conf.ReceiverHost, strconv.Itoa(r.conf.ReceiverPort))

		var ln net.Listener
		var err error
		// When using the trace-loader, the TCP listener might be provided as an already opened file descriptor
		// so we try to get a listener from it, and fallback to listening on the given address if it fails
		if tcpFDStr, ok := os.LookupEnv("DD_APM_NET_RECEIVER_FD"); ok {
			ln, err = loader.GetListenerFromFD(tcpFDStr, "tcp_conn")
			if err == nil {
				log.Debugf("Using TCP listener from file descriptor %s", tcpFDStr)
			} else {
				log.Errorf("Error creating TCP listener from file descriptor %s: %v", tcpFDStr, err)
			}
		}
		if ln == nil {
			// if the fd was not provided, or we failed to get a listener from it, listen on the given address
			ln, err = loader.GetTCPListener(addr)
		}
		if err == nil {
			ln, err = r.listenTCPListener(ln)
		}

		if err != nil {
			r.telemetryCollector.SendStartupError(telemetry.CantStartHttpServer, err)
			killProcess("Error creating tcp listener: %v", err)
		}
		go func() {
			defer watchdog.LogOnPanic(r.statsd)
			if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Could not start HTTP server: %v. HTTP receiver disabled.", err)
				r.telemetryCollector.SendStartupError(telemetry.CantStartHttpServer, err)
			}
		}()
		log.Infof("Listening for traces at http://%s", addr)
	} else {
		log.Debug("HTTP receiver disabled by config (apm_config.receiver_port: 0).")
	}

	if path := r.conf.ReceiverSocket; path != "" {
		log.Infof("Using UDS listener at %s", path)
		// When using the trace-loader, the UDS listener might be provided as an already opened file descriptor
		// so we try to get a listener from it, and fallback to listening on the given path if it fails
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			var ln net.Listener
			var err error
			if unixFDStr, ok := os.LookupEnv("DD_APM_UNIX_RECEIVER_FD"); ok {
				ln, err = loader.GetListenerFromFD(unixFDStr, "unix_conn")
				if err == nil {
					log.Debugf("Using UDS listener from file descriptor %s", unixFDStr)
				} else {
					log.Errorf("Error creating UDS listener from file descriptor %s: %v", unixFDStr, err)
				}
			}
			if ln == nil {
				// if the fd was not provided, or we failed to get a listener from it, listen on the given path
				ln, err = loader.GetUnixListener(path)
			}

			if err != nil {
				log.Errorf("Error creating UDS listener: %v", err)
				r.telemetryCollector.SendStartupError(telemetry.CantStartUdsServer, err)
			} else {
				ln = NewMeasuredListener(ln, "uds_connections", r.conf.MaxConnections, r.statsd)

				go func() {
					defer watchdog.LogOnPanic(r.statsd)
					if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
						log.Errorf("Could not start UDS server: %v. UDS receiver disabled.", err)
						r.telemetryCollector.SendStartupError(telemetry.CantStartUdsServer, err)
					}
				}()
				log.Infof("Listening for traces at unix://%s", path)
			}
		} else {
			log.Errorf("Could not start UDS listener: socket directory does not exist: %s", path)
		}
	}

	if path := r.conf.WindowsPipeName; path != "" {
		pipepath := `\\.\pipe\` + path
		bufferSize := r.conf.PipeBufferSize
		secdec := r.conf.PipeSecurityDescriptor
		ln, err := listenPipe(pipepath, secdec, bufferSize, r.conf.MaxConnections, r.statsd)
		if err != nil {
			r.telemetryCollector.SendStartupError(telemetry.CantStartWindowsPipeServer, err)
			killProcess("Error creating %q named pipe: %v", pipepath, err)
		}
		go func() {
			defer watchdog.LogOnPanic(r.statsd)
			if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Could not start Windows Pipes server: %v. Windows Pipes receiver disabled.", err)
				r.telemetryCollector.SendStartupError(telemetry.CantStartWindowsPipeServer, err)
			}
		}()
		log.Infof("Listening for traces on Windows pipe %q. Security descriptor is %q", pipepath, secdec)
	}

	go func() {
		defer watchdog.LogOnPanic(r.statsd)
		r.loop()
	}()
}

// listenTCP creates a new net.Listener on the provided TCP address.
func (r *HTTPReceiver) listenTCPListener(tcpln net.Listener) (net.Listener, error) {
	if climit := r.conf.ConnectionLimit; climit > 0 {
		ln, err := newRateLimitedListener(tcpln, climit, r.statsd)
		go func() {
			defer watchdog.LogOnPanic(r.statsd)
			ln.Refresh(climit)
		}()
		return ln, err
	}
	return NewMeasuredListener(tcpln, "tcp_connections", r.conf.MaxConnections, r.statsd), nil
}

// Stop stops the receiver and shuts down the HTTP server.
func (r *HTTPReceiver) Stop() error {
	if !r.conf.ReceiverEnabled || (r.conf.ReceiverPort == 0 && r.conf.ReceiverSocket == "" && r.conf.WindowsPipeName == "") {
		return nil
	}
	r.exit <- struct{}{}
	<-r.exit

	expiry := time.Now().Add(5 * time.Second) // give it 5 seconds
	ctx, cancel := context.WithDeadline(context.Background(), expiry)
	defer cancel()
	if err := r.server.Shutdown(ctx); err != nil {
		log.Warnf("Error shutting down HTTPReceiver: %v", err)
		return err
	}
	r.wg.Wait()
	r.telemetryForwarder.Stop()
	return nil
}

// BuildHandlers builds the handlers so they are available in the trace component
func (r *HTTPReceiver) BuildHandlers() {
	r.buildMux()
}

// UpdateAPIKey rebuilds the server handler to update API Keys in all endpoints
func (r *HTTPReceiver) UpdateAPIKey() {
	if r.server == nil {
		return
	}
	log.Debug("API Key updated. Rebuilding API handler.")
	handler := r.buildMux()
	r.server.Handler = handler
}

func (r *HTTPReceiver) handleWithVersion(v Version, f func(Version, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if mediaType := getMediaType(req); mediaType == "application/msgpack" && (v == v01 || v == v02) {
			// msgpack is only supported for versions >= v0.3
			httpFormatError(w, v, fmt.Errorf("unsupported media type: %q", mediaType), r.statsd)
			return
		}

		if req.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}

		// TODO(x): replace with http.MaxBytesReader?
		req.Body = apiutil.NewLimitedReader(req.Body, r.conf.MaxRequestBytes)

		f(v, w, req)
	}
}

var errInvalidHeaderTraceCountValue = fmt.Errorf("%q header value is not a number", header.TraceCount)

func traceCount(req *http.Request) (int64, error) {
	str := req.Header.Get(header.TraceCount)
	if str == "" {
		return 0, fmt.Errorf("HTTP header %q not found", header.TraceCount)
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0, errInvalidHeaderTraceCountValue
	}
	return int64(n), nil
}

const (
	// tagContainersTags specifies the name of the tag which holds key/value
	// pairs representing information about the container (Docker, EC2, etc).
	tagContainersTags = "_dd.tags.container"
	tagProcessTags    = "_dd.tags.process"
)

// TagStats returns the stats and tags coinciding with the information found in header.
// For more information, check the "Datadog-Meta-*" HTTP headers defined in this file.
func (r *HTTPReceiver) TagStats(v Version, header http.Header, service string) *info.TagStats {
	return r.tagStats(v, header, service)
}

func (r *HTTPReceiver) tagStats(v Version, httpHeader http.Header, service string) *info.TagStats {
	return r.Stats.GetTagStats(info.Tags{
		Lang:            httpHeader.Get(header.Lang),
		LangVersion:     httpHeader.Get(header.LangVersion),
		Interpreter:     httpHeader.Get(header.LangInterpreter),
		LangVendor:      httpHeader.Get(header.LangInterpreterVendor),
		TracerVersion:   httpHeader.Get(header.TracerVersion),
		EndpointVersion: string(v),
		Service:         service,
	})
}

// decodeTracerPayload decodes the payload in http request `req`.
// - tp is the decoded payload
// - ranHook reports whether the decoder was able to run the pb.MetaHook
// - err is the first error encountered
func decodeTracerPayload(v Version, req *http.Request, cIDProvider IDProvider, lang, langVersion, tracerVersion string) (tp *pb.TracerPayload, err error) {
	switch v {
	case v01:
		var spans []*pb.Span
		if err = json.NewDecoder(req.Body).Decode(&spans); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     cIDProvider.GetContainerID(req.Context(), req.Header),
			Chunks:          traceChunksFromSpans(spans),
			TracerVersion:   tracerVersion,
		}, nil
	case v05:
		buf := getBuffer()
		defer putBuffer(buf)
		if _, err = copyRequestBody(buf, req); err != nil {
			return nil, err
		}
		var traces pb.Traces
		if err = traces.UnmarshalMsgDictionary(buf.Bytes()); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     cIDProvider.GetContainerID(req.Context(), req.Header),
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   tracerVersion,
		}, err
	case V07:
		buf := getBuffer()
		defer putBuffer(buf)
		if _, err = copyRequestBody(buf, req); err != nil {
			return nil, err
		}
		var tracerPayload pb.TracerPayload
		_, err = tracerPayload.UnmarshalMsg(buf.Bytes())
		return &tracerPayload, err
	default:
		var traces pb.Traces
		if err = decodeRequest(req, &traces); err != nil {
			return nil, err
		}
		return &pb.TracerPayload{
			LanguageName:    lang,
			LanguageVersion: langVersion,
			ContainerID:     cIDProvider.GetContainerID(req.Context(), req.Header),
			Chunks:          traceChunksFromTraces(traces),
			TracerVersion:   tracerVersion,
		}, nil
	}
}

func decodeTracerPayloadV1(req *http.Request, cIDProvider IDProvider, conf *config.AgentConfig) (tp *idx.InternalTracerPayload, err error) {
	buf := getBuffer()
	defer putBuffer(buf)
	if _, err = copyRequestBody(buf, req); err != nil {
		return nil, err
	}
	var tracerPayload idx.InternalTracerPayload
	_, err = tracerPayload.UnmarshalMsg(buf.Bytes())
	if err != nil {
		if conf.DebugV1Payloads {
			encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
			log.Errorf("decodeTracerPayloadV1: failed to unmarshal payload, base64 received: %s", encoded)
		}
		return nil, err
	}
	if tracerPayload.ContainerID() == "" {
		cid := cIDProvider.GetContainerID(req.Context(), req.Header)
		tracerPayload.SetContainerID(cid)
	}
	return &tracerPayload, err
}

// replyOK replies to the given http.ResponseWriter w based on the endpoint version, with either status 200/OK
// or with a list of rates by service. It returns the number of bytes written along with reporting if the operation
// was successful.
func (r *HTTPReceiver) replyOK(req *http.Request, v Version, w http.ResponseWriter) (n uint64, ok bool) {
	switch v {
	case v01, v02, v03:
		return httpOK(w)
	default:
		ratesVersion := req.Header.Get(header.RatesPayloadVersion)
		return httpRateByService(ratesVersion, w, r.dynConf, r.statsd)
	}
}

// StatsProcessor implementations are able to process incoming client stats.
type StatsProcessor interface {
	// ProcessStats takes a stats payload and consumes it. It is considered to be originating from the given lang.
	// Context should be used to control processing timeouts, allowing the receiver to return the error response.
	ProcessStats(ctx context.Context, p *pb.ClientStatsPayload, lang, tracerVersion, containerID, obfuscationVersion string) error
}

// handleStats handles incoming stats payloads.
func (r *HTTPReceiver) handleStats(w http.ResponseWriter, req *http.Request) {
	defer r.timing.Since("datadog.trace_agent.receiver.stats_process_ms", time.Now())

	rd := apiutil.NewLimitedReader(req.Body, r.conf.MaxRequestBytes)
	req.Header.Set("Accept", "application/msgpack")
	in := &pb.ClientStatsPayload{}
	if err := msgp.Decode(rd, in); err != nil {
		log.Errorf("Error decoding pb.ClientStatsPayload: %v", err)
		tags := append(r.tagStats(V06, req.Header, "").AsTags(), "reason:decode")
		_ = r.statsd.Count("datadog.trace_agent.receiver.stats_payload_rejected", 1, tags, 1)
		httpDecodingError(err, []string{"handler:stats", "codec:msgpack", "v:v0.6"}, w, r.statsd)
		return
	}

	firstService := func(cs *pb.ClientStatsPayload) string {
		if cs == nil || len(cs.Stats) == 0 || len(cs.Stats[0].Stats) == 0 {
			return ""
		}
		return cs.Stats[0].Stats[0].Service
	}

	ts := r.tagStats(V06, req.Header, firstService(in))
	_ = r.statsd.Count("datadog.trace_agent.receiver.stats_payload", 1, ts.AsTags(), 1)
	_ = r.statsd.Count("datadog.trace_agent.receiver.stats_bytes", rd.Count, ts.AsTags(), 1)
	_ = r.statsd.Count("datadog.trace_agent.receiver.stats_buckets", int64(len(in.Stats)), ts.AsTags(), 1)

	// Resolve ContainerID based on HTTP headers
	lang := req.Header.Get(header.Lang)
	tracerVersion := req.Header.Get(header.TracerVersion)
	obfuscationVersion := req.Header.Get(header.TracerObfuscationVersion)
	containerID := r.containerIDProvider.GetContainerID(req.Context(), req.Header)

	timeout := getConfiguredRequestTimeoutDuration(r.conf)
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	if err := r.statsProcessor.ProcessStats(ctx, in, lang, tracerVersion, containerID, obfuscationVersion); err != nil {
		log.Errorf("Error processing pb.ClientStatsPayload: %v", err)
		tags := append(ts.AsTags(), "reason:timeout")
		_ = r.statsd.Count("datadog.trace_agent.receiver.stats_payload_rejected", 1, tags, 1)
		httpDecodingError(err, []string{"handler:stats", "codec:msgpack", "v:v0.6"}, w, r.statsd)
	}
}

// handleTraces knows how to handle a bunch of traces
func (r *HTTPReceiver) handleTracesV1(w http.ResponseWriter, req *http.Request) {
	tracen, err := traceCount(req)
	if err == errInvalidHeaderTraceCountValue {
		log.Errorf("Failed to count traces: %s", err)
	}
	defer req.Body.Close()
	select {
	// Wait for the semaphore to become available, allowing the handler to
	// decode its payload.
	// After the configured timeout, respond without ingesting the payload,
	// and sending the configured status.
	case r.recvsem <- struct{}{}:
	case <-time.After(time.Duration(r.conf.DecoderTimeout) * time.Millisecond):
		log.Debugf("trace-agent is overwhelmed, a payload has been rejected")
		// this payload can not be accepted
		io.Copy(io.Discard, req.Body) //nolint:errcheck
		w.WriteHeader(http.StatusTooManyRequests)
		r.replyOK(req, V10, w)
		r.tagStats(V10, req.Header, "").PayloadRefused.Inc()
		return
	}
	defer func() {
		// Signal the semaphore that we are done decoding, so another handler
		// routine can take a turn decoding a payload.
		<-r.recvsem
	}()

	firstService := func(tp *idx.InternalTracerPayload) string {
		if tp == nil || len(tp.Chunks) == 0 || len(tp.Chunks[0].Spans) == 0 {
			return ""
		}
		return tp.Chunks[0].Spans[0].Service()
	}

	start := time.Now()
	tp, err := decodeTracerPayloadV1(req, r.containerIDProvider, r.conf)
	ts := r.tagStats(V10, req.Header, firstService(tp))
	defer func(err error) {
		tags := append(ts.AsTags(), fmt.Sprintf("success:%v", err == nil))
		_ = r.statsd.Histogram("datadog.trace_agent.receiver.serve_traces_ms", float64(time.Since(start))/float64(time.Millisecond), tags, 1)
	}(err)
	if err != nil {
		httpDecodingError(err, []string{"handler:traces", fmt.Sprintf("v:%s", V10)}, w, r.statsd)
		switch err {
		case apiutil.ErrLimitedReaderLimitReached:
			ts.TracesDropped.PayloadTooLarge.Add(tracen)
		case io.EOF, io.ErrUnexpectedEOF:
			ts.TracesDropped.EOF.Add(tracen)
		case msgp.ErrShortBytes:
			ts.TracesDropped.MSGPShortBytes.Add(tracen)
		default:
			if err, ok := err.(net.Error); ok && err.Timeout() {
				ts.TracesDropped.Timeout.Add(tracen)
			} else {
				ts.TracesDropped.DecodingError.Add(tracen)
			}
		}
		log.Errorf("Cannot decode %s traces payload: %v", V10, err)
		return
	}
	if n, ok := r.replyOK(req, V10, w); ok {
		tags := append(ts.AsTags(), "endpoint:traces_"+string(V10))
		_ = r.statsd.Histogram("datadog.trace_agent.receiver.rate_response_bytes", float64(n), tags, 1)
	}

	ts.TracesReceived.Add(int64(len(tp.Chunks)))
	ts.TracesBytes.Add(req.Body.(*apiutil.LimitedReader).Count)
	ts.PayloadAccepted.Inc()

	ctags := getContainerTagsList(r.conf.ContainerTags, tp.ContainerID())
	if len(ctags) > 0 {
		tp.SetStringAttribute(tagContainersTags, strings.Join(ctags, ","))
	}

	payload := &PayloadV1{
		Source:                 ts,
		TracerPayload:          tp,
		ClientComputedTopLevel: isHeaderTrue(header.ComputedTopLevel, req.Header.Get(header.ComputedTopLevel)),
		ClientComputedStats:    isHeaderTrue(header.ComputedStats, req.Header.Get(header.ComputedStats)),
		ClientDroppedP0s:       droppedTracesFromHeader(req.Header, ts),
		ContainerTags:          ctags,
	}
	r.outV1 <- payload
}

// handleTraces knows how to handle a bunch of traces
func (r *HTTPReceiver) handleTraces(v Version, w http.ResponseWriter, req *http.Request) {
	r.wg.Add(1)
	defer r.wg.Done()
	if v == V10 {
		r.handleTracesV1(w, req)
		return
	}
	tracen, err := traceCount(req)
	if err == errInvalidHeaderTraceCountValue {
		log.Errorf("Failed to count traces: %s", err)
	}
	defer req.Body.Close()

	select {
	// Wait for the semaphore to become available, allowing the handler to
	// decode its payload.
	// After the configured timeout, respond without ingesting the payload,
	// and sending the configured status.
	case r.recvsem <- struct{}{}:
	case <-time.After(time.Duration(r.conf.DecoderTimeout) * time.Millisecond):
		log.Debugf("trace-agent is overwhelmed, a payload has been rejected")
		// this payload can not be accepted
		io.Copy(io.Discard, req.Body) //nolint:errcheck
		switch v {
		case v01, v02, v03:
			// do nothing
		default:
			w.Header().Set("Content-Type", "application/json")
		}
		if isHeaderTrue(header.SendRealHTTPStatus, req.Header.Get(header.SendRealHTTPStatus)) {
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(r.rateLimiterResponse)
		}
		r.replyOK(req, v, w)
		r.tagStats(v, req.Header, "").PayloadRefused.Inc()
		return
	}
	defer func() {
		// Signal the semaphore that we are done decoding, so another handler
		// routine can take a turn decoding a payload.
		<-r.recvsem
	}()

	firstService := func(tp *pb.TracerPayload) string {
		if tp == nil || len(tp.Chunks) == 0 || len(tp.Chunks[0].Spans) == 0 {
			return ""
		}
		return tp.Chunks[0].Spans[0].Service
	}

	start := time.Now()
	tp, err := decodeTracerPayload(v, req, r.containerIDProvider, req.Header.Get(header.Lang), req.Header.Get(header.LangVersion), req.Header.Get(header.TracerVersion))
	ts := r.tagStats(v, req.Header, firstService(tp))
	defer func(err error) {
		tags := append(ts.AsTags(), fmt.Sprintf("success:%v", err == nil))
		_ = r.statsd.Histogram("datadog.trace_agent.receiver.serve_traces_ms", float64(time.Since(start))/float64(time.Millisecond), tags, 1)
	}(err)
	if err != nil {
		httpDecodingError(err, []string{"handler:traces", fmt.Sprintf("v:%s", v)}, w, r.statsd)
		switch err {
		case apiutil.ErrLimitedReaderLimitReached:
			ts.TracesDropped.PayloadTooLarge.Add(tracen)
		case io.EOF, io.ErrUnexpectedEOF:
			ts.TracesDropped.EOF.Add(tracen)
		case msgp.ErrShortBytes:
			ts.TracesDropped.MSGPShortBytes.Add(tracen)
		default:
			if err, ok := err.(net.Error); ok && err.Timeout() {
				ts.TracesDropped.Timeout.Add(tracen)
			} else {
				ts.TracesDropped.DecodingError.Add(tracen)
			}
		}
		log.Errorf("Cannot decode %s traces payload: %v", v, err)
		return
	}
	if n, ok := r.replyOK(req, v, w); ok {
		tags := append(ts.AsTags(), "endpoint:traces_"+string(v))
		_ = r.statsd.Histogram("datadog.trace_agent.receiver.rate_response_bytes", float64(n), tags, 1)
	}

	ts.TracesReceived.Add(int64(len(tp.Chunks)))
	ts.TracesBytes.Add(req.Body.(*apiutil.LimitedReader).Count)
	ts.PayloadAccepted.Inc()
	ctags := getContainerTagsList(r.conf.ContainerTags, tp.ContainerID)
	if len(ctags) > 0 {
		if tp.Tags == nil {
			tp.Tags = make(map[string]string)
		}
		tp.Tags[tagContainersTags] = strings.Join(ctags, ",")
	}
	ptags := getProcessTags(req.Header, tp)
	if ptags != "" {
		if tp.Tags == nil {
			tp.Tags = make(map[string]string)
		}
		tp.Tags[tagProcessTags] = ptags
	}
	payload := &Payload{
		Source:                 ts,
		TracerPayload:          tp,
		ClientComputedTopLevel: isHeaderTrue(header.ComputedTopLevel, req.Header.Get(header.ComputedTopLevel)),
		ClientComputedStats:    isHeaderTrue(header.ComputedStats, req.Header.Get(header.ComputedStats)),
		ClientDroppedP0s:       droppedTracesFromHeader(req.Header, ts),
		ProcessTags:            ptags,
		ContainerTags:          ctags,
	}
	r.out <- payload
}

// isHeaderTrue returns true if value is non-empty and not a "false"-like value as defined by strconv.ParseBool
// e.g. (0, f, F, FALSE, False, false) will be considered false while all other values will be true.
func isHeaderTrue(key, value string) bool {
	if len(value) == 0 {
		return false
	}
	bval, err := strconv.ParseBool(value)
	if err != nil {
		log.Debug("Non-boolean value %s found in header %s, defaulting to true", value, key)
		return true
	}
	return bval
}

func droppedTracesFromHeader(h http.Header, ts *info.TagStats) int64 {
	var dropped int64
	if v := h.Get(header.DroppedP0Traces); v != "" {
		count, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			dropped = count
			ts.ClientDroppedP0Traces.Add(count)
		}
	}
	if v := h.Get(header.DroppedP0Spans); v != "" {
		count, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			ts.ClientDroppedP0Spans.Add(count)
		}
	}
	return dropped
}

// todo:raphael cleanup unused methods of extraction once implementation
// in all tracers is completed
// order of priority:
// 1. tags in the v07 payload
// 2. tags in the first span of the first chunk
// 3. tags in the header
func getProcessTags(h http.Header, p *pb.TracerPayload) string {
	if p.Tags != nil {
		if ptags, ok := p.Tags[tagProcessTags]; ok {
			return ptags
		}
	}
	if span, ok := getFirstSpan(p); ok {
		if ptags, ok := span.Meta[tagProcessTags]; ok {
			return ptags
		}
	}
	return h.Get(header.ProcessTags)
}

func getFirstSpan(p *pb.TracerPayload) (*pb.Span, bool) {
	if len(p.Chunks) == 0 {
		return nil, false
	}
	for _, chunk := range p.Chunks {
		if chunk == nil || len(chunk.Spans) == 0 {
			continue
		}
		if chunk.Spans[0] == nil {
			continue
		}
		return chunk.Spans[0], true
	}
	return nil, false
}

// handleServices handle a request with a list of several services
func (r *HTTPReceiver) handleServices(_ Version, w http.ResponseWriter, _ *http.Request) {
	httpOK(w)

	// Do nothing, services are no longer being sent to Datadog as of July 2019
	// and are now automatically extracted from traces.
}

// loop periodically submits stats about the receiver to statsd
func (r *HTTPReceiver) loop() {
	defer close(r.exit)

	var lastLog time.Time
	accStats := info.NewReceiverStats(r.conf.SendAllInternalStats)

	t := time.NewTicker(5 * time.Second)
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
			_ = r.statsd.Gauge("datadog.trace_agent.heartbeat", 1, nil, 1)
			if cap(r.out) == 0 {
				_ = r.statsd.Gauge("datadog.trace_agent.receiver.out_chan_fill", 0, []string{"is_trace_buffer_set:false"}, 1)
			} else if cap(r.out) > 0 {
				_ = r.statsd.Gauge("datadog.trace_agent.receiver.out_chan_fill", float64(len(r.out))/float64(cap(r.out)), []string{"is_trace_buffer_set:true"}, 1)
			}

			// We update accStats with the new stats we collected
			accStats.Acc(r.Stats)

			// Publish and reset the stats accumulated during the last flush
			r.Stats.PublishAndReset(r.statsd)

			if now.Sub(lastLog) >= time.Minute {
				// We expose the stats accumulated to expvar
				info.UpdateReceiverStats(accStats)

				// We reset the stats accumulated during the last minute
				accStats.LogAndResetStats()
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
	cpu, _ := r.info.CPU(now)
	wi := watchdog.Info{
		Mem: r.info.Mem(),
		CPU: cpu,
	}
	if r.conf.MaxMemory > 0 {
		if current, allowed := float64(wi.Mem.Alloc), r.conf.MaxMemory*1.5; current > allowed {
			// This is a safety mechanism: if the agent is using more than 1.5x max. memory, there
			// is likely a leak somewhere; we'll kill the process to avoid polluting host memory.
			_ = r.statsd.Count("datadog.trace_agent.receiver.oom_kill", 1, nil, 1)
			r.statsd.Flush()
			log.Criticalf("Killing process. Memory threshold exceeded: %.2fM / %.2fM", current/1024/1024, allowed/1024/1024)
			killProcess("OOM")
		}
	}
	info.UpdateWatchdogInfo(wi)

	_ = r.statsd.Gauge("datadog.trace_agent.heap_alloc", float64(wi.Mem.Alloc), nil, 1)
	_ = r.statsd.Gauge("datadog.trace_agent.cpu_percent", wi.CPU.UserAvg*100, nil, 1)
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
func decodeRequest(req *http.Request, dest *pb.Traces) error {
	switch mediaType := getMediaType(req); mediaType {
	case "application/msgpack":
		buf := getBuffer()
		defer putBuffer(buf)
		_, err := copyRequestBody(buf, req)
		if err != nil {
			return err
		}
		_, err = dest.UnmarshalMsg(buf.Bytes())
		return err
	case "application/json":
		fallthrough
	case "text/json":
		fallthrough
	case "":
		return json.NewDecoder(req.Body).Decode(&dest)
	default:
		// do our best
		if err1 := json.NewDecoder(req.Body).Decode(&dest); err1 != nil {
			buf := getBuffer()
			defer putBuffer(buf)
			_, err2 := copyRequestBody(buf, req)
			if err2 != nil {
				return err2
			}
			_, err2 = dest.UnmarshalMsg(buf.Bytes())
			return err2
		}
		return nil
	}
}

func traceChunksFromSpans(spans []*pb.Span) []*pb.TraceChunk {
	traceChunks := []*pb.TraceChunk{}
	byID := make(map[uint64][]*pb.Span)
	for _, s := range spans {
		byID[s.TraceID] = append(byID[s.TraceID], s)
	}
	for _, t := range byID {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    t,
			Tags:     make(map[string]string),
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
			Tags:     make(map[string]string),
		})
	}

	return traceChunks
}

func getContainerTagsList(fn func(string) ([]string, error), containerID string) []string {
	if containerID == "" {
		return nil
	}
	if fn == nil {
		log.Warn("ContainerTags not configured")
		return nil
	}
	list, err := fn(containerID)
	if err != nil {
		log.Tracef("Getting container tags for ID %q: %v", containerID, err)
		return nil
	}
	log.Tracef("Getting container tags for ID %q: %v", containerID, list)
	return list
}

// getContainerTag returns container and orchestrator tags belonging to containerID. If containerID
// is empty or no tags are found, an empty string is returned.
func getContainerTags(fn func(string) ([]string, error), containerID string) string {
	ctags := getContainerTagsList(fn, containerID)
	return strings.Join(ctags, ",")
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

// normalizeHTTPHeader takes a raw string and normalizes the value to be compatible
// with an HTTP header value.
func normalizeHTTPHeader(val string) string {
	val = strings.ReplaceAll(val, "\r\n", "_")
	val = strings.ReplaceAll(val, "\r", "_")
	val = strings.ReplaceAll(val, "\n", "_")
	return val
}
