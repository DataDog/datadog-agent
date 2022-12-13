// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestMain(m *testing.M) {
	origShutdownDelay := ShutdownDelay
	ShutdownDelay = 0
	defer func() { ShutdownDelay = origShutdownDelay }()
	os.Exit(m.Run())
}

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()

	complete := false
	go func() {
		<-time.After(10 * time.Millisecond)
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
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEnd(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneIfLocalTest(t *testing.T) {
	t.Setenv(localTestEnvVar, "1")
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	client := &http.Client{Timeout: 1 * time.Second}
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/flush", port), nil)
	assert.Nil(err)
	response, err := client.Do(request) //nolint:bodyclose
	if err != nil {
		// retry once in case the daemon wasn't ready to accept requests
		time.Sleep(100 * time.Millisecond)
		response, err = client.Do(request) //nolint:bodyclose
	}
	defer response.Body.Close()
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
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEndAndTimeout(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestRaceTellDaemonRuntimeStartedVersusTellDaemonRuntimeDone(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	go d.TellDaemonRuntimeStarted()
	go d.TellDaemonRuntimeDone()
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
	t.Setenv("DD_API_KEY", "x")
	agent.Start(true, &trace.LoadConfig{Path: "/does-not-exist.yml"}, nil)
	defer agent.Stop()
	d := Daemon{
		TraceAgent: agent,
	}
	assert.True(t, d.setTraceTags(tagsMap))
}

func TestOutOfOrderInvocations(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	assert.NotPanics(t, d.TellDaemonRuntimeDone)
	d.TellDaemonRuntimeStarted()
}
