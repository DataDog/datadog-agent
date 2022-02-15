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

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/stretchr/testify/assert"
)

type mockLifecycleProcessor struct {
	OnInvokeStartCalled bool
	OnInvokeEndCalled   bool
	isError             bool
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
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{"toto": "titi", "tata":true}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/start-invocation", body)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		assert.Equal(res.StatusCode, 200)
	}
	assert.True(m.OnInvokeStartCalled)
}

func TestEndInvocation(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/end-invocation", body)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		assert.Equal(res.StatusCode, 200)
	}
	assert.False(m.isError)
	assert.True(m.OnInvokeEndCalled)
}

func TestEndInvocationWithError(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/end-invocation", body)
	request.Header.Set("x-datadog-invocation-error", "true")
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	if res != nil {
		assert.Equal(res.StatusCode, 200)
	}
	assert.True(m.OnInvokeEndCalled)
	assert.True(m.isError)
}

func TestTraceContext(t *testing.T) {
	assert := assert.New(t)

	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	d.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:           d.ExtraTags,
		MetricChannel:       nil,
		ProcessTrace:        nil,
		DetectLambdaLibrary: func() bool { return false },
	}
	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{"toto": "tutu","Headers": {"x-datadog-trace-id": "2222"}}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/start-invocation", body)
	assert.Nil(err)
	_, err = client.Do(request)
	assert.Nil(err)
	request, err = http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/trace-context", nil)
	assert.Nil(err)
	res, err := client.Do(request)
	assert.Nil(err)
	assert.Equal("2222", fmt.Sprintf("%v", invocationlifecycle.TraceID()))
	if res != nil {
		assert.Equal(res.Header.Get("x-datadog-trace-id"), fmt.Sprintf("%v", invocationlifecycle.TraceID()))
		assert.Equal(res.Header.Get("x-datadog-span-id"), fmt.Sprintf("%v", invocationlifecycle.SpanID()))
	}
}
