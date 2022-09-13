// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

const (
	// ContainerEventActionStart is the action of starting a docker container
	ContainerEventActionStart = "start"
	// ContainerEventActionDie is the action of stopping a docker container
	ContainerEventActionDie = "die"
	// ContainerEventActionDied is the action of stopping a podman container
	ContainerEventActionDied = "died"
	// ContainerEventActionRename is the action of renaming a docker container
	ContainerEventActionRename = "rename"
	// ContainerEventActionHealthStatus is the action of changing a docker
	// container's health status
	ContainerEventActionHealthStatus = "health_status"
)

// ContainerEvent describes an event from the docker daemon
type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	ImageName     string
	Action        string
	Timestamp     time.Time
	Attributes    map[string]string
}

// Errors client might receive
var (
	ErrAlreadySubscribed = errors.New("already subscribed")
	ErrNotSubscribed     = errors.New("not subscribed")
	ErrEventTimeout      = errors.New("timeout on event sending, re-subscribe")
)

// eventSubscriber holds the state for a subscriber
type eventSubscriber struct {
	name       string
	eventChan  chan *ContainerEvent
	errorChan  chan error
	cancelChan chan struct{}
	filter     *containers.Filter
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
