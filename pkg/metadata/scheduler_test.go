// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
)

func TestNewScheduler(t *testing.T) {
	firstRun = false
	defer func() { firstRun = true }()

	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)

	assert.Equal(t, fwd, c.srl.Forwarder)
}

func TestStopScheduler(t *testing.T) {
	firstRun = false
	defer func() { firstRun = true }()

	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)
	c.AddCollector("test", time.Duration(60))
	c.AddCollector("test2", time.Duration(60))
	c.Stop()
	assert.Equal(t, context.Canceled, c.context.Err())
}

func mockNewTicker(d time.Duration) *time.Ticker {
	c := make(chan time.Time, 1)
	ticker := time.NewTicker(1000 * time.Hour)
	ticker.C = c
	c <- time.Now() // Ticks as soon as it's created
	return ticker
}

func mockNewTickerNoTick(d time.Duration) *time.Ticker {
	return time.NewTicker(1000 * time.Hour)
}

type MockCollector struct {
	SendCalledC chan bool
}

func (c MockCollector) Send(s *serializer.Serializer) error {
	c.SendCalledC <- true
	return nil
}

func TestCollectorSendLogic(t *testing.T) {
	firstRun = false
	defer func() { firstRun = true }()

	newTicker = mockNewTicker
	defer func() { newTicker = time.NewTicker }()

	mockCollector := MockCollector{
		SendCalledC: make(chan bool, 3),
	}

	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)
	RegisterCollector("testCollector", mockCollector)

	c.AddCollector("testCollector", 1000)

	select {
	case called := <-mockCollector.SendCalledC:
		assert.Equal(t, true, called)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for send to be called")
	}
	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called more than once")
	default:
	}

	newTicker = mockNewTickerNoTick
	c.SendNow("testCollector")

	select {
	case called := <-mockCollector.SendCalledC:
		assert.Equal(t, true, called)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for send to be called")
	}
	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called more than once")
	default:
	}

}
