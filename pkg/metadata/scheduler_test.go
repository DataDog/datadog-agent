// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
)

type MockCollector struct {
	SendCalledC chan bool
}

func (c MockCollector) Send(s *serializer.Serializer) error {
	c.SendCalledC <- true
	return nil
}

type MockCollectorWithInit struct {
	InitCalledC chan bool
}

func (c MockCollectorWithInit) Send(s *serializer.Serializer) error {
	return nil
}

func (c MockCollectorWithInit) Init() error {
	c.InitCalledC <- true
	return nil
}

func mockNewTimer(d time.Duration) *time.Timer {
	c := make(chan time.Time, 1)
	timer := time.NewTimer(10 * time.Hour)
	timer.C = c
	c <- time.Now() // Ticks as soon as it's created
	return timer
}

func mockNewTimerNoTick(d time.Duration) *time.Timer {
	return time.NewTimer(10 * time.Hour)
}

func TestNewScheduler(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	fwd := forwarder.NewDefaultForwarder(forwarder.NewOptions(nil))
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)

	assert.Equal(t, fwd, c.srl.Forwarder)
}

func TestStopScheduler(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	fwd := forwarder.NewDefaultForwarder(forwarder.NewOptions(nil))
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)

	mockCollector := MockCollector{}
	RegisterCollector("test", mockCollector)

	err := c.AddCollector("test", 10*time.Hour)
	assert.NoError(t, err)

	c.Stop()

	assert.Equal(t, context.Canceled, c.context.Err())
}

func TestAddCollector(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	newTimer = mockNewTimer
	defer func() { newTimer = time.NewTimer }()

	mockCollector := &MockCollector{
		SendCalledC: make(chan bool),
	}

	fwd := forwarder.NewDefaultForwarder(forwarder.NewOptions(nil))
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)
	RegisterCollector("testCollector", mockCollector)

	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called too early")
	default:
	}

	c.AddCollector("testCollector", 10*time.Hour)

	select {
	case <-mockCollector.SendCalledC:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for send to be called")
	}

	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called twice")
	default:
	}
}

func TestAddCollectorWithInit(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	mockCollectorWithInit := &MockCollectorWithInit{
		InitCalledC: make(chan bool, 1),
	}

	fwd := forwarder.NewDefaultForwarder(forwarder.NewOptions(nil))
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)
	RegisterCollector("testCollectorWithInit", mockCollectorWithInit)

	select {
	case <-mockCollectorWithInit.InitCalledC:
		assert.Fail(t, "Init was called too early")
	default:
	}

	c.AddCollector("testCollectorWithInit", 10*time.Hour)

	select {
	case <-mockCollectorWithInit.InitCalledC:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for Init to be called")
	}

	select {
	case <-mockCollectorWithInit.InitCalledC:
		assert.Fail(t, "Init was called twice")
	default:
	}
}

func TestTriggerAndResetCollectorTimer(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	newTimer = mockNewTimerNoTick
	defer func() { newTimer = time.NewTimer }()

	mockCollector := &MockCollector{
		SendCalledC: make(chan bool),
	}

	fwd := forwarder.NewDefaultForwarder(forwarder.NewOptions(nil))
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s)
	RegisterCollector("testCollector", mockCollector)

	c.AddCollector("testCollector", 10*time.Hour)

	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called too early")
	default:
	}

	c.TriggerAndResetCollectorTimer("testCollector", 0)

	select {
	case <-mockCollector.SendCalledC:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for send to be called")
	}

	select {
	case <-mockCollector.SendCalledC:
		assert.Fail(t, "Send was called twice")
	default:
	}

}
