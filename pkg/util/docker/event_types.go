// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"sync"
	"time"

	"github.com/docker/docker/api/types/events"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ActionDied is a custom action for container events
// This action is a podman-specific event
const ActionDied = events.Action("died")

var containerEventActions = []events.Action{
	events.ActionStart,
	events.ActionDie,
	events.ActionRename,
	events.ActionHealthStatus,
}

var imageEventActions = []events.Action{
	events.ActionPull,
	events.ActionDelete,
	events.ActionTag,
	events.ActionUnTag,
	ActionDied,
	// TODO: consider adding more image events such as events.ActionImport
}

var actionPrefixes = []events.Action{
	events.ActionExecDie,
	events.ActionExecStart,
	events.ActionExecDetach,
	events.ActionExecCreate,
	events.ActionHealthStatusRunning,
	events.ActionHealthStatusHealthy,
	events.ActionHealthStatusUnhealthy,
}

// ContainerEvent describes a container event from the docker daemon
type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	ImageName     string
	Action        events.Action
	Timestamp     time.Time
	Attributes    map[string]string
}

// ImageEvent describes an image event from the docker daemon
type ImageEvent struct {
	ImageID   string // In some events this is a sha, in others it's a name with tag
	Action    events.Action
	Timestamp time.Time
	// There are more attributes in the original event. Add them here if they're needed
}

// Errors client might receive
var (
	ErrAlreadySubscribed = errors.New("already subscribed")
	ErrNotSubscribed     = errors.New("not subscribed")
)

// eventSubscriber holds the state for a subscriber
type eventSubscriber struct {
	name                string
	containerEventsChan chan *ContainerEvent
	imageEventsChan     chan *ImageEvent
	cancelChan          chan struct{}
	filter              *containers.Filter
}

// eventStreamState holds the state for event streaming towards subscribers
type eventStreamState struct {
	sync.RWMutex
	subscribers map[string]*eventSubscriber
	cancelChan  chan struct{}
}

func newEventStreamState() *eventStreamState {
	return &eventStreamState{
		subscribers: make(map[string]*eventSubscriber),
		cancelChan:  make(chan struct{}),
	}
}
