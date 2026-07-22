// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var (
	requestStatsPool = ddsync.NewTypedPool[RequestStats](NewRequestStats)

	// emptyInternedPath is the shared interned representation of an empty
	// path used in discovery mode, where the path is not part of the key.
	emptyInternedPath = Interner.Get([]byte{})
)

const (
	// discoveryMaxStatsBuffered is the max entries for discovery mode.
	// With path+method removed from the key, cardinality drops by orders
	// of magnitude so a much smaller buffer suffices.
	discoveryMaxStatsBuffered = 5000
)

// StatKeeper is responsible for aggregating HTTP stats.
type StatKeeper struct {
	mux                  sync.Mutex
	stats                map[Key]*RequestStats
	incomplete           IncompleteBuffer
	maxEntries           int
	discoveryMode        bool
	quantizer            *URLQuantizer
	telemetry            *Telemetry
	connectionAggregator *utils.ConnectionAggregator

	// replace rules for HTTP path
	replaceRules []*config.ReplaceRule

	// http path buffer
	buffer []byte

	oversizedLogLimit *log.Limit

	// LLMO PoC: eBPF map used to flag connections whose decrypted bodies the
	// hooks should capture. nil unless EnableLLMO has been called (HTTP/2 only).
	llmConnMap *ebpf.Map
	// llmServiceExtractor resolves the span service name from the client PID in
	// userspace, using the same inference as USM (process_service_inference).
	llmServiceExtractor *parser.ServiceExtractor
	// llmGenUsage caches, per connection, the token usage of the most recent
	// tool-call generation seen on it (detected via finish_reason in the
	// response tail, which — unlike the head — is reliably captured). A
	// follow-up request on the same connection reads it back to give the
	// workflow's first llm span its cost, surviving response-slot churn.
	llmGenUsage   map[llmConnKey]llmUsage
	llmGenUsageMu sync.Mutex
	// llmRespReader consumes streamed response events (see llmo.go).
	llmRespReader *ringbuf.Reader
	// llmRespReasm reassembles multi-read responses, keyed by (conn, stream) so
	// concurrent streams on one connection don't interleave into one buffer.
	// Touched only by the response-consumer goroutine, so it needs no lock.
	llmRespReasm map[llmStreamKey]*llmRespReasm
	// llmReqByStream holds each streamed, parsed request body keyed by its
	// (conn, HTTP/2 stream) so the response consumer can pair a response with
	// its exact request — correct even when one connection carries several
	// conversations (sequentially or multiplexed). Written by the request
	// consumer goroutine, read/deleted by the response consumer goroutine.
	llmReqByStream map[llmStreamKey]llmReqParsed
	llmReqMu       sync.Mutex
	// llmReqReader consumes streamed request-body events (see llmo.go).
	llmReqReader *ringbuf.Reader
	// llmEmit emits a fully built span. Defaults to emitLLMSpan; overridable in
	// tests to capture what got paired without starting the real tracer.
	llmEmit func(string, Method, uint16, types.ConnectionKey, float64, llmSpanInfo)
	// llmConvAgents holds one long-lived agent span per conversation thread
	// (keyed by session id + the conversation's first user message, which is
	// re-sent on every turn), so all the LLM/tool calls of one multi-turn
	// conversation nest under a single agent flow. session_id is still tagged on
	// the agent so the UI's Sessions view groups conversations. Finished by the
	// reaper once the conversation goes idle. Guarded by llmConvMu.
	llmConvAgents map[string]*llmConvAgent
	llmConvMu     sync.Mutex
}

// llmStreamKey identifies one HTTP/2 request/response exchange: a connection
// plus the stream id it was carried on. Keying by stream (not just connection)
// pairs each response with its exact request even when one connection carries
// several conversations.
type llmStreamKey struct {
	conn   llmConnKey
	stream uint32
}

// llmStreamMapCap bounds the stream-keyed request/reassembly maps so orphaned
// entries (a request with no response, or a response that never completes)
// can't grow memory without bound (PoC-sized).
const llmStreamMapCap = 8192

// EnableLLMO wires up the eBPF connection-flag map used to gate body capture.
// Once enabled, the request/response ring-buffer consumers (started separately)
// parse the captured bodies, pair them by (conn, stream), and emit LLM spans
// enriched with model, prompt, response, token usage, and a service name
// resolved from the client PID.
func (h *StatKeeper) EnableLLMO(connMap *ebpf.Map) {
	h.llmConnMap = connMap
	// Same inference USM uses for service names (enabled, non-Windows,
	// improved algorithm).
	h.llmServiceExtractor = parser.NewServiceExtractor(true, false, true)
	h.llmGenUsage = make(map[llmConnKey]llmUsage)
	h.llmReqByStream = make(map[llmStreamKey]llmReqParsed)
	h.llmRespReasm = make(map[llmStreamKey]*llmRespReasm)
	h.llmConvAgents = make(map[string]*llmConvAgent)
}

// storeReq records a streamed, parsed request body under its (conn, stream)
// key for the response consumer to pair with. If the map is at capacity (an
// orphaned request built up with no matching response), one arbitrary entry is
// evicted first so memory stays bounded.
func (h *StatKeeper) storeReq(key llmStreamKey, req llmReqParsed) {
	if h.llmReqByStream == nil {
		return
	}
	h.llmReqMu.Lock()
	defer h.llmReqMu.Unlock()
	if len(h.llmReqByStream) >= llmStreamMapCap {
		for k := range h.llmReqByStream {
			delete(h.llmReqByStream, k)
			break
		}
	}
	h.llmReqByStream[key] = req
}

// takeReq removes and returns the request stored for a (conn, stream); ok is
// false when none was stored (e.g. warm-up, before the connection was flagged,
// or the request event was lost).
func (h *StatKeeper) takeReq(key llmStreamKey) (llmReqParsed, bool) {
	if h.llmReqByStream == nil {
		return llmReqParsed{}, false
	}
	h.llmReqMu.Lock()
	defer h.llmReqMu.Unlock()
	req, ok := h.llmReqByStream[key]
	if ok {
		delete(h.llmReqByStream, key)
	}
	return req, ok
}

// cacheGenUsage records, per connection, the token usage of a tool-call
// generation, for later attribution to a workflow's first llm span.
func (h *StatKeeper) cacheGenUsage(key llmConnKey, u llmUsage) {
	if h.llmGenUsage == nil || u.total == 0 {
		return
	}
	h.llmGenUsageMu.Lock()
	defer h.llmGenUsageMu.Unlock()
	// Bound the map; this is a PoC-sized cache.
	if len(h.llmGenUsage) > 4096 {
		h.llmGenUsage = make(map[llmConnKey]llmUsage)
	}
	h.llmGenUsage[key] = u
}

// lookupGenUsage returns the cached tool-call-generation usage for a connection.
func (h *StatKeeper) lookupGenUsage(key llmConnKey) (llmUsage, bool) {
	if h.llmGenUsage == nil {
		return llmUsage{}, false
	}
	h.llmGenUsageMu.Lock()
	defer h.llmGenUsageMu.Unlock()
	u, ok := h.llmGenUsage[key]
	return u, ok
}

// NewStatkeeper returns a new StatKeeper.
func NewStatkeeper(c *config.Config, telemetry *Telemetry, incompleteBuffer IncompleteBuffer) *StatKeeper {
	var quantizer *URLQuantizer
	// For now we're only enabling path quantization for HTTP/1 traffic
	if c.EnableUSMQuantization {
		quantizer = NewURLQuantizer()
	}

	var connectionAggregator *utils.ConnectionAggregator
	if c.EnableUSMConnectionRollup {
		connectionAggregator = utils.NewConnectionAggregator()
	}

	if len(c.HTTPReplaceRules) > 0 {
		// Sort rules, and place drop rules first
		slices.SortStableFunc(c.HTTPReplaceRules, func(a, b *config.ReplaceRule) int {
			if a.Repl == "" && b.Repl == "" {
				return 0
			}
			if a.Repl == "" {
				return -1
			}
			if b.Repl == "" {
				return 1
			}
			return 0
		})
	}

	maxEntries := c.MaxHTTPStatsBuffered
	if c.DiscoveryServiceMapEnabled {
		maxEntries = discoveryMaxStatsBuffered
		log.Infof("http statkeeper running in discovery mode: path/method dropped from key, max_stats_buffered=%d", maxEntries)
	}

	return &StatKeeper{
		stats:                make(map[Key]*RequestStats),
		incomplete:           incompleteBuffer,
		maxEntries:           maxEntries,
		discoveryMode:        c.DiscoveryServiceMapEnabled,
		quantizer:            quantizer,
		replaceRules:         c.HTTPReplaceRules,
		connectionAggregator: connectionAggregator,
		buffer:               make([]byte, getPathBufferSize(c)),
		telemetry:            telemetry,
		oversizedLogLimit:    log.NewLogLimit(10, time.Minute*10),
	}
}

// Process processes a transaction and updates the stats accordingly.
func (h *StatKeeper) Process(tx Transaction) {
	h.mux.Lock()
	defer h.mux.Unlock()

	if tx.Incomplete() {
		h.incomplete.Add(tx)
		return
	}

	h.add(tx)
}

// GetAndResetAllStats returns all the stats and resets the internal state.
func (h *StatKeeper) GetAndResetAllStats() (stats map[Key]*RequestStats) {
	var previousAggregationState *utils.ConnectionAggregator
	func() {
		h.mux.Lock()
		defer h.mux.Unlock()

		for _, tx := range h.incomplete.Flush() {
			h.add(tx)
		}

		// Rotate stats
		stats = h.stats
		h.stats = make(map[Key]*RequestStats)

		// Rotate ConnectionAggregator
		if h.connectionAggregator == nil {
			// Feature not enabled
			return
		}

		previousAggregationState = h.connectionAggregator
		h.connectionAggregator = utils.NewConnectionAggregator()
	}()

	h.clearEphemeralPorts(previousAggregationState, stats)
	return stats
}

// Close closes the stat keeper.
func (h *StatKeeper) Close() {
	if h.llmRespReader != nil {
		h.llmRespReader.Close()
	}
	if h.llmReqReader != nil {
		h.llmReqReader.Close()
	}
}

var (
	// grpcPattern is a regex pattern to match gRPC paths by the pattern of `/<package>.<service>/<url>`
	// Note - <service> can contain dots by itself. For instance `/google.pubsub.v2.PublisherService/CreateTopic`
	grpcPattern = regexp.MustCompile(`^/([^./]+(\.[^./]+)*?)\.([^./]+(\.[^./]+)*?)/([^./]+?)$`)
)

func (h *StatKeeper) add(tx Transaction) {
	if h.discoveryMode {
		h.addDiscovery(tx)
		return
	}

	rawPath, fullPath := tx.Path(h.buffer)
	if rawPath == nil {
		h.telemetry.emptyPath.Add(1)
		return
	}

	// LLMO PoC: detect LLM API traffic by path (USM does not capture the
	// Host/:authority header) and remember the un-quantized full path so we
	// can emit it as the span resource below.
	llmTraffic := isLLMPath(rawPath)

	// Quantize HTTP path
	// (eg. this turns /orders/123/view` into `/orders/*/view`)
	if h.quantizer != nil {
		// Quantize the endpoint if and only if, it is not a gRPC captured by HTTP2 monitoring
		if tx.Method() != MethodPost || !grpcPattern.Match(rawPath) {
			rawPath = h.quantizer.Quantize(rawPath)
		}
	}

	path, rejected := h.processHTTPPath(tx, rawPath)
	if rejected {
		return
	}

	if tx.Method() == MethodUnknown {
		h.telemetry.unknownMethod.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("method should never be unknown: %s", tx.String())
		}
		return
	}

	latency := tx.RequestLatency()
	if latency <= 0 {
		h.telemetry.invalidLatency.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("latency should never be non positive: %s", tx.String())
		}
		return
	}

	// Validate HTTP status code
	statusCode := tx.StatusCode()
	if !isValidStatusCode(statusCode) {
		h.telemetry.invalidStatusCode.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("invalid status code: %s", tx.String())
		}
		return
	}

	// LLMO PoC: flag the connection so the eBPF hooks capture its decrypted
	// request/response bodies. Spans are no longer emitted here — the transaction
	// is processed out of wire order and has no HTTP/2 stream id, so it can't pair
	// a response to its request. Emission moved to the response consumer, which
	// sees events in wire order and keys them by (conn, stream) — see pairAndEmit.
	if llmTraffic {
		h.flagLLMConn(tx.ConnTuple())
	}

	key := NewKeyWithConnection(tx.ConnTuple(), path, fullPath, tx.Method())
	if h.connectionAggregator != nil {
		key.ConnectionKey = h.connectionAggregator.RollupKey(key.ConnectionKey)
	}

	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			h.telemetry.dropped.Add(1)
			return
		}
		h.telemetry.aggregations.Add(1)
		stats = requestStatsPool.Get()
		h.stats[key] = stats
	}

	dynamicTagsSet := common.StringSet(nil)
	if dynamicTags := tx.DynamicTags(); len(dynamicTags) > 0 {
		dynamicTagsSet = common.NewStringSet(dynamicTags...)
	}
	stats.AddRequest(tx.StatusCode(), latency, tx.StaticTags(), dynamicTagsSet)
}

// addDiscovery is the fast path for discovery mode.
// It skips path extraction, path validation, path-based filter rules,
// and malformed path checks. The aggregation key is just the ConnectionKey
// (no path, no method), so 1000 unique URLs to the same service:port
// collapse into one entry.
func (h *StatKeeper) addDiscovery(tx Transaction) {
	latency := tx.RequestLatency()
	if latency <= 0 {
		h.telemetry.invalidLatency.Add(1)
		return
	}

	if tx.Method() == MethodUnknown {
		h.telemetry.unknownMethod.Add(1)
		return
	}

	statusCode := tx.StatusCode()
	if !isValidStatusCode(statusCode) {
		h.telemetry.invalidStatusCode.Add(1)
		return
	}

	key := Key{
		ConnectionKey: tx.ConnTuple(),
		Path:          Path{Content: emptyInternedPath},
	}
	if h.connectionAggregator != nil {
		key.ConnectionKey = h.connectionAggregator.RollupKey(key.ConnectionKey)
	}

	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			h.telemetry.dropped.Add(1)
			return
		}
		h.telemetry.aggregations.Add(1)
		stats = requestStatsPool.Get()
		h.stats[key] = stats
	}

	dynamicTagsSet := common.StringSet(nil)
	if dynamicTags := tx.DynamicTags(); len(dynamicTags) > 0 {
		dynamicTagsSet = common.NewStringSet(dynamicTags...)
	}
	stats.AddDiscoveryRequest(statusCode, latency, tx.StaticTags(), dynamicTagsSet)
}

func pathIsMalformed(fullPath []byte) bool {
	for _, r := range fullPath {
		if !strconv.IsPrint(rune(r)) {
			return true
		}
	}
	return false
}

func (h *StatKeeper) processHTTPPath(tx Transaction, path []byte) ([]byte, bool) {
	match := false
	for _, r := range h.replaceRules {
		if r.Re.Match(path) {
			if r.Repl == "" {
				// this is a "drop" rule
				h.telemetry.rejected.Add(1)
				return nil, true
			}

			path = r.Re.ReplaceAll(path, []byte(r.Repl))
			match = true
		}
	}

	// If the user didn't specify a rule matching this particular path, we can check for its format.
	// Otherwise, we don't want the custom path to be rejected by our path formatting check.
	if !match && pathIsMalformed(path) {
		if h.oversizedLogLimit.ShouldLog() {
			log.Debugf("http path malformed: %+v %s", tx.ConnTuple(), tx.String())
		}
		h.telemetry.nonPrintableCharacters.Add(1)
		return nil, true
	}
	return path, false
}

func (h *StatKeeper) clearEphemeralPorts(aggregator *utils.ConnectionAggregator, stats map[Key]*RequestStats) {
	if aggregator == nil {
		return
	}

	// Re-index entries that were generated from multiple connections
	// See comments on `ConnectionAggregator.ClearEphemeralPort()` for more context
	for key, aggregation := range stats {
		newConnKey := aggregator.ClearEphemeralPort(key.ConnectionKey)
		if newConnKey == key.ConnectionKey {
			continue
		}

		delete(stats, key)
		key.ConnectionKey = newConnKey
		stats[key] = aggregation
	}
}
