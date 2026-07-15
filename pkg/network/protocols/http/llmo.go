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
func isToolCallGen(raw []byte, provider string) bool {
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

// parseLLMMessages extracts every chat message (role + content) from a captured
// request body window, in order, using the parser for the given provider.
// Extraction is tolerant of surrounding binary/NUL bytes and truncation, same
// as parseLLMBody; a message truncated at the buffer boundary is omitted.
func parseLLMMessages(raw []byte, provider string) []llmMessage {
	if len(raw) == 0 {
		return nil
	}
	if provider == providerAnthropic {
		return parseAnthropicMessages(raw)
	}
	return parseOpenAIMessages(raw)
}

// parseOpenAIMessages extracts messages from an OpenAI chat request body,
// handling both field orders ({"content":..,"role":..} and {"role":..,
// "content":..}). Only one order matches a given body.
func parseOpenAIMessages(raw []byte) []llmMessage {
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
func parseAnthropicMessages(raw []byte) []llmMessage {
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
func parseResponseText(raw []byte, provider string) string {
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
func parseToolCalls(raw []byte, provider string) []llmToolCall {
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
func parseToolResults(raw []byte, provider string) []llmToolResult {
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
func parseLLMUsage(raw []byte, provider string) (inputTokens, outputTokens, totalTokens int64) {
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
		emitWorkflowSpan(path, connKey, latencyNs, info)
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
	span, _ := llmobs.StartLLMSpan(context.Background(), llmoSpanName,
		llmobs.WithMLApp(info.service),
		llmobs.WithModelName(info.model),
		llmobs.WithModelProvider(info.provider),
		llmobs.WithStartTime(start),
	)

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
func emitWorkflowSpan(path string, connKey types.ConnectionKey, latencyNs float64, info llmSpanInfo) {
	if !ensureLLMOTracer() {
		return
	}
	end := time.Now()
	start := end.Add(-time.Duration(latencyNs))
	t1 := start.Add(time.Duration(latencyNs / 3))
	t2 := start.Add(time.Duration(2 * latencyNs / 3))
	tags := map[string]string{"http.url": path, "llm.source": "apm-lite-ebpf"}

	agent, actx := llmobs.StartAgentSpan(context.Background(), "llm.conversation",
		llmobs.WithMLApp(info.service),
		llmobs.WithStartTime(start),
	)

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
		info.provider = detectProvider(info.model)
		info.messages = parseLLMMessages(reqBody.Data[:n], info.provider)
		// Prefer the last user message as the prompt (with a system message
		// present, the first "content" is the system prompt, not the user's).
		for i := len(info.messages) - 1; i >= 0; i-- {
			if info.messages[i].role == "user" {
				info.prompt = info.messages[i].content
				break
			}
		}
		// A follow-up request carries a prior turn's tool call + result in its
		// history; parse them to reconstruct a workflow (llm + tool spans).
		info.reqToolCalls = parseToolCalls(reqBody.Data[:n], info.provider)
		info.toolResults = parseToolResults(reqBody.Data[:n], info.provider)
	}

	// Response body tail -> token usage (parsed per provider).
	if data, ok := h.lookupBody(h.llmRespBodyMap, &key); ok {
		info.inputTokens, info.outputTokens, info.totalTokens = parseLLMUsage(data, info.provider)
	}

	// Response body head -> assistant message content + tool calls.
	if data, ok := h.lookupBody(h.llmRespHeadMap, &key); ok {
		info.response = parseResponseText(data, info.provider)
		info.toolCalls = parseToolCalls(data, info.provider)
	}
	// Prefer the streamed, per-connection cached answer over the head map: the
	// head is a single slot overwritten by later responses, so at poll-batched
	// processing time it may hold a different (e.g. tool_call) response with no
	// text, leaving the span output empty.
	if c, ok := h.lookupRespContent(key); ok && c != "" {
		info.response = c
	}

	// A connection with a cached tool-call generation is running tool workflows
	// (the consumer cached it as the response streamed in). Recover the
	// first-gen usage for the workflow's first llm span, and mark this
	// transaction to suppress its standalone flat span — the conversation is
	// shown as the agent workflow, so a flat llm span would be a duplicate.
	if u, ok := h.lookupGenUsage(key); ok {
		info.suppressFlat = true
		if len(info.reqToolCalls) > 0 {
			info.firstGenUsage = u
		}
	}

	return info
}

// lookupBody reads a captured body from an LLMO map and returns its valid
// prefix. ok is false when the map is unset or has no entry for the key.
func (h *StatKeeper) lookupBody(m *ebpf.Map, key *llmConnKey) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	var body llmBody
	if err := m.Lookup(key, &body); err != nil {
		return nil, false
	}
	n := body.Len
	if n > llmBodyBufferSize {
		n = llmBodyBufferSize
	}
	return body.Data[:n], true
}

// llmRespEvent mirrors the eBPF llm_resp_event_t: a connection key plus a
// captured response tail, streamed once per response.
type llmRespEvent struct {
	Key  llmConnKey
	Len  uint32
	Pad  uint32
	Data [llmBodyBufferSize]byte
}

// startLLMOResponseConsumer consumes response-tail events from the ring buffer
// and caches, per connection, the token usage of tool-call generations
// (detected via finish_reason/stop_reason). Because it runs continuously, it
// observes every turn's response in order — so a tool-call generation's usage
// is cached before its follow-up is processed, unlike the poll-batched map
// reads that only ever see the latest response.
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

// processLLMResponseEvent parses one streamed response-tail event and caches,
// per connection: the token usage of a tool-call generation (for the follow-up
// workflow's first llm span), and the latest assistant answer text (a reliable
// source for the span output, since the per-connection head map is churned by
// later responses before poll-batched processing reads it).
func (h *StatKeeper) processLLMResponseEvent(sample []byte) {
	var ev llmRespEvent
	if err := binary.Read(bytes.NewReader(sample), binary.LittleEndian, &ev); err != nil {
		return
	}
	n := ev.Len
	if n > llmBodyBufferSize {
		n = llmBodyBufferSize
	}
	tail := ev.Data[:n]
	provider := responseProvider(tail)

	if isToolCallGen(tail, provider) {
		in, out, tot := parseLLMUsage(tail, provider)
		h.cacheGenUsage(ev.Key, llmUsage{input: in, output: out, total: tot})
	}
	if c := parseResponseText(tail, provider); c != "" {
		h.cacheRespContent(ev.Key, c)
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
