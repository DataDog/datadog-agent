// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package fanout

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/fanout"
)

func TestSetupAndStop(t *testing.T) {
	fanner := MessageFanout{}
	fanner.Setup(fanout.Config{
		OutputBufferSize: 4,
		WriteTimeout:     time.Second,
		Name:             "test",
	})
	fanner.StopOnEOF()
}

func TestTwoListeners(t *testing.T) {
	fanner := MessageFanout{}
	inData, err := fanner.Setup(fanout.Config{
		OutputBufferSize: 4,
		WriteTimeout:     time.Second,
		Name:             "test",
	})
	assert.Nil(t, err)

	outData1, outErr1, err := fanner.Suscribe("listener1")
	assert.Nil(t, err)
	outData2, outErr2, err := fanner.Suscribe("listener2")
	assert.Nil(t, err)

	sentMessage := Message("testmessage")
	inData <- sentMessage
	receivedMessage1 := <-outData1
	assert.EqualValues(t, sentMessage, receivedMessage1)
	receivedMessage2 := <-outData2
	assert.EqualValues(t, sentMessage, receivedMessage2)

	select {
	case err := <-outErr1:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case err := <-outErr2:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case <-time.After(time.Millisecond):
		break
	}

	fanner.StopOnEOF()
}

func TestDataWriteTimeout(t *testing.T) {
	fanner := MessageFanout{}
	inData, err := fanner.Setup(fanout.Config{
		OutputBufferSize: 1,
		WriteTimeout:     time.Nanosecond,
		Name:             "test timeout",
	})
	assert.Nil(t, err)

	_, outErr, err := fanner.Suscribe("listener")
	assert.Nil(t, err)

	sentMessage := Message("testmessage")

	// First send should be OK
	inData <- sentMessage
	select {
	case err := <-outErr:
		assert.FailNow(t, "should not have received an error, received %s", err)
	case <-time.After(time.Millisecond):
		break
	}

	// Second send should timeout
	select {
	case inData <- sentMessage:
		break
	case <-time.After(time.Millisecond):
		assert.FailNow(t, "timeout on sending info")
	}

	select {
	case err := <-outErr:
		assert.Equal(t, err, fanout.ErrTimeout)
	case <-time.After(time.Second):
		assert.FailNow(t, "should have received a timeout error")
	}

	fanner.StopOnEOF()
}

func TestUnsuscribe(t *testing.T) {
	fanner := MessageFanout{}
	_, err := fanner.Setup(fanout.Config{
		OutputBufferSize: 1,
		WriteTimeout:     time.Nanosecond,
		Name:             "test timeout",
	})
	assert.Nil(t, err)

	_, outErr1, err := fanner.Suscribe("listener1")
	assert.Nil(t, err)
	_, outErr2, err := fanner.Suscribe("listener2")
	assert.Nil(t, err)

	last, err := fanner.Unsuscribe("listener1")
	assert.False(t, last)
	assert.Nil(t, err)

	select {
	case err = <-outErr1:
		assert.NotNil(t, err)
		assert.Equal(t, err, io.EOF)
	case <-time.After(time.Second):
		assert.FailNow(t, "should have received an EOF")
	}

	last, err = fanner.Unsuscribe("listener2")
	assert.True(t, last)
	assert.Nil(t, err)

	select {
	case err = <-outErr2:
		assert.Equal(t, err, io.EOF)
	case <-time.After(time.Second):
		assert.FailNow(t, "should have received an EOF")
	}

}
