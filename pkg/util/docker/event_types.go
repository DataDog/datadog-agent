// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"sync"
	"time"
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

// ContainerEntityName returns the event's container as a tagger entity name
func (ev *ContainerEvent) ContainerEntityName() string {
	return ContainerIDToTaggerEntityName(ev.ContainerID)
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
