// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/stretchr/testify/assert"
)

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	d.TellDaemonRuntimeStarted()

	complete := false
	go func() {
		<-time.After(100 * time.Millisecond)
		complete = true
		d.TellDaemonRuntimeDone()
	}()
	d.WaitForDaemon()
	assert.Equal(complete, true, "daemon didn't block until TellDaemonRuntimeDone")
}

func GetValueSyncOnce(so *sync.Once) uint64 {
	return reflect.ValueOf(so).Elem().FieldByName("done").Uint()
}

func TestTellDaemonRuntimeDoneOnceStartOnly(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEnd(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneIfLocalTest(t *testing.T) {
	os.Setenv(localTestEnvVar, "1")
	defer os.Unsetenv(localTestEnvVar)
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8200")
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	client := &http.Client{Timeout: 1 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8200/lambda/flush", nil)
	assert.Nil(err)
	_, err = client.Do(request)
	assert.Nil(err)
	select {
	case <-wrapWait(d.RuntimeWg):
		// all good
	case <-time.NewTimer(500 * time.Millisecond).C:
		t.Fail()
	}
}

func wrapWait(wg *sync.WaitGroup) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		wg.Wait()
		out <- struct{}{}
	}()
	return out
}

func TestTellDaemonRuntimeNotDoneIf(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEndAndTimeout(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestRaceTellDaemonRuntimeStartedVersusTellDaemonRuntimeDone(t *testing.T) {
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()

	d.TellDaemonRuntimeStarted()

	go func() {
		for i := 0; i < 1000; i++ {
			go d.TellDaemonRuntimeStarted()
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			go d.TellDaemonRuntimeDone()
		}
	}()

	time.Sleep(2 * time.Second)
}

func TestSetTraceTagNoop(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: nil,
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagNoopTraceGetNil(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: &trace.ServerlessTraceAgent{},
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagOk(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	var agent = &trace.ServerlessTraceAgent{}
	os.Setenv("DD_API_KEY", "x")
	defer os.Unsetenv("DD_API_KEY")
	agent.Start(true, &trace.LoadConfig{Path: "/does-not-exist.yml"})
	defer agent.Stop()
	d := Daemon{
		TraceAgent: agent,
	}
	assert.True(t, d.setTraceTags(tagsMap))
}

func TestSetExecutionContextUppercase(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	d.SetExecutionContext(testArn, testRequestID)
	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", d.ExecutionContext.ARN)
	assert.Equal(testRequestID, d.ExecutionContext.LastRequestID)
	assert.Equal(true, d.ExecutionContext.Coldstart)
	assert.Equal(testRequestID, d.ExecutionContext.ColdstartRequestID)
}

func TestSetExecutionContextNoColdstart(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	d.ExecutionContext.ColdstartRequestID = "coldstart-request-id"
	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	d.SetExecutionContext(testArn, testRequestID)
	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", d.ExecutionContext.ARN)
	assert.Equal(testRequestID, d.ExecutionContext.LastRequestID)
	assert.Equal(false, d.ExecutionContext.Coldstart)
}
