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
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func TestHello(t *testing.T) {
	assert := assert.New(t)

	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()
	d.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:           d.ExtraTags,
		Demux:               nil,
		ProcessTrace:        nil,
		DetectLambdaLibrary: d.IsLambdaLibraryDetected,
	}
	client := &http.Client{}
	body := bytes.NewBuffer([]byte(`{}`))
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/hello", port), body)
	assert.Nil(err)
	assert.False(d.IsLambdaLibraryDetected())
	response, err := client.Do(request)
	assert.Nil(err)
	response.Body.Close()
	assert.True(d.IsLambdaLibraryDetected())
}

func TestStartEndInvocationSpanParenting(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	var spans []*pb.Span
	var priorities []int32
	processTrace := func(p *api.Payload) {
		for _, c := range p.TracerPayload.Chunks {
			priorities = append(priorities, c.Priority)
			spans = append(spans, c.Spans...)
		}
	}

	processor := &invocationlifecycle.LifecycleProcessor{
		ProcessTrace:        processTrace,
		DetectLambdaLibrary: func() bool { return false },
	}
	d.InvocationProcessor = processor

	client := &http.Client{Timeout: 1 * time.Second}
	startURL := fmt.Sprintf("http://127.0.0.1:%d/lambda/start-invocation", port)
	endURL := fmt.Sprintf("http://127.0.0.1:%d/lambda/end-invocation", port)

	testcases := []struct {
		name        string
		payload     string
		expInfSpans int
		expTraceID  uint64
		expParentID uint64
		expPriority int32
	}{
		{
			name:        "empty-payload",
			payload:     "{}",
			expInfSpans: 0,
			expTraceID:  0,
			expParentID: 0,
			expPriority: -128,
		},
		{
			name:        "api-gateway",
			payload:     getEventFromFile("api-gateway.json"),
			expInfSpans: 1,
			expTraceID:  12345,
			expParentID: 67890,
			expPriority: 2,
		},
		{
			name:        "sqs",
			payload:     getEventFromFile("sqs.json"),
			expInfSpans: 1,
			expTraceID:  2684756524522091840,
			expParentID: 7431398482019833808,
			expPriority: 1,
		},
		{
			name:        "sqs-batch",
			payload:     getEventFromFile("sqs-batch.json"),
			expInfSpans: 1,
			expTraceID:  2684756524522091840,
			expParentID: 7431398482019833808,
			expPriority: 1,
		},
		{
			name:        "sqs_no_dd_context",
			payload:     getEventFromFile("sqs_no_dd_context.json"),
			expInfSpans: 1,
			expTraceID:  0,
			expParentID: 0,
			expPriority: -128,
		},
		{
			name:        "sqs-aws-header",
			payload:     getEventFromFile("sqs-aws-header.json"),
			expInfSpans: 1,
			expTraceID:  12297829382473034410,
			expParentID: 13527612320720337851,
			expPriority: 1,
		},
		{
			name:        "sns",
			payload:     getEventFromFile("sns.json"),
			expInfSpans: 1,
			expTraceID:  4948377316357291421,
			expParentID: 6746998015037429512,
			expPriority: 1,
		},
		{
			name:        "sns-sqs",
			payload:     getEventFromFile("snssqs.json"),
			expInfSpans: 2,
			expTraceID:  1728904347387697031,
			expParentID: 353722510835624345,
			expPriority: 1,
		},
	}

	for _, tc := range testcases {
		for _, infEnabled := range []int{0, 1} {
			t.Run(tc.name+fmt.Sprintf("-%v", infEnabled), func(t *testing.T) {
				assert := assert.New(t)
				spans = []*pb.Span{}
				priorities = []int32{}
				processor.InferredSpansEnabled = infEnabled == 1

				// start-invocation
				startReq, err := http.NewRequest(http.MethodPost, startURL, strings.NewReader(tc.payload))
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
				assert.Equal(1+tc.expInfSpans*infEnabled, len(spans))
				assert.Equal(tc.expParentID, parentID)
				var tailSpan *pb.Span
				for _, span := range spans {
					tailSpan = span
					assert.Equal(tc.expTraceID, span.TraceID)
					assert.Equal(parentID, span.ParentID)
					parentID = span.SpanID
				}
				assert.Equal("aws.lambda", tailSpan.Name)

				// assert sampling priorities
				for _, priority := range priorities {
					assert.Equal(tc.expPriority, priority)
				}

				// assert parenting passed to tracer
				if tailSpan.TraceID != 0 {
					assert.Equal(fmt.Sprintf("%d", tailSpan.TraceID),
						respHdr.Get("x-datadog-trace-id"))
					assert.Equal(fmt.Sprintf("%d", tc.expPriority), respHdr.Get("x-datadog-sampling-priority"))
				} else {
					assert.Equal("", respHdr.Get("x-datadog-trace-id"))
					assert.Equal("", respHdr.Get("x-datadog-sampling-priority"))
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
}

func TestStartEndInvocationIsExecutionSpanIncomplete(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprintf("127.0.0.1:%d", port))
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{}
	body := bytes.NewBuffer([]byte(`{"key": "value"}`))
	startReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/start-invocation", port), body)
	assert.Nil(err)
	startResp, err := client.Do(startReq)
	assert.Nil(err)
	startResp.Body.Close()
	assert.True(m.OnInvokeStartCalled)
	assert.True(d.IsExecutionSpanIncomplete())

	body = bytes.NewBuffer([]byte(`{}`))
	endReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/end-invocation", port), body)
	assert.Nil(err)
	endResp, err := client.Do(endReq)
	assert.Nil(err)
	endResp.Body.Close()
	assert.True(m.OnInvokeEndCalled)
	assert.False(d.IsExecutionSpanIncomplete())
}

// Helper function for reading test file
func getEventFromFile(filename string) string {
	event, err := os.ReadFile("../trace/testdata/event_samples/" + filename)
	if err != nil {
		panic(err)
	}
	return "a5a" + string(event) + "0"
}

func BenchmarkStartEndInvocation(b *testing.B) {
	// Set the logger up, so that it does not buffer all entries forever (some of these are BIG as they include the
	// JSON payload). We're not interested in any output here, so we send it all to `io.Discard`.
	l, err := seelog.LoggerFromWriterWithMinLevel(io.Discard, seelog.ErrorLvl)
	assert.Nil(b, err)
	log.SetupLogger(l, "error")

	// relative to location of this test file
	payloadFiles, err := os.ReadDir("../trace/testdata/event_samples")
	if err != nil {
		b.Fatal(err)
	}
	endBody := `{"hello":"world"}`
	for _, file := range payloadFiles {
		startBody := getEventFromFile(file.Name())
		b.Run("event="+file.Name(), func(b *testing.B) {
			startReq := httptest.NewRequest("GET", "/lambda/start-invocation", nil)
			endReq := httptest.NewRequest("GET", "/lambda/end-invocation", nil)
			rr := httptest.NewRecorder()

			d := startAgents()
			defer d.Stop()
			start := &StartInvocation{d}
			end := &EndInvocation{d}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// reset request bodies
				startReq.Body = io.NopCloser(strings.NewReader(startBody))
				endReq.Body = io.NopCloser(strings.NewReader(endBody))
				b.StartTimer()

				start.ServeHTTP(rr, startReq)
				end.ServeHTTP(rr, endReq)
			}
			b.StopTimer()
		})
	}
}

func startAgents() *Daemon {
	d := StartDaemon(fmt.Sprint("127.0.0.1:", testutil.FreeTCPPort(nil)))

	ta := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:         true,
		LoadConfig:      &trace.LoadConfig{Path: "/some/path/datadog.yml"},
		ColdStartSpanID: 123,
	})
	d.SetTraceAgent(ta)

	ma := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               nooptagger.NewTaggerClient(),
	}
	ma.Start(FlushTimeout, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	d.SetStatsdServer(ma)

	d.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:            d.ExtraTags,
		Demux:                d.MetricAgent.Demux,
		ProcessTrace:         d.TraceAgent.Process,
		DetectLambdaLibrary:  func() bool { return false },
		InferredSpansEnabled: true,
	}
	return d
}
