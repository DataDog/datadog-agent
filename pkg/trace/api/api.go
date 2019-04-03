package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"mime"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tinylib/msgp/msgp"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRequestBodyLength = 10 * 1024 * 1024
	tagTraceHandler      = "handler:traces"
	tagServiceHandler    = "handler:services"
)

// Version is a dumb way to version our collector handlers
type Version string

const (
	// v01 DEPRECATED, FIXME[1.x]
	// Traces: JSON, slice of spans
	// Services: JSON, map[string]map[string][string]
	v01 Version = "v0.1"
	// v02 DEPRECATED, FIXME[1.x]
	// Traces: JSON, slice of traces
	// Services: JSON, map[string]map[string][string]
	v02 Version = "v0.2"
	// v03
	// Traces: msgpack/JSON (Content-Type) slice of traces
	// Services: msgpack/JSON, map[string]map[string][string]
	v03 Version = "v0.3"
	// v04
	// Traces: msgpack/JSON (Content-Type) slice of traces + returns service sampling ratios
	// Services: msgpack/JSON, map[string]map[string][string]
	v04 Version = "v0.4"
)

// HTTPReceiver is a collector that uses HTTP protocol and just holds
// a chan where the spans received are sent one by one
type HTTPReceiver struct {
	Stats      *info.ReceiverStats
	PreSampler *sampler.PreSampler
	Out        chan pb.Trace

	services chan pb.ServicesMetadata
	conf     *config.AgentConfig
	dynConf  *sampler.DynamicConfig
	server   *http.Server

	maxRequestBodyLength int64
	debug                bool
	refuse               int64 // when set to 1 agent will refuse payloads

	exit chan struct{}
}

// NewHTTPReceiver returns a pointer to a new HTTPReceiver
func NewHTTPReceiver(
	conf *config.AgentConfig, dynConf *sampler.DynamicConfig, out chan pb.Trace, services chan pb.ServicesMetadata,
) *HTTPReceiver {
	// use buffered channels so that handlers are not waiting on downstream processing
	return &HTTPReceiver{
		Stats:      info.NewReceiverStats(),
		PreSampler: sampler.NewPreSampler(),
		Out:        out,

		conf:     conf,
		dynConf:  dynConf,
		services: services,

		maxRequestBodyLength: maxRequestBodyLength,
		debug:                strings.ToLower(conf.LogLevel) == "debug",

		exit: make(chan struct{}),
	}
}

// Start starts doing the HTTP server and is ready to receive traces
func (r *HTTPReceiver) Start() {
	// FIXME[1.x]: remove all those legacy endpoints + code that goes with it
	http.HandleFunc("/spans", r.httpHandleWithVersion(v01, r.handleTraces))
	http.HandleFunc("/services", r.httpHandleWithVersion(v01, r.handleServices))
	http.HandleFunc("/v0.1/spans", r.httpHandleWithVersion(v01, r.handleTraces))
	http.HandleFunc("/v0.1/services", r.httpHandleWithVersion(v01, r.handleServices))
	http.HandleFunc("/v0.2/traces", r.httpHandleWithVersion(v02, r.handleTraces))
	http.HandleFunc("/v0.2/services", r.httpHandleWithVersion(v02, r.handleServices))
	http.HandleFunc("/v0.3/traces", r.httpHandleWithVersion(v03, r.handleTraces))
	http.HandleFunc("/v0.3/services", r.httpHandleWithVersion(v03, r.handleServices))

	// current collector API
	http.HandleFunc("/v0.4/traces", r.httpHandleWithVersion(v04, r.handleTraces))
	http.HandleFunc("/v0.4/services", r.httpHandleWithVersion(v04, r.handleServices))

	// expvar implicitly publishes "/debug/vars" on the same port
	addr := fmt.Sprintf("%s:%d", r.conf.ReceiverHost, r.conf.ReceiverPort)
	if err := r.Listen(addr, ""); err != nil {
		osutil.Exitf("%v", err)
	}

	go r.PreSampler.Run()

	go func() {
		defer watchdog.LogOnPanic()
		r.loop()
	}()
}

// Listen creates a new HTTP server listening on the provided address.
func (r *HTTPReceiver) Listen(addr, logExtra string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %v", addr, err)
	}

	ln, err := newRateLimitedListener(listener, r.conf.ConnectionLimit)
	if err != nil {
		return fmt.Errorf("cannot create listener: %v", err)
	}
	timeout := 5 * time.Second
	if r.conf.ReceiverTimeout > 0 {
		timeout = time.Duration(r.conf.ReceiverTimeout) * time.Second
	}
	r.server = &http.Server{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		ErrorLog:     stdlog.New(writableFunc(log.Error), "http.Server: ", 0),
	}
	log.Infof("listening for traces at http://%s%s", addr, logExtra)

	go func() {
		defer watchdog.LogOnPanic()
		ln.Refresh(r.conf.ConnectionLimit)
	}()
	go func() {
		defer watchdog.LogOnPanic()
		r.server.Serve(ln)
	}()

	return nil
}

// Stop stops the receiver and shuts down the HTTP server.
func (r *HTTPReceiver) Stop() error {
	r.exit <- struct{}{}
	<-r.exit

	r.PreSampler.Stop()

	expiry := time.Now().Add(20 * time.Second) // give it 20 seconds
	ctx, cancel := context.WithDeadline(context.Background(), expiry)
	defer cancel()
	return r.server.Shutdown(ctx)
}

func (r *HTTPReceiver) httpHandle(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		req.Body = NewLimitedReader(req.Body, r.maxRequestBodyLength)
		defer req.Body.Close()

		fn(w, req)
	}
}

func (r *HTTPReceiver) httpHandleWithVersion(v Version, f func(Version, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return r.httpHandle(func(w http.ResponseWriter, req *http.Request) {
		mediaType := getMediaType(req)
		if mediaType == "application/msgpack" && (v == v01 || v == v02) {
			// msgpack is only supported for versions 0.3
			httpFormatError(w, v, fmt.Errorf("unsupported media type: %q", mediaType))
			return
		}

		f(v, w, req)
	})
}

func (r *HTTPReceiver) replyTraces(v Version, w http.ResponseWriter) {
	switch v {
	case v01:
		fallthrough
	case v02:
		fallthrough
	case v03:
		// Simple response, simply acknowledge with "OK"
		httpOK(w)
	case v04:
		// Return the recommended sampling rate for each service as a JSON.
		httpRateByService(w, r.dynConf)
	}
}

// handleTraces knows how to handle a bunch of traces
func (r *HTTPReceiver) handleTraces(v Version, w http.ResponseWriter, req *http.Request) {
	if atomic.LoadInt64(&r.refuse) == 1 {
		// using too much memory
		io.Copy(ioutil.Discard, req.Body)
		w.WriteHeader(http.StatusNotAcceptable)
		io.WriteString(w, "request rejected; trace-agent is past memory threshold (apm_config.max_memory)")
		metrics.Gauge("datadog.trace_agent.receiver.refused", 1, []string{"reason:mem"}, 1)
		return
	}
	if !r.PreSampler.Sample(req) {
		// using too much CPU
		io.Copy(ioutil.Discard, req.Body)
		r.replyTraces(v, w)
		metrics.Gauge("datadog.trace_agent.receiver.refused", 1, []string{"reason:cpu"}, 1)
		return
	}

	traces, ok := getTraces(v, w, req)
	if !ok {
		return
	}

	// We successfully decoded the payload
	r.replyTraces(v, w)

	// We parse the tags from the header
	tags := info.Tags{
		Lang:          req.Header.Get("Datadog-Meta-Lang"),
		LangVersion:   req.Header.Get("Datadog-Meta-Lang-Version"),
		Interpreter:   req.Header.Get("Datadog-Meta-Lang-Interpreter"),
		TracerVersion: req.Header.Get("Datadog-Meta-Tracer-Version"),
	}

	// We get the address of the struct holding the stats associated to the tags
	ts := r.Stats.GetTagStats(tags)

	bytesRead := req.Body.(*LimitedReader).Count
	if bytesRead > 0 {
		atomic.AddInt64(&ts.TracesBytes, int64(bytesRead))
	}

	// normalize data
	for _, trace := range traces {
		spans := len(trace)

		atomic.AddInt64(&ts.TracesReceived, 1)
		atomic.AddInt64(&ts.SpansReceived, int64(spans))

		err := normalizeTrace(trace)
		if err != nil {
			atomic.AddInt64(&ts.TracesDropped, 1)
			atomic.AddInt64(&ts.SpansDropped, int64(spans))

			msg := fmt.Sprintf("dropping trace; reason: %s", err)
			if len(msg) > 150 && !r.debug {
				// we're not in DEBUG log level, truncate long messages.
				msg = msg[:150] + "... (set DEBUG log level for more info)"
			}
			log.Errorf(msg)
			continue
		}

		r.Out <- trace
	}
}

// handleServices handle a request with a list of several services
func (r *HTTPReceiver) handleServices(v Version, w http.ResponseWriter, req *http.Request) {
	var servicesMeta pb.ServicesMetadata

	mediaType := getMediaType(req)
	if err := decodeReceiverPayload(req.Body, &servicesMeta, v, mediaType); err != nil {
		log.Errorf("cannot decode %s services payload: %v", v, err)
		httpDecodingError(err, []string{tagServiceHandler, fmt.Sprintf("v:%s", v)}, w)
		return
	}

	httpOK(w)

	// We parse the tags from the header
	tags := info.Tags{
		Lang:          req.Header.Get("Datadog-Meta-Lang"),
		LangVersion:   req.Header.Get("Datadog-Meta-Lang-Version"),
		Interpreter:   req.Header.Get("Datadog-Meta-Lang-Interpreter"),
		TracerVersion: req.Header.Get("Datadog-Meta-Tracer-Version"),
	}

	// We get the address of the struct holding the stats associated to the tags
	ts := r.Stats.GetTagStats(tags)

	atomic.AddInt64(&ts.ServicesReceived, int64(len(servicesMeta)))

	bytesRead := req.Body.(*LimitedReader).Count
	if bytesRead > 0 {
		atomic.AddInt64(&ts.ServicesBytes, int64(bytesRead))
	}

	r.services <- servicesMeta
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

			// We update accStats with the new stats we collected
			accStats.Acc(r.Stats)

			// Publish the stats accumulated during the last flush
			r.Stats.Publish()

			// We reset the stats accumulated during the last 10s.
			r.Stats.Reset()

			if now.Sub(lastLog) >= time.Minute {
				// We expose the stats accumulated to expvar
				info.UpdateReceiverStats(accStats)

				for _, logStr := range accStats.Strings() {
					log.Info(logStr)
				}

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

func (r *HTTPReceiver) watchdog(now time.Time) {
	wi := watchdog.Info{
		Mem: watchdog.Mem(),
		CPU: watchdog.CPU(now),
	}

	rate, err := sampler.CalcPreSampleRate(r.conf.MaxCPU, wi.CPU.UserAvg, r.PreSampler.RealRate())
	if err != nil {
		log.Warnf("problem computing pre-sample rate: %v", err)
	}

	r.PreSampler.SetRate(rate)
	r.PreSampler.SetError(err)

	stats := r.PreSampler.Stats()

	info.UpdatePreSampler(*stats)
	info.UpdateWatchdogInfo(wi)

	metrics.Gauge("datadog.trace_agent.heap_alloc", float64(wi.Mem.Alloc), nil, 1)
	metrics.Gauge("datadog.trace_agent.cpu_percent", wi.CPU.UserAvg*100, nil, 1)
	metrics.Gauge("datadog.trace_agent.presampler_rate", stats.Rate, nil, 1)

	if float64(wi.Mem.Alloc) > r.conf.MaxMemory && r.conf.MaxMemory > 0 {
		log.Warn("memory exceeds threshold (apm_config.max_memory), requests will be rate-limited")
		if atomic.SwapInt64(&r.refuse, 1) != 0 {
			// we're still not accepting requests, do a garbage collection;,
			// potentially blocking the program here is the least of our problems
			runtime.GC()
		}
	} else {
		if atomic.SwapInt64(&r.refuse, 0) == 1 {
			log.Warn("memory back below threshold (apm_config.max_memory), allowing requests")
		}
	}
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

func getTraces(v Version, w http.ResponseWriter, req *http.Request) (pb.Traces, bool) {
	var traces pb.Traces
	mediaType := getMediaType(req)
	switch v {
	case v01:
		// We cannot use decodeReceiverPayload because []model.Span does not
		// implement msgp.Decodable. This hack can be removed once we
		// drop v01 support.
		switch mediaType {
		case "application/json", "text/json", "":
			// ok
		default:
			httpFormatError(w, v, fmt.Errorf("unsupported media type: %q", mediaType))
			return nil, false
		}

		// in v01 we actually get spans that we have to transform in traces
		var spans []pb.Span
		if err := json.NewDecoder(req.Body).Decode(&spans); err != nil {
			log.Errorf("cannot decode %s traces payload: %v", v, err)
			httpDecodingError(err, []string{tagTraceHandler, fmt.Sprintf("v:%s", v)}, w)
			return nil, false
		}
		traces = tracesFromSpans(spans)
	case v02:
		fallthrough
	case v03:
		fallthrough
	case v04:
		if err := decodeReceiverPayload(req.Body, &traces, v, mediaType); err != nil {
			log.Errorf("cannot decode %s traces payload: %v", v, err)
			httpDecodingError(err, []string{tagTraceHandler, fmt.Sprintf("v:%s", v)}, w)
			return nil, false
		}
	default:
		httpEndpointNotSupported([]string{tagTraceHandler, fmt.Sprintf("v:%s", v)}, w)
		return nil, false
	}

	return traces, true
}

func decodeReceiverPayload(r io.Reader, dest msgp.Decodable, v Version, mediaType string) error {
	switch mediaType {
	case "application/msgpack":
		return msgp.Decode(r, dest)
	case "application/json":
		fallthrough
	case "text/json":
		fallthrough
	case "":
		return json.NewDecoder(r).Decode(dest)
	default:
		return fmt.Errorf("unknown content type: %q", mediaType)
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

// getMediaType attempts to return the media type from the Content-Type MIME header. If it fails
// it returns the default media type "application/json".
func getMediaType(req *http.Request) string {
	mt, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		log.Debugf(`error parsing media type: %v, assuming "application/json"`, err)
		return "application/json"
	}
	return mt
}

// writableFunc implements io.Writer over a function. Anything written will be
// forwarded to the function as one string argument.
type writableFunc func(v ...interface{}) error

// Write implements io.Writer.
func (fn writableFunc) Write(p []byte) (n int, err error) {
	if err = fn(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
