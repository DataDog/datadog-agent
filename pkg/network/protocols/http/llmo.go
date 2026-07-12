// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/DataDog/dd-trace-go/v2/llmobs"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LLM Observability (LLMO) PoC.
//
// When USM observes a request whose path looks like an LLM API call, we emit a
// single APM span per request (no aggregation) so the call shows up as a trace.
// USM does not capture the HTTP Host / :authority pseudo-header, so detection is
// based on the request path only. When the connection has been flagged as LLM
// traffic, the eBPF go-tls write hook also captures a window of the decrypted
// request body, which we parse here to extract the model and the prompt.

const (
	llmoSpanName    = "llm.request"
	llmoServiceName = "llmo-usm-poc"
	llmoAgentAddr   = "localhost:8126"
	// llmoMLApp is the default LLM Observability app name (the "AI" product
	// groups spans by ml_app); overridden per-span by the resolved service.
	llmoMLApp        = "apm-lite-ebpf"
	llmoModelVendor  = "openai"

	// llmBodyBufferSize must match LLM_BODY_BUFFER_SIZE in the eBPF code
	// (pkg/network/ebpf/c/protocols/tls/llmo.h).
	llmBodyBufferSize = 1024
)

// llmConnKey mirrors both pkg/network/types.ConnectionKey and the eBPF
// llm_conn_key_t struct (4x u64 + 2x u16) so it can be used as the key of the
// llm_monitored_connections / llm_request_bodies eBPF maps.
type llmConnKey struct {
	SrcIPHigh uint64
	SrcIPLow  uint64
	DstIPHigh uint64
	DstIPLow  uint64
	SrcPort   uint16
	DstPort   uint16
	// Pad makes the struct marshal to 40 bytes, matching the C struct's
	// trailing alignment padding (the eBPF map key size). Without it,
	// cilium/ebpf marshals only 36 bytes and map ops fail.
	Pad uint32
}

// llmBody mirrors the eBPF llm_body_t struct.
type llmBody struct {
	Len  uint32
	Data [llmBodyBufferSize]byte
}

func newLLMConnKey(c types.ConnectionKey) llmConnKey {
	return llmConnKey{
		SrcIPHigh: c.SrcIPHigh,
		SrcIPLow:  c.SrcIPLow,
		DstIPHigh: c.DstIPHigh,
		DstIPLow:  c.DstIPLow,
		SrcPort:   c.SrcPort,
		DstPort:   c.DstPort,
	}
}

// llmPathPrefixes are the request-path prefixes we treat as LLM API traffic.
var llmPathPrefixes = []string{
	"/v1/chat/completions",
	"/v1/completions",
	"/v1/responses",
	"/v1/embeddings",
	"/v1/messages", // Anthropic
	"/v1/moderations",
	"/v1/audio/",
	"/v1/images/",
}

var (
	llmoTracerOnce  sync.Once
	llmoTracerReady bool

	// Tolerant extractors: the captured body is a raw window of decrypted
	// bytes (HTTP/2 DATA frame payload, possibly preceded by a frame header
	// and possibly truncated at llmBodyBufferSize), not guaranteed-valid JSON.
	llmModelRe   = regexp.MustCompile(`"model"\s*:\s*"([^"]*)"`)
	llmContentRe = regexp.MustCompile(`"content"\s*:\s*"([^"]*)"`)

	// Token usage extractors (from the response body).
	llmPromptTokRe = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	llmComplTokRe  = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	llmTotalTokRe  = regexp.MustCompile(`"total_tokens"\s*:\s*(\d+)`)
)

// llmSpanInfo holds everything parsed from the captured request/response
// bodies to enrich an LLM span.
type llmSpanInfo struct {
	service          string
	model            string
	prompt           string
	response         string
	promptTokens     int64
	completionTokens int64
	totalTokens      int64
}

// ensureLLMOTracer lazily starts the dd-trace-go tracer pointed at the local
// trace-agent. It is started on the first LLM request observed.
func ensureLLMOTracer() bool {
	llmoTracerOnce.Do(func() {
		if err := tracer.Start(
			tracer.WithService(llmoServiceName),
			tracer.WithAgentAddr(llmoAgentAddr),
			tracer.WithLLMObsEnabled(true),
			tracer.WithLLMObsMLApp(llmoMLApp),
			tracer.WithLogStartup(false),
		); err != nil {
			log.Warnf("LLMO: failed to start tracer: %v", err)
			return
		}
		llmoTracerReady = true
		log.Infof("LLMO: tracer started with LLM Observability; emitting to %s", llmoAgentAddr)
	})
	return llmoTracerReady
}

// isLLMPath reports whether the request path looks like LLM API traffic.
func isLLMPath(path []byte) bool {
	if len(path) == 0 {
		return false
	}
	p := string(path)
	for _, prefix := range llmPathPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// parseLLMBody extracts the model and prompt from a captured request body
// window. The window may contain an HTTP/2 frame header before the JSON and
// may be truncated, so extraction is tolerant rather than a strict JSON decode.
// It returns the model and the first message content found (the user prompt).
func parseLLMBody(raw []byte) (model string, prompt string) {
	if len(raw) == 0 {
		return "", ""
	}
	// The buffer is a raw window of decrypted bytes: it may have an HTTP/2
	// frame header (containing NUL bytes) before the JSON and trailing NUL
	// padding after it. The regexes below match the JSON fields directly and
	// ignore any surrounding binary/NUL bytes, so no trimming is needed.
	if m := llmModelRe.FindSubmatch(raw); m != nil {
		model = string(m[1])
	}
	if c := llmContentRe.FindSubmatch(raw); c != nil {
		prompt = string(c[1])
	}
	return model, prompt
}

// parseLLMUsage extracts token usage from a captured response body window.
// The window is the tail of the response, where the usage object lives, and
// may contain surrounding binary/NUL bytes; extraction is tolerant.
func parseLLMUsage(raw []byte) (promptTokens, completionTokens, totalTokens int64) {
	if len(raw) == 0 {
		return 0, 0, 0
	}
	if m := llmPromptTokRe.FindSubmatch(raw); m != nil {
		promptTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	if m := llmComplTokRe.FindSubmatch(raw); m != nil {
		completionTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	if m := llmTotalTokRe.FindSubmatch(raw); m != nil {
		totalTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	return promptTokens, completionTokens, totalTokens
}

// emitLLMSpan emits a single APM span for one LLM request transaction.
// One span per request — there is intentionally no aggregation. model and
// prompt may be empty if the body was not captured (e.g. the first request on
// a connection) or could not be parsed.
func emitLLMSpan(path string, method Method, statusCode uint16, connKey types.ConnectionKey, latencyNs float64, info llmSpanInfo) {
	// Only emit a span when we resolved a real service AND captured LLM payload
	// data. Without a resolved service (PID→service inference failed) or any
	// model/prompt/response (e.g. first request on a connection, before the body
	// was captured), the span would be noise — the request is still counted in
	// USM's HTTP metrics.
	if info.service == "" {
		return
	}
	if info.model == "" && info.prompt == "" && info.response == "" {
		return
	}
	if !ensureLLMOTracer() {
		return
	}

	end := time.Now()
	start := end.Add(-time.Duration(latencyNs))
	destIP := util.FromLowHigh(connKey.DstIPLow, connKey.DstIPHigh)

	// Emit an LLM Observability span so it lands in the "AI" section, grouped by
	// ml_app = the resolved service (e.g. openai-test).
	span, _ := llmobs.StartLLMSpan(context.Background(), llmoSpanName,
		llmobs.WithMLApp(info.service),
		llmobs.WithModelName(info.model),
		llmobs.WithModelProvider(llmoModelVendor),
		llmobs.WithStartTime(start),
	)

	var input, output []llmobs.LLMMessage
	if info.prompt != "" {
		input = []llmobs.LLMMessage{{Role: "user", Content: info.prompt}}
	}
	if info.response != "" {
		output = []llmobs.LLMMessage{{Role: "assistant", Content: info.response}}
	}

	annotations := []llmobs.AnnotateOption{
		llmobs.WithAnnotatedTags(map[string]string{
			"http.method":              method.String(),
			"http.status_code":         strconv.Itoa(int(statusCode)),
			"http.url":                 path,
			"out.host":                 destIP.String(),
			"network.destination.port": strconv.Itoa(int(connKey.DstPort)),
			"llm.source":               "apm-lite-ebpf",
		}),
	}
	if info.totalTokens > 0 {
		annotations = append(annotations, llmobs.WithAnnotatedMetrics(map[string]float64{
			llmobs.MetricKeyInputTokens:  float64(info.promptTokens),
			llmobs.MetricKeyOutputTokens: float64(info.completionTokens),
			llmobs.MetricKeyTotalTokens:  float64(info.totalTokens),
		}))
	}
	span.AnnotateLLMIO(input, output, annotations...)

	var finishOpts []llmobs.FinishSpanOption
	finishOpts = append(finishOpts, llmobs.WithFinishTime(end))
	span.Finish(finishOpts...)
}

// captureLLMBody marks the connection as LLM traffic (so the eBPF write hook
// captures bodies for subsequent requests) and reads back the most recently
// captured request body for this connection, returning the parsed model and
// prompt. Returns empty strings when the LLMO maps are not wired up or no body
// has been captured yet for this connection.
func (h *StatKeeper) captureLLMBody(connKey types.ConnectionKey, pid uint32) (info llmSpanInfo) {
	if h.llmConnMap == nil || h.llmBodyMap == nil {
		return info
	}

	// Resolve the service name from the client PID in userspace, using the same
	// inference USM uses (process_service_inference).
	info.service = h.resolveLLMService(pid)

	key := newLLMConnKey(connKey)

	// Flag the connection so future writes/reads on it get captured.
	flag := uint8(1)
	if err := h.llmConnMap.Put(&key, &flag); err != nil {
		log.Debugf("LLMO: failed to flag connection: %v", err)
	}

	// Request body -> model + prompt.
	var reqBody llmBody
	if err := h.llmBodyMap.Lookup(&key, &reqBody); err == nil {
		n := reqBody.Len
		if n > llmBodyBufferSize {
			n = llmBodyBufferSize
		}
		info.model, info.prompt = parseLLMBody(reqBody.Data[:n])
	}

	// Response body tail -> token usage.
	if h.llmRespBodyMap != nil {
		var respBody llmBody
		if err := h.llmRespBodyMap.Lookup(&key, &respBody); err == nil {
			n := respBody.Len
			if n > llmBodyBufferSize {
				n = llmBodyBufferSize
			}
			info.promptTokens, info.completionTokens, info.totalTokens = parseLLMUsage(respBody.Data[:n])
		}
	}

	// Response body head -> assistant message content (the AI's answer).
	if h.llmRespHeadMap != nil {
		var respHead llmBody
		if err := h.llmRespHeadMap.Lookup(&key, &respHead); err == nil {
			n := respHead.Len
			if n > llmBodyBufferSize {
				n = llmBodyBufferSize
			}
			// The response head's only "content" field is the assistant message.
			if c := llmContentRe.FindSubmatch(respHead.Data[:n]); c != nil {
				info.response = string(c[1])
			}
		}
	}

	return info
}

// resolveLLMService resolves the service name for a client PID using the same
// userspace inference USM uses (process_service_inference / ServiceExtractor).
// It feeds the extractor the process cmdline read from /proc on demand and
// returns the inferred service name (empty if it can't be resolved).
func (h *StatKeeper) resolveLLMService(pid uint32) string {
	if pid == 0 || h.llmServiceExtractor == nil {
		return ""
	}

	// Populate the extractor's cache from /proc while the process is alive.
	// Short-lived clients may have exited by the time a later transaction is
	// processed, so we still consult the cache (GetServiceContext) below even
	// when the fresh read fails.
	if cmdline := readProcCmdline(pid); len(cmdline) > 0 {
		h.llmServiceExtractor.ExtractSingle(&procutil.Process{
			Pid:     int32(pid),
			Cmdline: cmdline,
		})
	}

	for _, tag := range h.llmServiceExtractor.GetServiceContext(int32(pid)) {
		// Tags look like "process_context:<service>".
		if _, name, ok := strings.Cut(tag, ":"); ok && name != "" {
			return name
		}
	}
	return ""
}

// readProcCmdline reads /proc/<pid>/cmdline and splits it into args.
func readProcCmdline(pid uint32) []string {
	raw, err := os.ReadFile("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/cmdline")
	if err != nil || len(raw) == 0 {
		return nil
	}
	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
