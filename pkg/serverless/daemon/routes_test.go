// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

type mockLifecycleProcessor struct {
	OnInvokeStartCalled bool
	OnInvokeEndCalled   bool
	isError             bool
	lastEndDetails      *invocationlifecycle.InvocationEndDetails
}

func (m *mockLifecycleProcessor) GetExecutionInfo() *invocationlifecycle.ExecutionStartInfo {
	return &invocationlifecycle.ExecutionStartInfo{}
}

func (m *mockLifecycleProcessor) OnInvokeStart(*invocationlifecycle.InvocationStartDetails) {
	m.OnInvokeStartCalled = true
}

func (m *mockLifecycleProcessor) OnInvokeEnd(endDetails *invocationlifecycle.InvocationEndDetails) {
	m.OnInvokeEndCalled = true
	m.isError = endDetails.IsError
	m.lastEndDetails = endDetails
}

func TestStartInvocation(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{"toto": "titi", "tata":true}`))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/start-invocation", port), body)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		assert.Equal(res.StatusCode, 200)
		res.Body.Close()
	}
	assert.True(m.OnInvokeStartCalled)
}

func TestEndInvocation(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{}
	body := bytes.NewBuffer([]byte(`{}`))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/end-invocation", port), body)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		res.Body.Close()
		assert.Equal(res.StatusCode, 200)
	}
	assert.False(m.isError)
	assert.True(m.OnInvokeEndCalled)

	lastRequestID := d.ExecutionContext.GetCurrentState().LastRequestID
	coldStartTags := d.ExecutionContext.GetColdStartTagsForRequestID(lastRequestID)

	assert.Equal(m.lastEndDetails.ColdStart, coldStartTags.IsColdStart)
	assert.Equal(m.lastEndDetails.ProactiveInit, coldStartTags.IsProactiveInit)
	assert.Equal(m.lastEndDetails.Runtime, d.ExecutionContext.GetCurrentState().Runtime)
}

func TestEndInvocationWithError(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{}
	body := bytes.NewBuffer([]byte(`{}`))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/end-invocation", port), body)
	request.Header.Set("x-datadog-invocation-error", "true")
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		res.Body.Close()
		assert.Equal(res.StatusCode, 200)
	}
	assert.True(m.OnInvokeEndCalled)
	assert.True(m.isError)
}

func TestTraceContext(t *testing.T) {
	assert := assert.New(t)

	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()
	d.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:           d.ExtraTags,
		Demux:               nil,
		ProcessTrace:        nil,
		DetectLambdaLibrary: func() bool { return false },
	}
	client := &http.Client{}
	body := bytes.NewBuffer([]byte(`{"toto": "tutu","Headers": {"x-datadog-trace-id": "2222","x-datadog-parent-id":"3333"}}`))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/start-invocation", port), body)
	assert.Nil(err)
	response, err := client.Do(request)
	assert.Nil(err)
	response.Body.Close()
	request, err = http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/trace-context", port), nil)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	assert.Equal("2222", fmt.Sprintf("%v", d.InvocationProcessor.GetExecutionInfo().TraceID))
	if res != nil {
		res.Body.Close()
		assert.Equal(res.Header.Get("x-datadog-trace-id"), fmt.Sprintf("%v", d.InvocationProcessor.GetExecutionInfo().TraceID))
		assert.Equal(res.Header.Get("x-datadog-span-id"), fmt.Sprintf("%v", d.InvocationProcessor.GetExecutionInfo().SpanID))
	}
}

func TestStartEndInvocationSpanParenting(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	var spans []*pb.Span
	processTrace := func(p *api.Payload) {
		for _, c := range p.TracerPayload.Chunks {
			for _, span := range c.Spans {
				spans = append(spans, span)
			}
		}
	}

	d.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ProcessTrace:         processTrace,
		InferredSpansEnabled: true,
		DetectLambdaLibrary:  func() bool { return false },
	}

	client := &http.Client{Timeout: 1 * time.Second}
	startURL := fmt.Sprintf("http://127.0.0.1:%d/lambda/start-invocation", port)
	endURL := fmt.Sprintf("http://127.0.0.1:%d/lambda/end-invocation", port)

	testcases := []struct {
		name        string
		payload     io.Reader
		expSpans    int
		expTraceID  uint64
		expParentID uint64
	}{
		{
			name:        "empty-payload",
			payload:     bytes.NewBuffer([]byte("{}")),
			expSpans:    1,
			expTraceID:  0,
			expParentID: 0,
		},
		{
			name:        "api-gateway",
			payload:     getEventFromFile("api-gateway.json"),
			expSpans:    2,
			expTraceID:  12345,
			expParentID: 67890,
		},
		{
			name:        "sqs",
			payload:     getEventFromFile("sqs.json"),
			expSpans:    2,
			expTraceID:  2684756524522091840,
			expParentID: 7431398482019833808,
		},
		{
			name:        "sqs-batch",
			payload:     getEventFromFile("sqs-batch.json"),
			expSpans:    2,
			expTraceID:  2684756524522091840,
			expParentID: 7431398482019833808,
		},
		{
			name:        "sqs_no_dd_context",
			payload:     getEventFromFile("sqs_no_dd_context.json"),
			expSpans:    2,
			expTraceID:  0,
			expParentID: 0,
		},
		{
			// NOTE: sns trace extraction not implemented yet
			name:        "sns",
			payload:     getEventFromFile("sns.json"),
			expSpans:    2,
			expTraceID:  0,
			expParentID: 0,
		},
		{
			name:        "sns-sqs",
			payload:     getEventFromFile("snssqs.json"),
			expSpans:    3,
			expTraceID:  1728904347387697031,
			expParentID: 353722510835624345,
		},
		// TODO: test inferred spans disabled
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			spans = []*pb.Span{}

			// start-invocation
			startReq, err := http.NewRequest(http.MethodPost, startURL, tc.payload)
			assert.Nil(err)
			startResp, err := client.Do(startReq)
			assert.Nil(err)
			var respHdr http.Header
			if startResp != nil {
				assert.Equal(startResp.StatusCode, 200)
				respHdr = startResp.Header
				startResp.Body.Close()
			}

			// end-invocation
			endReq, err := http.NewRequest(http.MethodPost, endURL, nil)
			assert.Nil(err)
			endResp, err := client.Do(endReq)
			assert.Nil(err)
			if endResp != nil {
				assert.Equal(endResp.StatusCode, 200)
				endResp.Body.Close()
			}

			// sort spans by start time
			slices.SortFunc(spans, func(a, b *pb.Span) int { return int(a.Start - b.Start) })

			// assert parenting of inferred and execution spans
			rootSpan := spans[0]
			parentID := rootSpan.ParentID
			assert.Equal(tc.expSpans, len(spans))
			assert.Equal(tc.expParentID, parentID)
			var tailSpan *pb.Span
			for _, span := range spans {
				tailSpan = span
				assert.Equal(tc.expTraceID, span.TraceID)
				assert.Equal(parentID, span.ParentID)
				parentID = span.SpanID
			}
			assert.Equal("aws.lambda", tailSpan.Name)

			// assert parenting passed to tracer
			if tailSpan.TraceID != 0 {
				assert.Equal(fmt.Sprintf("%d", tailSpan.TraceID),
					respHdr.Get("x-datadog-trace-id"))
			} else {
				assert.Equal("", respHdr.Get("x-datadog-trace-id"))
			}
			if tailSpan.SpanID != 0 {
				assert.Equal(fmt.Sprintf("%d", tailSpan.SpanID),
					respHdr.Get("x-datadog-parent-id"))
			} else {
				assert.Equal("", respHdr.Get("x-datadog-parent-id"))
			}
		})
	}
}

// Helper function for reading test file
func getEventFromFile(filename string) io.Reader {
	event, err := os.ReadFile("../trace/testdata/event_samples/" + filename)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	buf.WriteString("a5a")
	buf.Write(event)
	buf.WriteString("0")
	return &buf
}
