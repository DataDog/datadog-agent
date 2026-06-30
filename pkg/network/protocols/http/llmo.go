// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/network/types"
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

	// llmBodyBufferSize must match LLM_BODY_BUFFER_SIZE in the eBPF code
	// (pkg/network/ebpf/c/protocols/tls/llmo.h).
	llmBodyBufferSize = 256
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
)

// ensureLLMOTracer lazily starts the dd-trace-go tracer pointed at the local
// trace-agent. It is started on the first LLM request observed.
func ensureLLMOTracer() bool {
	llmoTracerOnce.Do(func() {
		if err := tracer.Start(
			tracer.WithService(llmoServiceName),
			tracer.WithAgentAddr(llmoAgentAddr),
			tracer.WithLogStartup(false),
		); err != nil {
			log.Warnf("LLMO: failed to start tracer: %v", err)
			return
		}
		llmoTracerReady = true
		log.Infof("LLMO: tracer started; emitting one span per LLM request to %s", llmoAgentAddr)
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

// emitLLMSpan emits a single APM span for one LLM request transaction.
// One span per request — there is intentionally no aggregation. model and
// prompt may be empty if the body was not captured (e.g. the first request on
// a connection) or could not be parsed.
func emitLLMSpan(path string, method Method, statusCode uint16, connKey types.ConnectionKey, latencyNs float64, model, prompt string) {
	if !ensureLLMOTracer() {
		return
	}

	end := time.Now()
	start := end.Add(-time.Duration(latencyNs))

	destIP := util.FromLowHigh(connKey.DstIPLow, connKey.DstIPHigh)

	span := tracer.StartSpan(
		llmoSpanName,
		tracer.ServiceName(llmoServiceName),
		tracer.ResourceName(path),
		tracer.SpanType("http"),
		tracer.StartTime(start),
	)
	span.SetTag("http.method", method.String())
	span.SetTag("http.status_code", statusCode)
	span.SetTag("http.url", path)
	span.SetTag("out.host", destIP.String())
	span.SetTag("network.destination.port", connKey.DstPort)
	span.SetTag("llm.source", "usm-ebpf")
	if model != "" {
		span.SetTag("llm.request.model", model)
	}
	if prompt != "" {
		span.SetTag("llm.request.prompt", prompt)
	}
	if statusCode >= 400 {
		span.SetTag("error", true)
	}
	span.Finish(tracer.FinishTime(end))
}

// captureLLMBody marks the connection as LLM traffic (so the eBPF write hook
// captures bodies for subsequent requests) and reads back the most recently
// captured request body for this connection, returning the parsed model and
// prompt. Returns empty strings when the LLMO maps are not wired up or no body
// has been captured yet for this connection.
func (h *StatKeeper) captureLLMBody(connKey types.ConnectionKey) (model, prompt string) {
	if h.llmConnMap == nil || h.llmBodyMap == nil {
		return "", ""
	}
	key := newLLMConnKey(connKey)

	// Flag the connection so future writes on it get their body captured.
	flag := uint8(1)
	if err := h.llmConnMap.Put(&key, &flag); err != nil {
		log.Debugf("LLMO: failed to flag connection: %v", err)
	}

	var body llmBody
	if err := h.llmBodyMap.Lookup(&key, &body); err != nil {
		// No body captured yet for this connection (e.g. first request).
		return "", ""
	}
	n := body.Len
	if n > llmBodyBufferSize {
		n = llmBodyBufferSize
	}
	return parseLLMBody(body.Data[:n])
}
