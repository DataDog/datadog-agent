// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
)

// ProxyLifecycleProcessor is an implementation of the invocationlifecycle.InvocationProcessor
// interface called by the Runtime API proxy on every function invocation calls and responses.
// This allows AppSec to run by monitoring the function invocations, and run the security
// rules upon reception of the HTTP request span in the SpanModifier function created by
// the WrapSpanModifier() method.
// A value of this type can be used by a single function invocation at a time.
type ProxyLifecycleProcessor struct {
	// AppSec instance
	appsec Monitorer

	// Parsed invocation event value
	invocationEvent interface{}

	demux aggregator.Demultiplexer
}

// NewProxyLifecycleProcessor returns a new httpsec proxy processor monitored with the
// given Monitorer.
func NewProxyLifecycleProcessor(appsec Monitorer, demux aggregator.Demultiplexer) *ProxyLifecycleProcessor {
	panic("not called")
}

//nolint:revive // TODO(ASM) Fix revive linter
func (lp *ProxyLifecycleProcessor) GetExecutionInfo() *invocationlifecycle.ExecutionStartInfo {
	panic("not called")
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *ProxyLifecycleProcessor) OnInvokeStart(startDetails *invocationlifecycle.InvocationStartDetails) {
	panic("not called")
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *ProxyLifecycleProcessor) OnInvokeEnd(_ *invocationlifecycle.InvocationEndDetails) {
	panic("not called")
}

//nolint:revive // TODO(ASM) Fix revive linter
func (lp *ProxyLifecycleProcessor) spanModifier(lastReqId string, chunk *pb.TraceChunk, s *pb.Span) {
	panic("not called")
}

// multiOrSingle picks the first non-nil map, and returns the content formatted
// as the multi-map.
func multiOrSingle(multi map[string][]string, single map[string]string) map[string][]string {
	panic("not called")
}

//nolint:revive // TODO(ASM) Fix revive linter
type ExecutionContext interface {
	LastRequestID() string
}

// WrapSpanModifier wraps the given SpanModifier function with AppSec monitoring
// and returns it. When non nil, the given modifySpan function is called first,
// before the AppSec monitoring.
// The resulting function will run AppSec when the span's request_id span tag
// matches the one observed at function invocation with OnInvokeStat() through
// the Runtime API proxy.
func (lp *ProxyLifecycleProcessor) WrapSpanModifier(ctx ExecutionContext, modifySpan func(*pb.TraceChunk, *pb.Span)) func(*pb.TraceChunk, *pb.Span) {
	panic("not called")
}

type spanWrapper pb.Span

func (s *spanWrapper) SetMetaTag(tag string, value string) {
	panic("not called")
}

func (s *spanWrapper) SetMetricsTag(tag string, value float64) {
	panic("not called")
}

func (s *spanWrapper) GetMetaTag(tag string) (value string, exists bool) {
	panic("not called")
}

type bytesStringer []byte

func (b bytesStringer) String() string {
	panic("not called")
}
