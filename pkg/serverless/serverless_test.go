// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const mutexLocked = 1

func longInvocationHandler(doneChanel chan bool, daemon *Daemon, arn string, coldstart bool) {
	time.Sleep(50 * time.Millisecond)
	doneChanel <- true
}

func shortInvocationHandler(doneChanel chan bool, daemon *Daemon, arn string, coldstart bool) {
	doneChanel <- true
}

func MutexLocked(m *sync.Mutex) bool {
	state := reflect.ValueOf(m).Elem().FieldByName("state")
	return state.Int()&mutexLocked == mutexLocked
}

func TestInvokeMutexShouldBeLocked(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	d := StartDaemon(cancel)
	d.ReadyWg.Done()
	defer d.Stop(false)

	d.clientLibReady = false
	d.WaitForDaemon()

	d.StartInvocation()

	//deadline = current time + 20 ms
	deadlineMs := (time.Now().UnixNano())/1000000 + 20
	invoke(d, "fakeArn", deadlineMs, 0, true, longInvocationHandler)

	assert.True(t, MutexLocked(&d.flushing))
}

func TestInvokeMutexShouldBeUnLocked(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	d := StartDaemon(cancel)
	d.ReadyWg.Done()
	defer d.Stop(false)

	d.clientLibReady = false
	d.WaitForDaemon()

	d.StartInvocation()

	//deadline = current time + 20 ms
	deadlineMs := (time.Now().UnixNano())/1000000 + 20
	invoke(d, "fakeArn", deadlineMs, 0, true, shortInvocationHandler)

	assert.False(t, MutexLocked(&d.flushing))
}
