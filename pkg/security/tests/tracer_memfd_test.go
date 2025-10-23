// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// tracerMemfdConsumer is a test consumer that captures TracerMemfdSeal events
type tracerMemfdConsumer struct {
	capturedPid   atomic.Uint32
	capturedFd    atomic.Uint32
	capturedTags  []string
	capturedMutex sync.Mutex
	eventReceived atomic.Bool
}

// ID returns the ID of this consumer
func (c *tracerMemfdConsumer) ID() string {
	return "tracer_memfd_test_consumer"
}

// Start the consumer
func (c *tracerMemfdConsumer) Start() error {
	return nil
}

// Stop the consumer
func (c *tracerMemfdConsumer) Stop() {
}

// EventTypes returns the event types handled by this consumer
func (c *tracerMemfdConsumer) EventTypes() []model.EventType {
	return []model.EventType{
		model.TracerMemfdSealEventType,
	}
}

// ChanSize returns the chan size used by the consumer
func (c *tracerMemfdConsumer) ChanSize() int {
	return 10
}

// HandleEvent handles this event
func (c *tracerMemfdConsumer) HandleEvent(event any) {
	ev, ok := event.(*tracerMemfdEvent)
	if !ok {
		return
	}

	c.capturedPid.Store(ev.pid)
	c.capturedFd.Store(ev.fd)
	c.capturedMutex.Lock()
	c.capturedTags = ev.tracerTags
	c.capturedMutex.Unlock()
	c.eventReceived.Store(true)
}

// tracerMemfdEvent is a minimal copy of the event fields we care about
type tracerMemfdEvent struct {
	pid        uint32
	fd         uint32
	tracerTags []string
}

// Copy returns a copy of the event for this consumer
func (c *tracerMemfdConsumer) Copy(ev *model.Event) any {
	if ev.GetEventType() != model.TracerMemfdSealEventType {
		return nil
	}

	event := &tracerMemfdEvent{
		pid: ev.GetProcessPid(),
		fd:  ev.TracerMemfdSeal.Fd,
	}

	// Copy the TracerTags using the getter
	tracerTags := ev.GetProcessTracerTags()
	if len(tracerTags) > 0 {
		event.tracerTags = make([]string, len(tracerTags))
		copy(event.tracerTags, tracerTags)
	}

	return event
}

func TestTracerMemfd(t *testing.T) {
	SkipIfNotAvailable(t)

	consumer := &tracerMemfdConsumer{}
	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			if err := test.eventMonitor.AddEventConsumerHandler(consumer); err != nil {
				t.Fatalf("failed to add event consumer handler: %v", err)
			}
			test.eventMonitor.RegisterEventConsumer(consumer)
		},
	}))
	require.NoError(t, err)
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	require.NoError(t, err)

	test.RunMultiMode(t, "validate-event-and-tracer-tags", func(t *testing.T, _ wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd) {
		consumer.eventReceived.Store(false)
		consumer.capturedPid.Store(0)
		consumer.capturedFd.Store(0)

		cmdExec := cmd(syscallTester, []string{"tracer-memfd"}, nil)
		_ = cmdExec.Run()

		require.Eventually(t, consumer.eventReceived.Load, 2*time.Second, 200*time.Millisecond, "tracer-memfd event should be received")

		capturedPid := consumer.capturedPid.Load()
		capturedFd := consumer.capturedFd.Load()

		require.NotZero(t, capturedPid, "pid should be set in event")
		require.NotZero(t, capturedFd, "fd should be non-zero")
		require.Greater(t, capturedFd, uint32(2), "fd should be > 2 (stdin/stdout/stderr)")

		// Verify tracer tags from ProcessCacheEntry
		consumer.capturedMutex.Lock()
		tracerTags := consumer.capturedTags
		consumer.capturedMutex.Unlock()

		require.NotEmpty(t, tracerTags, "TracerTags should not be empty")

		// Verify expected tags from the msgp-encoded metadata
		expectedTags := []string{
			"service:test-service",
			"env:test-env",
			"version:1.0.0",
			"custom.tag:value",
		}

		require.ElementsMatch(t, tracerTags, expectedTags, "TracerTags")
	})
}
