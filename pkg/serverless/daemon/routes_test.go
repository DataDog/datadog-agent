// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

type mockLifecycleProcessor struct {
	OnInvokeStartCalled bool
	OnInvokeEndCalled   bool
	isError             bool
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
	body := bytes.NewBuffer([]byte(`{"toto": "tutu","Headers": {"x-datadog-trace-id": "2222"}}`))
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
