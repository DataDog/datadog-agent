// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type MockCollector struct {
	SendCalledC chan bool
}

func (c MockCollector) Send(ctx context.Context, s serializer.MetricSerializer) error {
	c.SendCalledC <- true
	return nil
}

type MockCollectorWithInit struct {
	InitCalledC chan bool
}

func (c MockCollectorWithInit) Send(ctx context.Context, s serializer.MetricSerializer) error {
	return nil
}

func (c MockCollectorWithInit) Init() error {
	c.InitCalledC <- true
	return nil
}

type mockCollectorWithFirstRun struct {
	sendCalledC chan bool
}

func (c mockCollectorWithFirstRun) Send(ctx context.Context, s serializer.MetricSerializer) error {
	c.sendCalledC <- true
	return nil
}

func (c mockCollectorWithFirstRun) FirstRunInterval() time.Duration {
	return 2 * time.Second
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

	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true
	forwarder := fxutil.Test[defaultforwarder.Component](t, defaultforwarder.MockModule, config.MockModule)
	demux := aggregator.InitAndStartAgentDemultiplexer(forwarder, opts, "hostname")
	c := NewScheduler(demux)

	assert.Equal(t, demux, c.demux)
}

func TestStopScheduler(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	demux := buildDemultiplexer()
	c := NewScheduler(demux)

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

	demux := buildDemultiplexer()
	c := NewScheduler(demux)

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

	demux := buildDemultiplexer()
	c := NewScheduler(demux)

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

func TestAddCollectorWithFirstRun(t *testing.T) {
	enableFirstRunCollection = false
	defer func() { enableFirstRunCollection = true }()

	mockCollector := &mockCollectorWithFirstRun{
		sendCalledC: make(chan bool, 1),
	}

	demux := buildDemultiplexer()
	c := NewScheduler(demux)

	RegisterCollector("testCollectorWithFirstRun", mockCollector)

	c.AddCollector("testCollectorWithFirstRun", 10*time.Hour)

	select {
	case <-mockCollector.sendCalledC:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for Send to be called")
	}

	select {
	case <-mockCollector.sendCalledC:
		assert.Fail(t, "Send was called twice")
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

	demux := buildDemultiplexer()
	defer demux.Stop(false)
	c := NewScheduler(demux)

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

func buildDemultiplexer() aggregator.Demultiplexer {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true
	forwarder := forwarder.NewDefaultForwarder(pkgconfig.Datadog, forwarder.NewOptions(pkgconfig.Datadog, nil))
	demux := aggregator.InitAndStartAgentDemultiplexer(forwarder, opts, "hostname")
	return demux
}
