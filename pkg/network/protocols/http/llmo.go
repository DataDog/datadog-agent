// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"

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
	llmoMLApp = "apm-lite-ebpf"

	// Providers we recognize. The provider is inferred from the model name and
	// selects the per-provider body/response/usage parsers.
	providerOpenAI    = "openai"
	providerAnthropic = "anthropic"

	// llmBodyBufferSize must match LLM_BODY_BUFFER_SIZE in the eBPF code
	// (pkg/network/ebpf/c/protocols/tls/llmo.h).
	llmBodyBufferSize = 1024
	// llmReqBodyBufferSize must match LLM_REQ_BUFFER_SIZE in the eBPF code: the
	// larger window used for the request body (the user's prompt).
	llmReqBodyBufferSize = 16384
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

// llmReqParsed is a request body parsed from the request ring buffer: model,
// provider, messages/prompt, and any tool call + result carried in the request
// history (a follow-up call). Queued FIFO per connection until a transaction
// consumes it.
type llmReqParsed struct {
	model        string
	provider     string
	messages     []llmMessage
	prompt       string
	reqToolCalls []llmToolCall
	toolResults  []llmToolResult
	sessionID    string    // app-supplied session/conversation id (see parseSessionID)
	pid          uint32    // client PID that wrote the request (for service resolution)
	arrived      time.Time // when the request event was processed (for latency)
}

// llmReqEvent mirrors the eBPF llm_req_event_t: a connection key plus a captured
// request body, streamed once per request.
type llmReqEvent struct {
	Key      llmConnKey
	StreamID uint32
	Pid      uint32
	Len      uint32
	Pad      uint32
	Data     [llmReqBodyBufferSize]byte
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

	// OpenAI chat messages are {"content":"...","role":"..."} objects, but the
	// field order depends on the serializer: the openai-go SDK emits content
	// before role, while the raw API / other SDKs emit role before content. We
	// try both orders (a given body is internally consistent, so only one
	// matches) so every message — system, user, assistant — is captured in order.
	llmMessageRe          = regexp.MustCompile(`"content"\s*:\s*"([^"]*)"\s*,\s*"role"\s*:\s*"([^"]*)"`)
	llmMessageRoleFirstRe = regexp.MustCompile(`"role"\s*:\s*"([^"]*)"\s*,\s*"content"\s*:\s*"([^"]*)"`)

	// Anthropic request messages carry content as an array of text blocks:
	// "content":[{"text":"...","type":"text"}],"role":"user". Capture the block
	// text followed by the message role.
	llmAnthropicMsgRe = regexp.MustCompile(`"text"\s*:\s*"([^"]*)"[^\]]*\]\s*,\s*"role"\s*:\s*"([^"]*)"`)
	// Anthropic has no "system" message role; the system prompt is a top-level
	// array of text blocks.
	llmAnthropicSystemRe = regexp.MustCompile(`"system"\s*:\s*\[\s*\{\s*"text"\s*:\s*"([^"]*)"`)
	// Anthropic response content and usage: the assistant answer is in a "text"
	// field and token usage uses input_tokens/output_tokens (no total_tokens).
	llmTextRe      = regexp.MustCompile(`"text"\s*:\s*"([^"]*)"`)
	llmInputTokRe  = regexp.MustCompile(`"input_tokens"\s*:\s*(\d+)`)
	llmOutputTokRe = regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)

	// Tool-call extractors (from the response body). The model does not run
	// tools; it emits a structured request to call one, which is on the wire.
	// OpenAI: choices[].message.tool_calls[] = {id,type:"function",function:
	// {name, arguments:"<json-string>"}}. arguments is a JSON-encoded string.
	llmOpenAIToolRe = regexp.MustCompile(`"id"\s*:\s*"([^"]*)"\s*,\s*"type"\s*:\s*"function"\s*,\s*"function"\s*:\s*\{\s*"name"\s*:\s*"([^"]*)"\s*,\s*"arguments"\s*:\s*"((?:\\.|[^"\\])*)"`)
	// Anthropic: content[] = {type:"tool_use",id,name,input:{...}}. input is a
	// JSON object (flat inputs only for this PoC regex).
	llmAnthropicToolRe = regexp.MustCompile(`"type"\s*:\s*"tool_use"\s*,\s*"id"\s*:\s*"([^"]*)"\s*,\s*"name"\s*:\s*"([^"]*)"\s*,\s*"input"\s*:\s*(\{[^{}]*\})`)

	// Tool-result extractors (from the *request* history of a follow-up call):
	// the app runs the tool in-process and feeds the result back. OpenAI uses a
	// {"role":"tool","tool_call_id":..,"content":..} message; Anthropic a
	// tool_result block. These pair a result back to its tool call by id.
	llmOpenAIToolResultRe    = regexp.MustCompile(`"role"\s*:\s*"tool"\s*,\s*"tool_call_id"\s*:\s*"([^"]*)"\s*,\s*"content"\s*:\s*"([^"]*)"`)
	llmAnthropicToolResultRe = regexp.MustCompile(`"type"\s*:\s*"tool_result"\s*,\s*"tool_use_id"\s*:\s*"([^"]*)"\s*,\s*"content"\s*:\s*"([^"]*)"`)

	// OpenAI token usage extractors (from the response body).
	llmPromptTokRe = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	llmComplTokRe  = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	llmTotalTokRe  = regexp.MustCompile(`"total_tokens"\s*:\s*(\d+)`)

	// A response whose finish/stop reason indicates a tool call. This lives in
	// the response tail (alongside usage), which is reliably captured, unlike
	// the head where the tool_calls array can be lost on multi-read responses.
	llmFinishToolRe  = regexp.MustCompile(`"finish_reason"\s*:\s*"tool_calls"`) // OpenAI
	llmStopToolUseRe = regexp.MustCompile(`"stop_reason"\s*:\s*"tool_use"`)     // Anthropic
)

// isToolCallGen reports whether a response tail is a tool-call generation.
func isToolCallGenRegex(raw []byte, provider string) bool {
	if provider == providerAnthropic {
		return llmStopToolUseRe.Match(raw)
	}
	return llmFinishToolRe.Match(raw)
}

// llmMessage is one chat message (role + content) parsed from the request body.
type llmMessage struct {
	role    string
	content string
}

// llmToolCall is a tool the model requested the app to invoke, parsed from the
// response body. The model never executes the tool; it emits this request and
// the app runs it, so only the request (name + arguments) is on the wire.
type llmToolCall struct {
	id        string
	name      string
	arguments string // JSON arguments object
}

// llmToolResult is the result the app fed back for a tool call, parsed from the
// message history of a follow-up request. Paired to its call by id.
type llmToolResult struct {
	id      string
	content string
}

// llmUsage is provider-neutral token usage (input/output/total).
type llmUsage struct {
	input  int64
	output int64
	total  int64
}

// llmSpanInfo holds everything parsed from the captured request/response
// bodies to enrich an LLM span.
type llmSpanInfo struct {
	service  string
	provider string // "openai" | "anthropic" — inferred from the model
	model    string
	// messages are the request's chat messages (system, user, ...) in order.
	// prompt is kept as the last user message for the tag/fallback path.
	messages     []llmMessage
	prompt       string
	response     string
	toolCalls    []llmToolCall // tool calls in the response (this turn's output)
	reqToolCalls []llmToolCall // tool calls in the request history (a prior turn)
	toolResults  []llmToolResult
	inputTokens  int64
	outputTokens int64
	totalTokens  int64
	// firstGenUsage is the token usage of the tool_call generation (turn 1),
	// recovered from the previous-response maps and matched to reqToolCalls by
	// tool_call id, so the workflow's first llm span carries its cost.
	firstGenUsage llmUsage
	// suppressFlat drops the standalone per-request span: the connection is
	// running tool workflows, which are represented by the agent workflow span
	// instead, so a flat llm span here would be a duplicate.
	suppressFlat bool
	// sessionID is an app-supplied session/conversation id parsed from the
	// request (OpenAI "user"/metadata, Anthropic metadata.user_id). When set, it
	// groups multiple requests into one LLM Observability session.
	sessionID string
}

// detectProvider infers the LLM provider from the model name. Anthropic models
// are named "claude-*"; everything else defaults to OpenAI-shaped parsing.
func detectProvider(model string) string {
	if strings.HasPrefix(model, "claude") {
		return providerAnthropic
	}
	return providerOpenAI
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
func parseLLMBodyRegex(raw []byte) (model string, prompt string) {
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

// parseLLMMessages extracts every chat message (role + content) from a captured
// request body window, in order, using the parser for the given provider.
// Extraction is tolerant of surrounding binary/NUL bytes and truncation, same
// as parseLLMBody; a message truncated at the buffer boundary is omitted.
func parseLLMMessagesRegex(raw []byte, provider string) []llmMessage {
	if len(raw) == 0 {
		return nil
	}
	if provider == providerAnthropic {
		return parseAnthropicMessagesRegex(raw)
	}
	return parseOpenAIMessagesRegex(raw)
}

// parseOpenAIMessages extracts messages from an OpenAI chat request body,
// handling both field orders ({"content":..,"role":..} and {"role":..,
// "content":..}). Only one order matches a given body.
func parseOpenAIMessagesRegex(raw []byte) []llmMessage {
	if matches := llmMessageRe.FindAllSubmatch(raw, -1); matches != nil {
		msgs := make([]llmMessage, 0, len(matches))
		for _, m := range matches {
			msgs = append(msgs, llmMessage{content: string(m[1]), role: string(m[2])})
		}
		return msgs
	}
	if matches := llmMessageRoleFirstRe.FindAllSubmatch(raw, -1); matches != nil {
		msgs := make([]llmMessage, 0, len(matches))
		for _, m := range matches {
			msgs = append(msgs, llmMessage{role: string(m[1]), content: string(m[2])})
		}
		return msgs
	}
	return nil
}

// parseAnthropicMessages extracts messages from an Anthropic request body. The
// system prompt is a top-level array (no "system" role); user/assistant
// messages carry their text inside a content-block array. System is returned
// first, then the messages in order, so the resulting list matches OpenAI's
// system-first ordering.
func parseAnthropicMessagesRegex(raw []byte) []llmMessage {
	var msgs []llmMessage
	if m := llmAnthropicSystemRe.FindSubmatch(raw); m != nil {
		msgs = append(msgs, llmMessage{role: "system", content: string(m[1])})
	}
	for _, m := range llmAnthropicMsgRe.FindAllSubmatch(raw, -1) {
		msgs = append(msgs, llmMessage{content: string(m[1]), role: string(m[2])})
	}
	return msgs
}

// parseResponseText extracts the assistant's answer from a captured response
// body window, using the shape for the given provider.
func parseResponseTextRegex(raw []byte, provider string) string {
	if len(raw) == 0 {
		return ""
	}
	re := llmContentRe // OpenAI: choices[].message.content
	if provider == providerAnthropic {
		re = llmTextRe // Anthropic: content[].text
	}
	if m := re.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}

// parseToolCalls extracts the tool calls the model requested from a captured
// response body window, per provider. Returns nil when there are none (a plain
// text answer). OpenAI arguments arrive as a JSON-encoded string and are
// unescaped to raw JSON; Anthropic inputs are already a JSON object.
func parseToolCallsRegex(raw []byte, provider string) []llmToolCall {
	if len(raw) == 0 {
		return nil
	}
	var calls []llmToolCall
	if provider == providerAnthropic {
		for _, m := range llmAnthropicToolRe.FindAllSubmatch(raw, -1) {
			calls = append(calls, llmToolCall{id: string(m[1]), name: string(m[2]), arguments: string(m[3])})
		}
		return calls
	}
	for _, m := range llmOpenAIToolRe.FindAllSubmatch(raw, -1) {
		args := string(m[3])
		// arguments is a JSON string literal (escaped); unescape to raw JSON.
		if unq, err := strconv.Unquote(`"` + args + `"`); err == nil {
			args = unq
		}
		calls = append(calls, llmToolCall{id: string(m[1]), name: string(m[2]), arguments: args})
	}
	return calls
}

// parseToolResults extracts tool results from a captured request body window
// (present on follow-up calls where the app fed a tool's output back to the
// model), per provider. Each result carries the id of the tool call it answers.
func parseToolResultsRegex(raw []byte, provider string) []llmToolResult {
	if len(raw) == 0 {
		return nil
	}
	re := llmOpenAIToolResultRe
	if provider == providerAnthropic {
		re = llmAnthropicToolResultRe
	}
	var results []llmToolResult
	for _, m := range re.FindAllSubmatch(raw, -1) {
		results = append(results, llmToolResult{id: string(m[1]), content: string(m[2])})
	}
	return results
}

// parseLLMUsage extracts token usage from a captured response body window,
// using the field names for the given provider. Returned counts are
// provider-neutral (input, output, total) mapping to the LLM Observability
// token metrics. The window may contain surrounding binary/NUL bytes;
// extraction is tolerant.
func parseLLMUsageRegex(raw []byte, provider string) (inputTokens, outputTokens, totalTokens int64) {
	if len(raw) == 0 {
		return 0, 0, 0
	}
	if provider == providerAnthropic {
		// Anthropic: input_tokens/output_tokens, and no total (derive it).
		if m := llmInputTokRe.FindSubmatch(raw); m != nil {
			inputTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		if m := llmOutputTokRe.FindSubmatch(raw); m != nil {
			outputTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		return inputTokens, outputTokens, inputTokens + outputTokens
	}
	// OpenAI: prompt_tokens/completion_tokens/total_tokens.
	if m := llmPromptTokRe.FindSubmatch(raw); m != nil {
		inputTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	if m := llmComplTokRe.FindSubmatch(raw); m != nil {
		outputTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	if m := llmTotalTokRe.FindSubmatch(raw); m != nil {
		totalTokens, _ = strconv.ParseInt(string(m[1]), 10, 64)
	}
	return inputTokens, outputTokens, totalTokens
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
	// A follow-up request that completed a tool round-trip (assistant tool_call
	// + tool result in its history) is emitted as a workflow with child llm +
	// tool spans instead of a single flat span.
	if len(info.reqToolCalls) > 0 && len(info.toolResults) > 0 {
		emitWorkflowSpan(path, latencyNs, info)
		return
	}
	// On a tool-workflow connection, the conversation is shown as the agent
	// workflow; skip the standalone flat span so it isn't duplicated.
	if info.suppressFlat {
		return
	}
	if info.model == "" && info.prompt == "" && info.response == "" && len(info.toolCalls) == 0 {
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
	llmOpts := []llmobs.StartSpanOption{
		llmobs.WithMLApp(info.service),
		llmobs.WithModelName(info.model),
		llmobs.WithModelProvider(info.provider),
		llmobs.WithStartTime(start),
	}
	if info.sessionID != "" {
		llmOpts = append(llmOpts, llmobs.WithSessionID(info.sessionID))
	}
	span, _ := llmobs.StartLLMSpan(context.Background(), llmoSpanName, llmOpts...)

	var input, output []llmobs.LLMMessage
	// Prefer the full parsed message list (system, user, ...) so every request
	// message shows as a distinct input on the span; fall back to the single
	// user prompt when the message list wasn't parsed.
	if len(info.messages) > 0 {
		input = make([]llmobs.LLMMessage, 0, len(info.messages))
		for _, m := range info.messages {
			input = append(input, llmobs.LLMMessage{Role: m.role, Content: m.content})
		}
	} else if info.prompt != "" {
		input = []llmobs.LLMMessage{{Role: "user", Content: info.prompt}}
	}
	// The assistant's turn is a tool call, a text answer, or both. Emit an
	// output message whenever we have either.
	if info.response != "" || len(info.toolCalls) > 0 {
		out := llmobs.LLMMessage{Role: "assistant", Content: info.response}
		for _, tc := range info.toolCalls {
			out.ToolCalls = append(out.ToolCalls, llmobs.ToolCall{
				Name:      tc.name,
				Arguments: json.RawMessage(tc.arguments),
				ToolID:    tc.id,
				Type:      "function",
			})
		}
		output = []llmobs.LLMMessage{out}
	}

	tags := map[string]string{
		"http.method":              method.String(),
		"http.status_code":         strconv.Itoa(int(statusCode)),
		"http.url":                 path,
		"out.host":                 destIP.String(),
		"network.destination.port": strconv.Itoa(int(connKey.DstPort)),
		"llm.source":               "apm-lite-ebpf",
	}
	if len(info.toolCalls) > 0 {
		names := make([]string, 0, len(info.toolCalls))
		for _, tc := range info.toolCalls {
			names = append(names, tc.name)
		}
		tags["llm.tool_calls"] = strings.Join(names, ",")
	}
	annotations := []llmobs.AnnotateOption{llmobs.WithAnnotatedTags(tags)}
	if info.totalTokens > 0 {
		annotations = append(annotations, llmobs.WithAnnotatedMetrics(map[string]float64{
			llmobs.MetricKeyInputTokens:  float64(info.inputTokens),
			llmobs.MetricKeyOutputTokens: float64(info.outputTokens),
			llmobs.MetricKeyTotalTokens:  float64(info.totalTokens),
		}))
	}
	span.AnnotateLLMIO(input, output, annotations...)

	var finishOpts []llmobs.FinishSpanOption
	finishOpts = append(finishOpts, llmobs.WithFinishTime(end))
	span.Finish(finishOpts...)
}

// emitWorkflowSpan reconstructs a tool-using turn as an LLM Observability
// agent trace, matching how the SDK would present model-driven tool use: a root
// "agent" span with three children in sequence — the llm call that requested
// the tool, the tool call, and the llm call that produced the final answer.
// Each llm span is one model API call (as the SDK models it). The spans are
// linked because we own the tracer context here and thread it ourselves.
//
// Two things can't match the SDK, for data (not structural) reasons: the first
// llm span carries no token usage — turn 1's usage lives in turn 1's response,
// which our single-slot-per-connection capture has overwritten by the time the
// follow-up is processed — and the tool span's timing is approximate (the tool
// runs in-process, so we split the follow-up request's latency into thirds).
func emitWorkflowSpan(path string, latencyNs float64, info llmSpanInfo) {
	if !ensureLLMOTracer() {
		return
	}
	end := time.Now()
	start := end.Add(-time.Duration(latencyNs))
	t1 := start.Add(time.Duration(latencyNs / 3))
	t2 := start.Add(time.Duration(2 * latencyNs / 3))
	tags := map[string]string{"http.url": path, "llm.source": "apm-lite-ebpf"}

	agentOpts := []llmobs.StartSpanOption{
		llmobs.WithMLApp(info.service),
		llmobs.WithStartTime(start),
	}
	if info.sessionID != "" {
		agentOpts = append(agentOpts, llmobs.WithSessionID(info.sessionID))
	}
	agent, actx := llmobs.StartAgentSpan(context.Background(), "llm.conversation", agentOpts...)

	// Base input messages (system + user), shared by both generations.
	var baseInput []llmobs.LLMMessage
	for _, m := range info.messages {
		baseInput = append(baseInput, llmobs.LLMMessage{Role: m.role, Content: m.content})
	}
	toolCallMsg := llmobs.LLMMessage{Role: "assistant"}
	for _, tc := range info.reqToolCalls {
		toolCallMsg.ToolCalls = append(toolCallMsg.ToolCalls, llmobs.ToolCall{
			Name:      tc.name,
			Arguments: json.RawMessage(tc.arguments),
			ToolID:    tc.id,
			Type:      "function",
		})
	}

	// Child 1 (llm): the model API call that decided to call the tool(s).
	llm1, _ := llmobs.StartLLMSpan(actx, llmoSpanName,
		llmobs.WithMLApp(info.service),
		llmobs.WithModelName(info.model),
		llmobs.WithModelProvider(info.provider),
		llmobs.WithStartTime(start),
	)
	llm1Annotations := []llmobs.AnnotateOption{llmobs.WithAnnotatedTags(tags)}
	if info.firstGenUsage.total > 0 {
		llm1Annotations = append(llm1Annotations, llmobs.WithAnnotatedMetrics(map[string]float64{
			llmobs.MetricKeyInputTokens:  float64(info.firstGenUsage.input),
			llmobs.MetricKeyOutputTokens: float64(info.firstGenUsage.output),
			llmobs.MetricKeyTotalTokens:  float64(info.firstGenUsage.total),
		}))
	}
	llm1.AnnotateLLMIO(baseInput, []llmobs.LLMMessage{toolCallMsg}, llm1Annotations...)
	llm1.Finish(llmobs.WithFinishTime(t1))

	// Child 2 (tool): one span per call, output = the matching result (by id).
	results := make(map[string]string, len(info.toolResults))
	for _, r := range info.toolResults {
		results[r.id] = r.content
	}
	for _, tc := range info.reqToolCalls {
		toolSpan, _ := llmobs.StartToolSpan(actx, tc.name,
			llmobs.WithMLApp(info.service),
			llmobs.WithStartTime(t1),
		)
		toolSpan.AnnotateTextIO(tc.arguments, results[tc.id])
		toolSpan.Finish(llmobs.WithFinishTime(t2))
	}

	// Child 3 (llm): the model API call that produced the final answer. Its
	// input is the full conversation so far; its output + token usage come from
	// the captured follow-up response.
	llm2, _ := llmobs.StartLLMSpan(actx, llmoSpanName,
		llmobs.WithMLApp(info.service),
		llmobs.WithModelName(info.model),
		llmobs.WithModelProvider(info.provider),
		llmobs.WithStartTime(t2),
	)
	finalInput := append([]llmobs.LLMMessage{}, baseInput...)
	finalInput = append(finalInput, toolCallMsg)
	for _, r := range info.toolResults {
		finalInput = append(finalInput, llmobs.LLMMessage{Role: "tool", Content: r.content})
	}
	annotations := []llmobs.AnnotateOption{llmobs.WithAnnotatedTags(tags)}
	if info.totalTokens > 0 {
		annotations = append(annotations, llmobs.WithAnnotatedMetrics(map[string]float64{
			llmobs.MetricKeyInputTokens:  float64(info.inputTokens),
			llmobs.MetricKeyOutputTokens: float64(info.outputTokens),
			llmobs.MetricKeyTotalTokens:  float64(info.totalTokens),
		}))
	}
	llm2.AnnotateLLMIO(finalInput, []llmobs.LLMMessage{{Role: "assistant", Content: info.response}}, annotations...)
	llm2.Finish(llmobs.WithFinishTime(end))

	// The agent's own I/O: the user prompt in, the model's final answer out.
	agent.AnnotateTextIO(info.prompt, info.response)
	agent.Finish(llmobs.WithFinishTime(end))
}

// flagLLMConn marks a connection as LLM traffic so the eBPF write/read hooks
// begin capturing its decrypted request/response bodies. Called from add() the
// first time an LLM-looking path is seen on the connection (subsequent
// requests/responses on it are then captured and paired by the consumers).
func (h *StatKeeper) flagLLMConn(connKey types.ConnectionKey) {
	if h.llmConnMap == nil {
		return
	}
	key := newLLMConnKey(connKey)
	flag := uint8(1)
	if err := h.llmConnMap.Put(&key, &flag); err != nil {
		log.Debugf("LLMO: failed to flag connection: %v", err)
	}
}

// toConnectionKey rebuilds a types.ConnectionKey from the LLMO key (they share
// field layout; the LLMO key just adds trailing padding for the eBPF map).
func (k llmConnKey) toConnectionKey() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: k.SrcIPHigh,
		SrcIPLow:  k.SrcIPLow,
		DstIPHigh: k.DstIPHigh,
		DstIPLow:  k.DstIPLow,
		SrcPort:   k.SrcPort,
		DstPort:   k.DstPort,
	}
}

// llmPathForProvider synthesizes the request path for the span resource. The
// capture is the DATA frame (JSON body), not the HEADERS frame, so the real
// :path isn't available in the consumer; these APIs use a fixed path per
// provider anyway.
func llmPathForProvider(provider string) string {
	switch provider {
	case providerAnthropic:
		return "/v1/messages"
	default:
		return "/v1/chat/completions"
	}
}

// pairAndEmit builds and emits an LLM span from a request and the reassembled
// response body captured on the same (conn, stream). Running from the response
// consumer, it pairs each response with its exact request — correct regardless
// of transaction ordering or HTTP/2 multiplexing.
func (h *StatKeeper) pairAndEmit(key llmStreamKey, req llmReqParsed, respBuf []byte) {
	var info llmSpanInfo
	// Resolve the service from the client PID captured with the request, using
	// the same inference USM uses (process_service_inference).
	info.service = h.resolveLLMService(req.pid)
	info.model = req.model
	info.provider = req.provider
	info.messages = req.messages
	info.prompt = req.prompt
	info.reqToolCalls = req.reqToolCalls
	info.toolResults = req.toolResults
	info.sessionID = req.sessionID

	provider := info.provider
	if provider == "" {
		provider = responseProvider(respBuf)
	}
	info.inputTokens, info.outputTokens, info.totalTokens = parseLLMUsage(respBuf, provider)
	info.response = parseResponseText(respBuf, provider)
	info.toolCalls = parseToolCalls(respBuf, provider)

	// A tool-call generation is turn 1 of a workflow: cache its usage by
	// connection (the workflow's two turns share the connection, sequentially)
	// so the follow-up's first llm span carries turn-1 cost, and suppress this
	// turn's flat span — the conversation is shown as the agent workflow instead.
	if isToolCallGen(respBuf, provider) {
		h.cacheGenUsage(key.conn, llmUsage{input: info.inputTokens, output: info.outputTokens, total: info.totalTokens})
		info.suppressFlat = true
	}
	// A follow-up request carries the prior turn's tool call + result: recover
	// turn 1's usage for the workflow's first llm span.
	if len(info.reqToolCalls) > 0 && len(info.toolResults) > 0 {
		if u, ok := h.lookupGenUsage(key.conn); ok {
			info.firstGenUsage = u
		}
	}

	latencyNs := float64(time.Since(req.arrived).Nanoseconds())
	if latencyNs <= 0 {
		latencyNs = 1
	}
	emit := h.llmEmit
	if emit == nil {
		emit = emitLLMSpan
	}
	emit(llmPathForProvider(provider), MethodPost, 200, key.conn.toConnectionKey(), latencyNs, info)
}

// llmRespReasmCap bounds a reassembled response buffer (protects memory against
// a response that never completes, e.g. a streamed one).
const llmRespReasmCap = 64 * 1024

// llmRespReasm accumulates a response's read events until it is complete (token
// usage seen, which sits at the very end of the response JSON). One per
// connection, touched only by the ring-buffer consumer goroutine.
type llmRespReasm struct {
	buf      []byte
	complete bool
}

// llmRespEvent mirrors the eBPF llm_resp_event_t: a connection key plus a large
// window from the start of the response (the full assistant answer, plus usage
// for responses that fit), streamed once per response.
type llmRespEvent struct {
	Key       llmConnKey
	StreamID  uint32
	EndStream uint32
	Len       uint32
	Pad       uint32
	Data      [llmReqBodyBufferSize]byte
}

// StartLLMORequestConsumer consumes streamed request-body events, parses each,
// and queues it FIFO per connection for captureLLMBody to drain. Streaming (vs
// a single map slot) means a connection that fires several requests in quick
// succession delivers every body, in order, instead of overwriting.
func (h *StatKeeper) StartLLMORequestConsumer(m *ebpf.Map) error {
	reader, err := ringbuf.NewReader(m)
	if err != nil {
		return err
	}
	h.llmReqReader = reader
	go func() {
		for {
			rec, err := reader.Read()
			if err != nil {
				return // reader closed on shutdown
			}
			h.processLLMRequestEvent(rec.RawSample)
		}
	}()
	return nil
}

// processLLMRequestEvent parses one streamed request body and queues it for the
// connection.
func (h *StatKeeper) processLLMRequestEvent(sample []byte) {
	var ev llmReqEvent
	if err := binary.Read(bytes.NewReader(sample), binary.LittleEndian, &ev); err != nil {
		return
	}
	n := ev.Len
	if n > llmReqBodyBufferSize {
		n = llmReqBodyBufferSize
	}
	raw := ev.Data[:n]

	var req llmReqParsed
	req.model, req.prompt = parseLLMBody(raw)
	req.provider = detectProvider(req.model)
	req.messages = parseLLMMessages(raw, req.provider)
	// Prefer the last user message as the prompt (a system message makes the
	// first "content" the system prompt, not the user's).
	for i := len(req.messages) - 1; i >= 0; i-- {
		if req.messages[i].role == "user" {
			req.prompt = req.messages[i].content
			break
		}
	}
	// A follow-up request carries a prior turn's tool call + result in its
	// history; parse them to reconstruct a workflow (llm + tool spans).
	req.reqToolCalls = parseToolCalls(raw, req.provider)
	req.toolResults = parseToolResults(raw, req.provider)
	req.sessionID = parseSessionID(raw, req.provider)
	req.pid = ev.Pid
	req.arrived = time.Now()

	h.storeReq(llmStreamKey{conn: ev.Key, stream: ev.StreamID}, req)
}

// StartLLMOResponseConsumer is exported so the HTTP/2 protocol can start it.
func (h *StatKeeper) StartLLMOResponseConsumer(m *ebpf.Map) error {
	reader, err := ringbuf.NewReader(m)
	if err != nil {
		return err
	}
	h.llmRespReader = reader
	go func() {
		for {
			rec, err := reader.Read()
			if err != nil {
				return // reader closed on shutdown
			}
			h.processLLMResponseEvent(rec.RawSample)
		}
	}()
	return nil
}

// processLLMResponseEvent reassembles a response, keyed by (conn, stream), and
// when the response is complete pairs it with its exact request (stored under
// the same key by the request consumer) and emits the span. Running on a single
// goroutine, the reassembly map needs no lock.
func (h *StatKeeper) processLLMResponseEvent(sample []byte) {
	var ev llmRespEvent
	if err := binary.Read(bytes.NewReader(sample), binary.LittleEndian, &ev); err != nil {
		return
	}
	n := ev.Len
	if n > llmReqBodyBufferSize {
		n = llmReqBodyBufferSize
	}
	chunk := ev.Data[:n]

	key := llmStreamKey{conn: ev.Key, stream: ev.StreamID}

	// A response larger than one read arrives across several read events (the
	// server streams it in multiple TLS records). Reassemble per stream by
	// appending each read until the response is complete.
	//
	// NOTE: when the server splits the body across multiple HTTP/2 DATA frames,
	// the interleaved 9-byte frame headers make the concatenation invalid JSON,
	// so parsing falls back to regex and the answer can be truncated. Cleanly
	// stitching multi-frame bodies needs USM's HTTP/2 frame reassembly (the
	// naive tap here starts mid-stream after warm-up, so it can't frame-align).
	if h.llmRespReasm == nil {
		h.llmRespReasm = make(map[llmStreamKey]*llmRespReasm)
	}
	// Bound the reassembly map against responses that never complete.
	if len(h.llmRespReasm) >= llmStreamMapCap {
		for k := range h.llmRespReasm {
			delete(h.llmRespReasm, k)
			break
		}
	}
	r := h.llmRespReasm[key]
	if r == nil || r.complete {
		r = &llmRespReasm{}
		h.llmRespReasm[key] = r
	}
	if room := llmRespReasmCap - len(r.buf); room > 0 {
		if len(chunk) > room {
			chunk = chunk[:room]
		}
		r.buf = append(r.buf, chunk...)
	}

	// The response is complete when token usage is present (it sits at the very
	// end of the response JSON, so seeing it means the whole body — content
	// included — has been accumulated). END_STREAM is an early-finalize/drop
	// defense: if the stream terminated we stop accumulating even without usage,
	// so a response that carries none (an error, a streamed body) can't wedge the
	// buffer open and grow memory. Best-effort — seeing END_STREAM depends on a
	// read starting at the terminating frame.
	provider := responseProvider(r.buf)
	_, _, tot := parseLLMUsage(r.buf, provider)
	if tot == 0 && ev.EndStream == 0 {
		return
	}
	r.complete = true
	delete(h.llmRespReasm, key)

	// Pair with the request captured on this exact stream and emit. Without a
	// stored request (warm-up, or the request event was lost) there's no prompt
	// or model to build a span from, so drop it.
	if req, ok := h.takeReq(key); ok {
		h.pairAndEmit(key, req, r.buf)
	}
}

// responseProvider infers the provider from markers in a response tail.
func responseProvider(tail []byte) string {
	if bytes.Contains(tail, []byte("stop_reason")) || bytes.Contains(tail, []byte("input_tokens")) {
		return providerAnthropic
	}
	return providerOpenAI
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
