// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/json"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// tracerMemfdConsumer is a test consumer that captures TracerMemfdSeal events
type tracerMemfdConsumer struct {
	capturedPid            atomic.Uint32
	capturedFd             atomic.Uint32
	capturedMetadata       tracermetadata.TracerMetadata
	capturedSerializedJSON []byte
	capturedMutex          sync.Mutex
	eventReceived          atomic.Bool
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
	c.capturedMetadata = ev.tracerMetadata
	c.capturedSerializedJSON = ev.serializedJSON
	c.capturedMutex.Unlock()
	c.eventReceived.Store(true)
}

// tracerMemfdEvent is a minimal copy of the event fields we care about
type tracerMemfdEvent struct {
	pid            uint32
	fd             uint32
	tracerMetadata tracermetadata.TracerMetadata
	serializedJSON []byte
}

// Copy returns a copy of the event for this consumer
func (c *tracerMemfdConsumer) Copy(ev *model.Event) any {
	if ev.GetEventType() != model.TracerMemfdSealEventType {
		return nil
	}

	event := &tracerMemfdEvent{
		pid:            ev.GetProcessPid(),
		fd:             ev.TracerMemfdSeal.Fd,
		tracerMetadata: ev.GetProcessTracerMetadata(),
	}

	// Serialize the event to JSON for validation
	scrubber, err := utils.NewScrubber(nil, nil)
	if err == nil {
		event.serializedJSON, _ = serializers.MarshalEvent(ev, nil, scrubber)
	}

	return event
}

func TestTracerMemfd(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "TracerMemfd test not supported on RHEL7", func(kv *kernel.Version) bool {
		// Test fails on RHEL7 for unknown reasons, skip it for now
		return kv.IsRH7Kernel()
	})

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

		// Verify tracer metadata from ProcessCacheEntry
		consumer.capturedMutex.Lock()
		tmeta := consumer.capturedMetadata
		consumer.capturedMutex.Unlock()

		require.NotEqual(t, tracermetadata.TracerMetadata{}, tmeta, "TracerMetadata should not be empty")

		assert.Equal(t, "test-service", tmeta.ServiceName, "ServiceName mismatch")
		assert.Equal(t, "test-env", tmeta.ServiceEnv, "ServiceEnv mismatch")
		assert.Equal(t, "1.0.0", tmeta.ServiceVersion, "ServiceVersion mismatch")
		assert.Contains(t, tmeta.ProcessTags, "custom.tag:value", "ProcessTags should contain custom.tag")
	})

	test.RunMultiMode(t, "validate-threadlocal-attribute-keys", func(t *testing.T, _ wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd) {
		consumer.eventReceived.Store(false)
		consumer.capturedPid.Store(0)
		consumer.capturedFd.Store(0)

		cmdExec := cmd(syscallTester, []string{"tracer-memfd-with-keys"}, nil)
		_ = cmdExec.Run()

		require.Eventually(t, consumer.eventReceived.Load, 2*time.Second, 200*time.Millisecond, "tracer-memfd event should be received")

		consumer.capturedMutex.Lock()
		tmeta := consumer.capturedMetadata
		consumer.capturedMutex.Unlock()

		require.NotEmpty(t, tmeta.ServiceName, "ServiceName should not be empty")
		assert.Equal(t, "test-service", tmeta.ServiceName, "ServiceName mismatch")
		assert.Equal(t, "cpp", tmeta.TracerLanguage, "TracerLanguage mismatch")

		// Verify threadlocal_attribute_keys are parsed from the memfd
		require.Len(t, tmeta.ThreadlocalAttributeKeys, 3, "should have 3 threadlocal attribute keys")
		assert.Equal(t, "http.method", tmeta.ThreadlocalAttributeKeys[0])
		assert.Equal(t, "http.target", tmeta.ThreadlocalAttributeKeys[1])
		assert.Equal(t, "http.user", tmeta.ThreadlocalAttributeKeys[2])
	})

	test.RunMultiMode(t, "validate-tracer-serialization", func(t *testing.T, _ wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd) {
		consumer.eventReceived.Store(false)
		consumer.capturedPid.Store(0)
		consumer.capturedFd.Store(0)

		cmdExec := cmd(syscallTester, []string{"tracer-memfd"}, nil)
		_ = cmdExec.Run()

		require.Eventually(t, consumer.eventReceived.Load, 2*time.Second, 200*time.Millisecond, "tracer-memfd event should be received")

		consumer.capturedMutex.Lock()
		serializedJSON := consumer.capturedSerializedJSON
		consumer.capturedMutex.Unlock()

		require.NotEmpty(t, serializedJSON, "serialized JSON should not be empty")

		// Unmarshal the serialized event and validate the tracer field
		var data map[string]interface{}
		err := json.Unmarshal(serializedJSON, &data)
		require.NoError(t, err, "failed to unmarshal serialized event")

		processData, ok := data["process"].(map[string]interface{})
		require.True(t, ok, "process field should be present in serialized event")

		tracerData, ok := processData["tracer"].(map[string]interface{})
		require.True(t, ok, "tracer field should be present in serialized process, got: %v", processData)

		assert.Equal(t, "test-service", tracerData["service_name"], "service_name mismatch")
		assert.Equal(t, "test-env", tracerData["service_env"], "service_env mismatch")
		assert.Equal(t, "1.0.0", tracerData["service_version"], "service_version mismatch")
		assert.Contains(t, tracerData["process_tags"], "custom.tag:value", "process_tags should contain custom.tag:value")
	})
}
