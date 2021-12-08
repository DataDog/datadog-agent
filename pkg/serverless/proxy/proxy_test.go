// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testProcessorResponseValid struct{}

func (tp *testProcessorResponseValid) OnInvokeStart(startDetails *InvocationStartDetails) {
	if startDetails.StartTime.IsZero() {
		panic("isZero")
	}
	if len(startDetails.InvokeHeaders) != 3 {
		panic("headers")
	}
	if !strings.HasSuffix(startDetails.InvokeEventPayload, "ok") {
		panic("payload")
	}
}

func (tp *testProcessorResponseValid) OnInvokeEnd(endDetails *InvocationEndDetails) {
	if endDetails.IsError != false {
		panic("isError")
	}
	if endDetails.EndTime.IsZero() {
		panic("isZero")
	}
}

type testProcessorResponseError struct{}

func (tp *testProcessorResponseError) OnInvokeStart(startDetails *InvocationStartDetails) {
	if startDetails.StartTime.IsZero() {
		panic("isZero")
	}
	if len(startDetails.InvokeHeaders) != 3 {
		panic("headers")
	}
	if !strings.HasSuffix(startDetails.InvokeEventPayload, "ok") {
		panic("payload")
	}
}

func (tp *testProcessorResponseError) OnInvokeEnd(endDetails *InvocationEndDetails) {
	if endDetails.IsError != true {
		panic("isError")
	}
}

func TestStartTrue(t *testing.T) {
	os.Setenv("DD_EXPERIMENTAL_ENABLE_PROXY", "true")
	defer os.Unsetenv("DD_EXPERIMENTAL_ENABLE_PROXY")
	assert.True(t, Start("127.0.0.1:7000", "127.0.0.1:7001", &testProcessorResponseValid{}))
}

func TestStartFalse(t *testing.T) {
	assert.False(t, Start("127.0.0.1:5000", "127.0.0.1:5001", &testProcessorResponseValid{}))
}

func TestProxyResponseValid(t *testing.T) {
	// fake the runtime API running on 5001
	l, err := net.Listen("tcp", "127.0.0.1:5001")
	assert.Nil(t, err)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	}))
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	defer ts.Close()

	os.Setenv("DD_EXPERIMENTAL_ENABLE_PROXY", "true")
	defer os.Unsetenv("DD_EXPERIMENTAL_ENABLE_PROXY")

	go setup("127.0.0.1:5000", "127.0.0.1:5001", &testProcessorResponseValid{})
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:5000/xxx/next")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	resp, err = http.Post("http://127.0.0.1:5000/xxx/response", "text/plain", strings.NewReader("bla bla bla"))
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestProxyResponseError(t *testing.T) {
	// fake the runtime API running on 6001
	l, err := net.Listen("tcp", "127.0.0.1:6001")
	assert.Nil(t, err)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	}))
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	defer ts.Close()

	os.Setenv("DD_EXPERIMENTAL_ENABLE_PROXY", "true")
	defer os.Unsetenv("DD_EXPERIMENTAL_ENABLE_PROXY")

	go setup("127.0.0.1:6000", "127.0.0.1:6001", &testProcessorResponseError{})
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:6000/xxx/next")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	resp, err = http.Post("http://127.0.0.1:6000/xxx/error", "text/plain", strings.NewReader("bla bla bla"))
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

}
