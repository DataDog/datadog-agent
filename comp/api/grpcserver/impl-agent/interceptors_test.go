// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	googleGrpc "google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
)

// fakeServerStream is a stub ServerStream that records SendMsg calls and lets the test
// inject a return value for SendMsg.
type fakeServerStream struct {
	googleGrpc.ServerStream
	sent    []any
	sendErr error
}

func (f *fakeServerStream) SendMsg(m any) error {
	f.sent = append(f.sent, m)
	return f.sendErr
}

// interceptorHarness wires up a fresh telemetry mock and the two interceptors under
// test, and exposes a small DSL (sendStream / callUnary / count / bigMsg) so individual
// tests don't have to repeat the gRPC interceptor invocation boilerplate.
type interceptorHarness struct {
	t         *testing.T
	tel       telemetry.Mock
	threshold int
	unary     googleGrpc.UnaryServerInterceptor
	stream    googleGrpc.StreamServerInterceptor
}

func newInterceptorHarness(t *testing.T, threshold int) *interceptorHarness {
	t.Helper()
	tel := telemetrymock.New(t)
	unary, stream := newOversizedMessageInterceptors(threshold, tel)
	require.NotNil(t, unary)
	require.NotNil(t, stream)

	return &interceptorHarness{
		t: t, tel: tel, threshold: threshold, unary: unary, stream: stream,
	}
}

// sendStream invokes the stream interceptor with a handler that sends each msg through
// the wrapped ServerStream, against the supplied fake. Returns the interceptor's error
// so callers can assert on propagation.
func (h *interceptorHarness) sendStream(fake *fakeServerStream, method string, msgs ...any) error {
	h.t.Helper()
	handler := func(_ any, ss googleGrpc.ServerStream) error {
		for _, m := range msgs {
			if err := ss.SendMsg(m); err != nil {
				return err
			}
		}
		return nil
	}
	return h.stream(nil, fake, &googleGrpc.StreamServerInfo{FullMethod: method}, handler)
}

// callUnary invokes the unary interceptor with a handler that returns the supplied
// (resp, err). Returns the interceptor's (response, error) so callers can assert
// pass-through.
func (h *interceptorHarness) callUnary(method string, resp any, err error) (any, error) {
	h.t.Helper()
	handler := func(_ context.Context, _ any) (any, error) { return resp, err }
	return h.unary(context.Background(), nil, &googleGrpc.UnaryServerInfo{FullMethod: method}, handler)
}

// count returns the current value of the oversized counter for the given method tag,
// or 0 if it hasn't been observed yet.
func (h *interceptorHarness) count(method string) float64 {
	h.t.Helper()
	metrics, err := h.tel.GetCountMetric("agent_ipc", "grpc_oversized_messages")
	if err != nil {
		return 0
	}
	for _, m := range metrics {
		if m.Tags()["method"] == method {
			return m.Value()
		}
	}
	return 0
}

// bigMsg returns a proto.Message whose serialized size is guaranteed to exceed the
// harness's threshold (2× threshold worth of payload bytes, plus protobuf framing).
func (h *interceptorHarness) bigMsg() proto.Message {
	return wrapperspb.Bytes(make([]byte, h.threshold*2))
}

const testMethod = "/datadog.api.v1.AgentSecure/StreamConfigEvents"

func TestOversizedMessageInterceptors_DisabledWhenThresholdNonPositive(t *testing.T) {
	tel := telemetrymock.New(t)
	for _, threshold := range []int{0, -1} {
		unary, stream := newOversizedMessageInterceptors(threshold, tel)
		assert.Nilf(t, unary, "unary interceptor should be nil for threshold=%d", threshold)
		assert.Nilf(t, stream, "stream interceptor should be nil for threshold=%d", threshold)
	}
}

func TestOversizedMessageStreamInterceptor_CountsAndForwardsWhenOverThreshold(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	fake := &fakeServerStream{}
	// One oversized + one small message: counter increments exactly once, both messages
	// are still forwarded to the underlying stream (the warning is non-fatal).
	require.NoError(t, h.sendStream(fake, testMethod, h.bigMsg(), wrapperspb.Bool(true)))
	require.Len(t, fake.sent, 2)

	assert.Equal(t, float64(1), h.count(testMethod))
}

func TestOversizedMessageStreamInterceptor_DoesNotCountUnderThreshold(t *testing.T) {
	h := newInterceptorHarness(t, 1024)

	fake := &fakeServerStream{}
	require.NoError(t, h.sendStream(fake, testMethod,
		wrapperspb.String("a"), wrapperspb.String("b"), wrapperspb.String("c"),
	))
	require.Len(t, fake.sent, 3)

	assert.Equal(t, float64(0), h.count(testMethod))
}

func TestOversizedMessageStreamInterceptor_PropagatesSendErrors(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	sentinelErr := errors.New("downstream send failure")
	fake := &fakeServerStream{sendErr: sentinelErr}

	err := h.sendStream(fake, testMethod, h.bigMsg())
	assert.ErrorIs(t, err, sentinelErr)
}

func TestOversizedMessageStreamInterceptor_SeparateCounterPerMethod(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	require.NoError(t, h.sendStream(&fakeServerStream{}, "/svc/A", h.bigMsg(), h.bigMsg()))
	require.NoError(t, h.sendStream(&fakeServerStream{}, "/svc/B", h.bigMsg(), h.bigMsg(), h.bigMsg()))

	assert.Equal(t, float64(2), h.count("/svc/A"))
	assert.Equal(t, float64(3), h.count("/svc/B"))
}

func TestOversizedMessageStreamInterceptor_IgnoresNonProtoMessages(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	// A non-proto value whose Go-side length far exceeds threshold: the interceptor
	// only inspects values that implement proto.Message, so this must not increment
	// the counter (and must not panic in proto.Size).
	bigNonProto := make([]byte, h.threshold*4)
	require.NoError(t, h.sendStream(&fakeServerStream{}, testMethod, bigNonProto))

	assert.Equal(t, float64(0), h.count(testMethod))
}

func TestOversizedMessageUnaryInterceptor_CountsLargeResponse(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	resp := h.bigMsg()
	got, err := h.callUnary(testMethod, resp, nil)
	require.NoError(t, err)
	require.Equal(t, resp, got, "interceptor must pass the handler's response through unchanged")

	assert.Equal(t, float64(1), h.count(testMethod))
}

func TestOversizedMessageUnaryInterceptor_SkipsCounterOnHandlerError(t *testing.T) {
	h := newInterceptorHarness(t, 100)

	sentinelErr := errors.New("handler failed")
	resp := h.bigMsg() // would be over threshold if checked
	got, err := h.callUnary(testMethod, resp, sentinelErr)
	assert.ErrorIs(t, err, sentinelErr)
	assert.Equal(t, resp, got)

	// When the handler returns an error gRPC discards the response, so the size
	// check is skipped to avoid noisy alerts on failed RPCs.
	assert.Equal(t, float64(0), h.count(testMethod))
}

func TestOversizedMessageUnaryInterceptor_DoesNotCountSmallResponses(t *testing.T) {
	h := newInterceptorHarness(t, 1024)

	_, err := h.callUnary(testMethod, wrapperspb.Bool(true), nil)
	require.NoError(t, err)

	assert.Equal(t, float64(0), h.count(testMethod))
}
