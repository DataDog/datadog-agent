// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTwoSubs(t *testing.T) {
	state := newEventStreamState()

	outData1, outErr1, err, shouldStart := state.subscribe("listener1")
	assert.Nil(t, err)
	assert.True(t, shouldStart)
	state.running = true

	outData2, outErr2, err, shouldStart := state.subscribe("listener2")
	assert.Nil(t, err)
	assert.False(t, shouldStart)

	ev := &ContainerEvent{}
	state.dispatch(ev)
	received1 := <-outData1
	assert.Equal(t, ev, received1)
	received2 := <-outData2
	assert.Equal(t, ev, received2)

	select {
	case err := <-outErr1:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case err := <-outErr2:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case <-time.After(time.Millisecond):
		break
	}
}

func TestSendTimeout(t *testing.T) {
	state := newEventStreamState()

	_, outErr, err, _ := state.subscribe("listener1")
	assert.Nil(t, err)

	ev := &ContainerEvent{}

	// First sends should be OK
	for i := 0; i < eventSendBuffer; i++ {
		badSubs := state.dispatch(ev)
		assert.Equal(t, 0, len(badSubs))
	}

	select {
	case err := <-outErr:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case <-time.After(time.Millisecond):
		break
	}

	// Next send should timeout
	badSubs := state.dispatch(ev)
	assert.Equal(t, 1, len(badSubs))

	select {
	case err := <-outErr:
		require.NotNil(t, err)
		assert.Equal(t, err, ErrEventTimeout)
	case <-time.After(time.Second):
		assert.FailNow(t, "should have received a timeout error")
	}
}
