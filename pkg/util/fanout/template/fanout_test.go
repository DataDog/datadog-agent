// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package fanout

import (
	"errors"
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
	fanner.Stop()
}

func TestTwoListeners(t *testing.T) {
	fanner := MessageFanout{}
	inData, inErr, err := fanner.Setup(fanout.Config{
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
	recievedMessage1 := <-outData1
	assert.EqualValues(t, sentMessage, recievedMessage1)
	recievedMessage2 := <-outData2
	assert.EqualValues(t, sentMessage, recievedMessage2)

	sentError := errors.New("testerror")
	inErr <- sentError
	recievedError1 := <-outErr1
	assert.EqualValues(t, sentError, recievedError1)
	recievedError2 := <-outErr2
	assert.EqualValues(t, sentError, recievedError2)

	fanner.Stop()
}
