// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("http://localhost:8124")
	defer d.Stop()

	// WaitForDaemon doesn't block if the client library hasn't
	// registered with the extension's /hello route
	d.clientLibReady = false
	d.WaitForDaemon()

	// WaitForDaemon blocks if the client library has registered with the extension's /hello route
	d.clientLibReady = true

	d.StartInvocation()

	complete := false
	go func() {
		<-time.After(100 * time.Millisecond)
		complete = true
		d.FinishInvocation()
	}()
	d.WaitForDaemon()
	assert.Equal(complete, true, "daemon didn't block until FinishInvocation")
}

func TestWaitUntilReady(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("http://localhost:8124")
	defer d.Stop()

	ready := d.WaitUntilClientReady(50 * time.Millisecond)
	assert.Equal(ready, false, "client was ready")
}

func GetValueSyncOnce(so *sync.Once) uint64 {
	return reflect.ValueOf(so).Elem().FieldByName("done").Uint()
}

func TestFinishInvocationOnceStartOnly(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("http://localhost:8124")
	defer d.Stop()

	d.StartInvocation()
	assert.Equal(uint64(0), GetValueSyncOnce(&d.finishInvocationOnce))
}

func TestFinishInvocationOnceStartAndEnd(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("http://localhost:8124")
	defer d.Stop()

	d.StartInvocation()
	d.FinishInvocation()

	assert.Equal(uint64(1), GetValueSyncOnce(&d.finishInvocationOnce))
}

func TestFinishInvocationOnceStartAndEndAndTimeout(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("http://localhost:8124")
	defer d.Stop()

	d.StartInvocation()
	d.FinishInvocation()
	d.FinishInvocation()

	assert.Equal(uint64(1), GetValueSyncOnce(&d.finishInvocationOnce))
}
