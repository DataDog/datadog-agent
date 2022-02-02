// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/containerd/containerd"
	containerdevents "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events"
	prototypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

type mockEvt struct {
	events.Publisher
	events.Forwarder
	mockSubscribe func(ctx context.Context, filter ...string) (ch <-chan *events.Envelope, errs <-chan error)
}

func (m *mockEvt) Subscribe(ctx context.Context, filters ...string) (ch <-chan *events.Envelope, errs <-chan error) {
	return m.mockSubscribe(ctx)
}

// TestCheckEvent is an integration test as the underlying logic that we test is the listener for events.
func TestCheckEvents(t *testing.T) {
	cha := make(chan *events.Envelope)
	errorsCh := make(chan error)
	me := &mockEvt{
		mockSubscribe: func(ctx context.Context, filter ...string) (ch <-chan *events.Envelope, errs <-chan error) {
			return cha, errorsCh
		},
	}
	itf := &fake.MockedContainerdClient{
		MockEvents: func() containerd.EventService {
			return containerd.EventService(me)
		},
	}
	// Test the basic listener
	sub := CreateEventSubscriber("subscriberTest1", nil)
	sub.CheckEvents(containerdutil.ContainerdItf(itf))

	tp := containerdevents.TaskPaused{
		ContainerID: "42",
	}
	vp, err := tp.Marshal()
	assert.NoError(t, err)

	en := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/tasks/paused",
		Event: &prototypes.Any{
			Value: vp,
		},
	}
	cha <- &en

	timeout := time.NewTimer(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	condition := false
	for {
		select {
		case <-ticker.C:
			if !sub.IsRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting event listener to be healthy")
		}
		if condition {
			break
		}
	}

	ev := sub.Flush(time.Now().Unix())
	assert.Len(t, ev, 1)
	assert.Equal(t, ev[0].Topic, "/tasks/paused")
	errorsCh <- fmt.Errorf("chan breaker")
	condition = false
	for {
		select {
		case <-ticker.C:
			if sub.IsRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting for error")
		}
		if condition {
			break
		}
	}

	// Test the multiple events one unsupported
	sub = CreateEventSubscriber("subscriberTest2", nil)
	sub.CheckEvents(containerdutil.ContainerdItf(itf))

	tk := containerdevents.TaskOOM{
		ContainerID: "42",
	}
	vk, err := tk.Marshal()
	assert.NoError(t, err)

	ek := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/tasks/oom",
		Event: &prototypes.Any{
			Value: vk,
		},
	}

	nd := containerdevents.NamespaceDelete{
		Name: "k10s.io",
	}
	vnd, err := nd.Marshal()
	assert.NoError(t, err)

	evnd := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/namespaces/delete",
		Event: &prototypes.Any{
			Value: vnd,
		},
	}

	cha <- &ek
	cha <- &evnd

	condition = false
	for {
		select {
		case <-ticker.C:
			if !sub.IsRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting event listener to be healthy")
		}
		if condition {
			break
		}
	}
	ev2 := sub.Flush(time.Now().Unix())
	fmt.Printf("\n\n 2/ Flush %v\n\n", ev2)
	assert.Len(t, ev2, 1)
	assert.Equal(t, ev2[0].Topic, "/tasks/oom")

}
