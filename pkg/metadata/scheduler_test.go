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
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type MockCollector struct {
	SendCalledC chan bool
}

//nolint:revive // TODO(ASC) Fix revive linter
func (c MockCollector) Send(ctx context.Context, s serializer.MetricSerializer) error {
	c.SendCalledC <- true
	return nil
}

type MockCollectorWithInit struct {
	InitCalledC chan bool
}

//nolint:revive // TODO(ASC) Fix revive linter
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

//nolint:revive // TODO(ASC) Fix revive linter
func (c mockCollectorWithFirstRun) Send(ctx context.Context, s serializer.MetricSerializer) error {
	c.sendCalledC <- true
	return nil
}

func (c mockCollectorWithFirstRun) FirstRunInterval() time.Duration {
	return 2 * time.Second
}

//nolint:revive // TODO(ASC) Fix revive linter
func mockNewTimer(d time.Duration) *time.Timer {
	c := make(chan time.Time, 1)
	timer := time.NewTimer(10 * time.Hour)
	timer.C = c
	c <- time.Now() // Ticks as soon as it's created
	return timer
}

//nolint:revive // TODO(ASC) Fix revive linter
func mockNewTimerNoTick(d time.Duration) *time.Timer {
	return time.NewTimer(10 * time.Hour)
}

func TestNewScheduler(t *testing.T) {
	deps := buildDeps(t)
	demux := deps.Demultiplexer

	c := NewScheduler(demux)

	assert.Equal(t, demux, c.demux)
}

func TestStopScheduler(t *testing.T) {
	deps := buildDeps(t)
	c := NewScheduler(deps.Demultiplexer)

	mockCollector := MockCollector{}
	RegisterCollector("test", mockCollector)

	err := c.AddCollector("test", 10*time.Hour)
	assert.NoError(t, err)

	c.Stop()

	assert.Equal(t, context.Canceled, c.context.Err())
}

func TestAddCollector(t *testing.T) {
	newTimer = mockNewTimer
	defer func() { newTimer = time.NewTimer }()

	mockCollector := &MockCollector{
		SendCalledC: make(chan bool),
	}
	deps := buildDeps(t)
	c := NewScheduler(deps.Demultiplexer)

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
	mockCollectorWithInit := &MockCollectorWithInit{
		InitCalledC: make(chan bool, 1),
	}

	deps := buildDeps(t)
	c := NewScheduler(deps.Demultiplexer)

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
	mockCollector := &mockCollectorWithFirstRun{
		sendCalledC: make(chan bool, 1),
	}

	deps := buildDeps(t)
	c := NewScheduler(deps.Demultiplexer)

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
	newTimer = mockNewTimerNoTick
	defer func() { newTimer = time.NewTimer }()

	mockCollector := &MockCollector{
		SendCalledC: make(chan bool),
	}

	deps := buildDeps(t)
	demux := deps.Demultiplexer
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

type deps struct {
	fx.In
	Demultiplexer demultiplexer.Component
}

func buildDeps(t *testing.T) deps {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true
	return fxutil.Test[deps](t, defaultforwarder.MockModule(), config.MockModule(), logimpl.MockModule(), demultiplexerimpl.MockModule())
}
