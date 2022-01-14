// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/stretchr/testify/assert"
)

type mockLifecycleProcessor struct {
	OnInvokeStartCalled bool
	OnInvokeEndCalled   bool
}

func (m *mockLifecycleProcessor) OnInvokeStart(*invocationlifecycle.InvocationStartDetails) {
	m.OnInvokeStartCalled = true
}

func (m *mockLifecycleProcessor) OnInvokeEnd(*invocationlifecycle.InvocationEndDetails) {
	m.OnInvokeEndCalled = true
}

func TestStartInvocation(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{"start_time": "2019-10-12T07:20:50.52Z", "headers": {"header-1": ["value-1"]}, "payload": "payload-string"}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/start-invocation", body)
	assert.Nil(err)
	_, err = client.Do(request)
	assert.Nil(err)
	assert.True(m.OnInvokeStartCalled)
}

func TestEndInvocation(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	m := &mockLifecycleProcessor{}
	d.InvocationProcessor = m

	client := &http.Client{Timeout: 1 * time.Second}
	body := bytes.NewBuffer([]byte(`{"end_time": "2019-10-12T07:20:50.52Z", "is_error": false}`))
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/end-invocation", body)
	assert.Nil(err)
	_, err = client.Do(request)
	assert.Nil(err)
	assert.True(m.OnInvokeEndCalled)
}

func TestTraceContext(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	client := &http.Client{Timeout: 1 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/trace-context", nil)
	assert.Nil(err)
	response, err := client.Do(request)
	assert.Nil(err)
	assert.NotEmpty(response.Header.Get("x-datadog-trace-id"))
	assert.NotEmpty(response.Header.Get("x-datadog-span-id"))
}
